package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) handleReminderJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.discord == nil {
		return jobs.Result{Done: true}, errors.New("discord is not connected")
	}
	content, _ := job.Payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		content, _ = job.Payload["message"].(string)
	}
	if strings.TrimSpace(content) == "" {
		return jobs.Result{Done: true}, errors.New("payload.content or payload.message is required")
	}
	if strings.TrimSpace(job.ChannelID) == "" {
		return jobs.Result{Done: true}, errors.New("channel_id is required")
	}
	if _, err := a.discord.SendMessage(ctx, job.ChannelID, strings.TrimSpace(content)); err != nil {
		return jobs.Result{Done: true}, err
	}
	return jobs.Result{
		NextRunAt:       time.Now().UTC(),
		Done:            true,
		Details:         "reminder sent",
		AlreadyNotified: true,
	}, nil
}

func (a *App) handleMonthlyReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-30 * 24 * time.Hour)
	nextRun := nextMonthClock(end, a.loc, 1, 21, 30)
	return a.runSummaryJob(ctx, job, "monthly", start.UTC(), end.UTC(), nextRun, false)
}

func (a *App) runReviewJob(ctx context.Context, job jobs.Job, period string, defaultInterval time.Duration, build func(context.Context) (string, bool, error)) (jobs.Result, error) {
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, defaultInterval))
	prompt, ok, err := build(ctx)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun}, err
	}
	if !ok {
		return jobs.Result{NextRunAt: nextRun}, nil
	}
	return a.runNarrativeJob(ctx, job, period, prompt, nextRun, false)
}

func (a *App) runNarrativeJob(ctx context.Context, job jobs.Job, period string, prompt string, nextRun time.Time, done bool) (jobs.Result, error) {
	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}

	raw, err := a.runThreadTurn(ctx, session.ID, prompt)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	reply := parseAssistantReply(raw)
	if reply.Action == "ignore" || strings.TrimSpace(reply.Message) == "" {
		return jobs.Result{NextRunAt: nextRun, Done: done}, nil
	}
	text := strings.TrimSpace(reply.Message)
	if strings.TrimSpace(job.ChannelID) != "" && a.discord != nil {
		if _, err := a.discord.SendMessage(ctx, job.ChannelID, text); err != nil {
			return jobs.Result{NextRunAt: nextRun, Done: done}, err
		}
	}
	now := time.Now().UTC()
	if err := a.store.SaveSummary(ctx, memory.Summary{
		Period:    period,
		ChannelID: job.ChannelID,
		Content:   text,
		StartsAt:  now,
		EndsAt:    now,
		CreatedAt: now,
	}); err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	return jobs.Result{
		NextRunAt:       nextRun,
		Done:            done,
		Details:         fmt.Sprintf("%s narrative saved", period),
		AlreadyNotified: strings.TrimSpace(job.ChannelID) != "",
	}, nil
}
