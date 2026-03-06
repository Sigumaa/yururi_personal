package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/decision"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

const autonomyPulseJobID = "autonomy-pulse"

func (a *App) runConversationTurn(ctx context.Context, threadID string, msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact, currentImageURLs []string) (string, error) {
	prompt := buildConversationPrompt(msg, profile, recent, facts, a.tools.Specs(), a.discordSelfMention(), len(currentImageURLs))
	a.logger.Info("conversation turn start", "thread_id", threadID, "channel", msg.ChannelName, "message_id", msg.ID, "prompt_bytes", len(prompt))
	a.logger.Debug("conversation prompt", "thread_id", threadID, "channel", msg.ChannelName, "message_id", msg.ID, "prompt_preview", previewText(prompt, 1800))
	input := []codex.InputItem{codex.TextInput(prompt)}
	imageInputs, imageNotes := a.buildImageInputs(ctx, currentImageURLs)
	if len(imageInputs) > 0 {
		input = append(input, imageInputs...)
		a.logger.Debug("conversation image inputs ready", "thread_id", threadID, "channel", msg.ChannelName, "message_id", msg.ID, "count", len(imageInputs), "notes", strings.Join(imageNotes, " | "))
	}
	raw, err := a.runThreadInputTurn(ctx, threadID, input)
	if err != nil {
		return "", fmt.Errorf("run conversation turn: %w", err)
	}
	a.logger.Info("conversation turn completed", "thread_id", threadID, "channel", msg.ChannelName, "message_id", msg.ID, "response_bytes", len(raw))
	a.logger.Debug("conversation output", "thread_id", threadID, "channel", msg.ChannelName, "message_id", msg.ID, "raw_preview", previewText(raw, 1800))
	reply := parseAssistantReply(raw)
	if reply.Action == decision.ActionIgnore || strings.TrimSpace(reply.Message) == "" {
		return "", nil
	}
	if looksLikePromiseOnly(reply.Message) {
		a.logger.Warn("conversation reply suppressed promise-only", "channel", msg.ChannelName, "message_id", msg.ID, "reply", previewText(reply.Message, 240))
		return "", nil
	}
	return strings.TrimSpace(reply.Message), nil
}

func (a *App) ensureAutonomyPulseJob(ctx context.Context) error {
	schedule := strings.TrimSpace(a.cfg.Behavior.AutonomyPulseInterval)
	if schedule == "" {
		return nil
	}

	existing, ok, err := a.store.GetJob(ctx, autonomyPulseJobID)
	if err != nil {
		return err
	}
	if ok && existing.Kind == "autonomy_pulse" && existing.ScheduleExpr == schedule {
		a.logger.Debug("autonomy pulse job already configured", "job_id", existing.ID, "schedule", existing.ScheduleExpr)
		return nil
	}

	job := jobs.NewJob(autonomyPulseJobID, "autonomy_pulse", "autonomy pulse", "", schedule, nil)
	job.NextRunAt = time.Now().UTC().Add(mustDuration(schedule, 7*time.Minute))
	if err := a.store.UpsertJob(ctx, job); err != nil {
		return err
	}
	a.logger.Info("autonomy pulse job ready", "job_id", job.ID, "schedule", job.ScheduleExpr)
	return nil
}

func (a *App) handleAutonomyPulseJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	nextRunAt := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, mustDuration(a.cfg.Behavior.AutonomyPulseInterval, 7*time.Minute)))
	session, err := a.ensureAutonomyThread(ctx)
	if err != nil {
		return jobs.Result{NextRunAt: nextRunAt}, err
	}

	targetChannelID, _, _ := a.store.GetKV(ctx, "autonomy.last_target_channel_id")
	if targetChannelID == "" {
		if latest, found, err := a.store.LatestChannelIDForAuthor(ctx, a.cfg.Discord.OwnerUserID); err == nil && found {
			targetChannelID = latest
		}
	}
	targetChannelName := targetChannelID
	if targetChannelID != "" && a.discord != nil {
		if channel, err := a.discord.GetChannel(ctx, targetChannelID); err == nil {
			targetChannelName = channel.Name
		}
	}

	latestPresence, _, _ := a.store.LastPresence(ctx, a.cfg.Discord.OwnerUserID)
	recentActivity, _ := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-12*time.Hour), 12)
	summaries, _ := a.store.RecentSummaries(ctx, "daily", 2)
	prompt := buildAutonomyPulsePrompt(targetChannelID, targetChannelName, latestPresence, recentActivity, summaries)
	a.logger.Info("autonomy pulse start", "job_id", job.ID, "thread_id", session.ID, "target_channel_id", targetChannelID, "prompt_bytes", len(prompt))
	a.logger.Debug("autonomy pulse prompt", "job_id", job.ID, "thread_id", session.ID, "prompt_preview", previewText(prompt, 1800))
	raw, err := a.runThreadTurn(ctx, session.ID, prompt)
	if err != nil {
		return jobs.Result{NextRunAt: nextRunAt}, err
	}
	a.logger.Info("autonomy pulse completed", "job_id", job.ID, "thread_id", session.ID, "response_bytes", len(raw))
	a.logger.Debug("autonomy pulse output", "job_id", job.ID, "thread_id", session.ID, "response_preview", previewText(raw, 1600))

	reply := parseAssistantReply(raw)
	if reply.Action == decision.ActionIgnore || strings.TrimSpace(reply.Message) == "" {
		return jobs.Result{NextRunAt: nextRunAt}, nil
	}
	if looksLikePromiseOnly(reply.Message) {
		return jobs.Result{NextRunAt: nextRunAt}, nil
	}
	if targetChannelID == "" || a.discord == nil {
		return jobs.Result{NextRunAt: nextRunAt, Details: strings.TrimSpace(reply.Message)}, nil
	}
	sentID, err := a.discord.SendMessage(ctx, targetChannelID, strings.TrimSpace(reply.Message))
	if err != nil {
		return jobs.Result{NextRunAt: nextRunAt}, err
	}
	a.logger.Info("autonomy pulse reply sent", "job_id", job.ID, "channel_id", targetChannelID, "message_id", sentID)
	return jobs.Result{
		NextRunAt:       nextRunAt,
		Details:         "autonomy pulse acted",
		AlreadyNotified: true,
	}, nil
}
