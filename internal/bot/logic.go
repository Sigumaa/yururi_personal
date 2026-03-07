package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

const noReplyToken = "<NO_REPLY>"

type assistantAction string

const (
	assistantActionIgnore assistantAction = "ignore"
	assistantActionReply  assistantAction = "reply"
)

type assistantReply struct {
	Action  assistantAction
	Reason  string
	Message string
}

func parseAssistantReply(raw string) assistantReply {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "", strings.EqualFold(trimmed, noReplyToken):
		return assistantReply{
			Action: assistantActionIgnore,
			Reason: "codex selected silence",
		}
	default:
		return assistantReply{
			Action:  assistantActionReply,
			Reason:  "codex text reply",
			Message: trimmed,
		}
	}
}

func (a *App) handleReleaseWatchJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	repo, _ := job.Payload["repo"].(string)
	if repo == "" {
		repo = "openai/codex"
	}
	a.logger.Info("release watch check", "job_id", job.ID, "repo", repo)
	release, err := a.fetchLatestStableRelease(ctx, repo)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 6*time.Hour))
	if err != nil {
		return jobs.Result{NextRunAt: nextRun}, err
	}

	lastSeen, _ := job.Payload["last_seen_tag"].(string)
	if lastSeen == "" {
		job.Payload["last_seen_tag"] = release.TagName
		job.NextRunAt = nextRun
		job.UpdatedAt = time.Now().UTC()
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		return jobs.Result{NextRunAt: nextRun}, nil
	}
	if lastSeen != release.TagName {
		message := fmt.Sprintf("Codex の安定リリースが更新されていましたよ。\nよかったら見てみてくださいね。\n- tag: %s\n- name: %s\n- published: %s\n- url: %s",
			release.TagName, release.Name, release.PublishedAt.Format(time.RFC3339), release.HTMLURL)
		if _, err := a.discord.SendMessage(ctx, job.ChannelID, message); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		job.Payload["last_seen_tag"] = release.TagName
		job.NextRunAt = nextRun
		job.UpdatedAt = time.Now().UTC()
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		return jobs.Result{
			NextRunAt:       nextRun,
			Details:         fmt.Sprintf("release updated: %s %s", release.TagName, release.HTMLURL),
			AlreadyNotified: true,
		}, nil
	}
	return jobs.Result{NextRunAt: nextRun}, nil
}

func (a *App) handleDailySummaryJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-24 * time.Hour)
	nextRun := nextLocalClock(end, a.loc, 23, 30)
	return a.runSummaryJob(ctx, job, "daily", start.UTC(), end.UTC(), nextRun, false)
}

func (a *App) handleWeeklyReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-7 * 24 * time.Hour)
	nextRun := nextWeekdayClock(end, a.loc, time.Sunday, 21, 0)
	return a.runSummaryJob(ctx, job, "weekly", start.UTC(), end.UTC(), nextRun, false)
}

func (a *App) handleGrowthLogJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	nextRun := nextLocalClock(time.Now().In(a.loc), a.loc, 23, 45)
	return a.runSummaryJob(ctx, job, "growth", start, end, nextRun, false)
}

func (a *App) handleSpaceReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.discord == nil {
		return jobs.Result{Done: true}, fmt.Errorf("discord is not connected")
	}
	if strings.TrimSpace(job.ChannelID) == "" {
		return jobs.Result{Done: true}, fmt.Errorf("space review channel is required")
	}

	sinceHours := 168
	if raw, ok := job.Payload["since_hours"]; ok {
		switch value := raw.(type) {
		case float64:
			if value > 0 {
				sinceHours = int(value)
			}
		case int:
			if value > 0 {
				sinceHours = value
			}
		}
	}

	channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
	if err != nil {
		return jobs.Result{Done: false}, err
	}
	profiles, err := a.store.ListChannelProfiles(ctx)
	if err != nil {
		return jobs.Result{Done: false}, err
	}
	activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(sinceHours)*time.Hour), 256)
	if err != nil {
		return jobs.Result{Done: false}, err
	}

	report := "空間整理の候補を見てきましたよ。\n" + describeSpaceCandidates(channels, profiles, activity, a.loc)
	if _, err := a.discord.SendMessage(ctx, job.ChannelID, report); err != nil {
		return jobs.Result{Done: false}, err
	}

	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 24*time.Hour))
	return jobs.Result{
		NextRunAt:       nextRun,
		Done:            false,
		Details:         "space review sent",
		AlreadyNotified: true,
	}, nil
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
			"summary": map[string]any{
				"type": "string",
			},
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

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

func (a *App) fetchLatestStableRelease(ctx context.Context, repo string) (githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=10", repo)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", a.cfg.AppName)

	response, err := a.http.Do(request)
	if err != nil {
		return githubRelease{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return githubRelease{}, fmt.Errorf("github releases: %s", strings.TrimSpace(string(body)))
	}
	var releases []githubRelease
	if err := json.NewDecoder(response.Body).Decode(&releases); err != nil {
		return githubRelease{}, err
	}
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			continue
		}
		return release, nil
	}
	return githubRelease{}, fmt.Errorf("no stable release found for %s", repo)
}
