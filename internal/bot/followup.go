package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) handleJobResult(job jobs.Job, result jobs.Result, runErr error) {
	if a.discord == nil || strings.TrimSpace(job.ChannelID) == "" {
		a.logger.Debug("job observer skipped", "job_id", job.ID, "kind", job.Kind, "reason", "missing_discord_or_channel")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	session, err := a.ensureChannelThread(ctx, job.ChannelID)
	if err != nil {
		a.logger.Warn("job follow-up thread unavailable", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}

	a.logger.Info("job observer fired", "job_id", job.ID, "kind", job.Kind, "done", result.Done, "already_notified", result.AlreadyNotified, "details_preview", previewText(result.Details, 320), "error", runErr)
	prompt := buildJobFollowUpPrompt(job, result, runErr)
	a.logger.Debug("job follow-up prompt", "job_id", job.ID, "kind", job.Kind, "prompt_preview", previewText(prompt, 1400))
	raw, err := a.runThreadTurn(ctx, session.ID, prompt)
	if err != nil {
		a.logger.Warn("job follow-up turn failed", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}
	a.logger.Debug("job follow-up output", "job_id", job.ID, "kind", job.Kind, "raw_preview", previewText(raw, 800))
	reply := parseAssistantReply(raw)
	if reply.Action == assistantActionIgnore || strings.TrimSpace(reply.Message) == "" {
		a.logger.Debug("job follow-up skipped", "job_id", job.ID, "kind", job.Kind, "reason", "assistant_ignore_or_empty")
		return
	}
	sentID, err := a.discord.SendMessage(ctx, job.ChannelID, strings.TrimSpace(reply.Message))
	if err != nil {
		a.logger.Warn("job follow-up send failed", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}
	a.logger.Info("job follow-up sent", "job_id", job.ID, "kind", job.Kind, "message_id", sentID)
}

func (a *App) handleBackgroundCodexTaskJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	prompt, _ := job.Payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return jobs.Result{Done: true}, fmt.Errorf("payload.prompt is required")
	}

	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{Done: true}, err
	}

	taskPrompt := buildBackgroundTaskPrompt(job, prompt)
	a.logger.Info("background codex task start", "job_id", job.ID, "title", job.Title, "thread_id", session.ID, "prompt_bytes", len(taskPrompt))
	raw, err := a.runThreadTurn(ctx, session.ID, taskPrompt)
	if err != nil {
		return jobs.Result{Done: true}, err
	}
	a.logger.Info("background codex task completed", "job_id", job.ID, "thread_id", session.ID, "response_bytes", len(raw))
	a.logger.Debug("background codex task output", "job_id", job.ID, "thread_id", session.ID, "response_preview", previewText(raw, 1600))
	return jobs.Result{
		NextRunAt: time.Now().UTC(),
		Done:      true,
		Details:   strings.TrimSpace(raw),
	}, nil
}

func (a *App) handlePeriodicCodexTaskJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	prompt, _ := job.Payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, time.Hour))}, fmt.Errorf("payload.prompt is required")
	}

	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, time.Hour))}, err
	}

	taskPrompt := buildBackgroundTaskPrompt(job, prompt)
	a.logger.Info("periodic codex task start", "job_id", job.ID, "title", job.Title, "thread_id", session.ID, "prompt_bytes", len(taskPrompt))
	raw, err := a.runThreadTurn(ctx, session.ID, taskPrompt)
	nextRunAt := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 6*time.Hour))
	if err != nil {
		return jobs.Result{NextRunAt: nextRunAt}, err
	}
	a.logger.Info("periodic codex task completed", "job_id", job.ID, "thread_id", session.ID, "response_bytes", len(raw))
	a.logger.Debug("periodic codex task output", "job_id", job.ID, "thread_id", session.ID, "response_preview", previewText(raw, 1600))
	return jobs.Result{
		NextRunAt: nextRunAt,
		Details:   strings.TrimSpace(raw),
	}, nil
}
