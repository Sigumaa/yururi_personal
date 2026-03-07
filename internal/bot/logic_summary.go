package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) handleDailySummaryJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-24 * time.Hour)
	return a.runSummaryJob(ctx, job, "daily", start.UTC(), end.UTC(), nextLocalClock(end, a.loc, 23, 30), false)
}

func (a *App) handleWeeklyReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-7 * 24 * time.Hour)
	return a.runSummaryJob(ctx, job, "weekly", start.UTC(), end.UTC(), nextWeekdayClock(end, a.loc, time.Sunday, 21, 0), false)
}

func (a *App) handleGrowthLogJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	return a.runSummaryJob(ctx, job, "growth", start, end, nextLocalClock(time.Now().In(a.loc), a.loc, 23, 45), false)
}

func (a *App) handleWakeSummaryJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	sinceRaw, _ := job.Payload["since"].(string)
	since, err := time.Parse(time.RFC3339Nano, sinceRaw)
	if err != nil {
		return jobs.Result{Done: true}, err
	}
	return a.runSummaryJob(ctx, job, "wake", since.UTC(), time.Now().UTC(), time.Now().UTC(), true)
}

func (a *App) runSummaryJob(ctx context.Context, job jobs.Job, period string, start time.Time, end time.Time, nextRun time.Time, done bool) (jobs.Result, error) {
	messages, err := a.store.MessagesBetween(ctx, start, end, 200)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	if len(messages) == 0 {
		a.logger.Info("summary skipped", "job_id", job.ID, "period", period, "reason", "no messages")
		return jobs.Result{NextRunAt: nextRun, Done: done}, nil
	}
	a.logger.Info("summary building", "job_id", job.ID, "period", period, "message_count", len(messages))
	a.logger.Debug("summary source messages", "job_id", job.ID, "period", period, "messages_preview", previewJSON(messages, 1600))

	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	summaryText, err := a.summarizeMessages(ctx, session.ID, period, start, end, messages)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	if _, err := a.discord.SendMessage(ctx, job.ChannelID, summaryText); err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	a.logger.Info("summary sent", "job_id", job.ID, "period", period, "channel_id", job.ChannelID)
	if err := a.store.SaveSummary(ctx, memory.Summary{
		Period:    period,
		ChannelID: job.ChannelID,
		Content:   summaryText,
		StartsAt:  start,
		EndsAt:    end,
	}); err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	return jobs.Result{
		NextRunAt:       nextRun,
		Done:            done,
		Details:         fmt.Sprintf("%s summary saved for %d messages", period, len(messages)),
		AlreadyNotified: true,
	}, nil
}

func (a *App) summarizeMessages(ctx context.Context, threadID string, period string, start time.Time, end time.Time, messages []memory.Message) (string, error) {
	if threadID == "" {
		return "", fmt.Errorf("summary thread is required")
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary"},
		"properties": map[string]any{
			"summary": map[string]any{"type": "string"},
		},
	}
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		lines = append(lines, fmt.Sprintf("- [%s/%s] %s: %s", msg.CreatedAt.In(a.loc).Format("01-02 15:04"), msg.ChannelName, msg.AuthorName, msg.Content))
	}
	prompt := fmt.Sprintf(`%s のまとめを作成してください。
期間: %s - %s
出力は JSON だけにし、summary に完成文を入れてください。
daily と wake は短め、weekly と monthly と growth は少し俯瞰を入れてください。
文章は日本語で、ゆるりとしてやわらかく。

messages:
%s`, period, start.In(a.loc).Format(time.RFC3339), end.In(a.loc).Format(time.RFC3339), strings.Join(lines, "\n"))

	raw, err := a.runThreadJSONTurn(ctx, threadID, prompt, schema)
	if err != nil {
		return "", err
	}
	a.logger.Debug("summary codex output", "period", period, "raw_preview", previewText(raw, 800))
	var response struct {
		Summary string `json:"summary"`
	}
	if parseErr := json.Unmarshal([]byte(raw), &response); parseErr != nil || strings.TrimSpace(response.Summary) == "" {
		if parseErr != nil {
			return "", fmt.Errorf("parse summary output: %w", parseErr)
		}
		return "", errors.New("summary output is empty")
	}
	a.logger.Debug("summary final text", "period", period, "summary_preview", previewText(response.Summary, 800))
	return response.Summary, nil
}
