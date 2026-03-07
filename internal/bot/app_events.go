package bot

import (
	"context"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func (a *App) processMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	if event.Author == nil || event.Author.Bot {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	channelName := a.channelName(session, event.ChannelID)
	msg := memory.Message{
		ID:          event.ID,
		ChannelID:   event.ChannelID,
		ChannelName: channelName,
		AuthorID:    event.Author.ID,
		AuthorName:  event.Author.Username,
		Content:     event.Content,
		CreatedAt:   event.Timestamp,
		Metadata: map[string]any{
			"guild_id":    event.GuildID,
			"attachments": attachmentURLs(event.Attachments),
		},
	}
	a.logger.Info("message received", "channel", channelName, "channel_id", event.ChannelID, "author", event.Author.Username, "message_id", event.ID, "attachments", len(event.Attachments), "content_preview", previewText(event.Content, 240))
	if err := a.store.SaveMessage(ctx, msg); err != nil {
		a.logger.Error("save message failed", "error", err)
		return
	}
	if msg.AuthorID == a.cfg.Discord.OwnerUserID {
		if err := a.store.SetKV(ctx, "autonomy.last_target_channel_id", msg.ChannelID); err != nil {
			a.logger.Warn("save autonomy target channel failed", "channel_id", msg.ChannelID, "error", err)
		}
	}

	profile, err := a.resolveChannelProfile(ctx, event.ChannelID, channelName)
	if err != nil {
		a.logger.Error("resolve channel profile failed", "error", err)
		return
	}

	recent, _ := a.store.RecentMessages(ctx, event.ChannelID, 12)
	facts, _ := a.collectConversationFacts(ctx, msg, 12)
	a.logger.Info("message context ready", "channel", channelName, "recent_messages", len(recent), "related_facts", len(facts), "profile_kind", profile.Kind)
	a.logger.Debug("message context detail", "channel", channelName, "profile", previewJSON(profile, 320), "recent_preview", previewJSON(recent, 900), "fact_preview", previewJSON(facts, 900))

	threadSession, err := a.ensureChannelThread(ctx, msg.ChannelID)
	if err != nil {
		a.logger.Error("ensure channel thread failed", "channel", channelName, "channel_id", msg.ChannelID, "error", err)
		return
	}
	if err := a.interruptChannelTurn(ctx, msg.ChannelID, threadSession.ID, msg.ID); err != nil {
		a.logger.Warn("interrupt active channel turn failed", "channel", channelName, "channel_id", msg.ChannelID, "thread_id", threadSession.ID, "error", err)
	}

	reply, err := a.runConversationTurn(ctx, threadSession.ID, msg, profile, recent, facts, imageAttachmentURLs(event.Attachments))
	if err != nil {
		if err == codex.ErrTurnInterrupted {
			a.logger.Info("conversation turn interrupted", "channel", channelName, "channel_id", msg.ChannelID, "message_id", msg.ID)
			return
		}
		a.logger.Error("conversation turn failed", "channel", channelName, "channel_id", msg.ChannelID, "error", err)
		return
	}
	if strings.TrimSpace(reply) == "" {
		a.logger.Info("reply skipped", "channel", channelName, "message_id", event.ID, "reason", "empty_or_no_reply")
		return
	}

	sentID, err := a.discord.SendMessage(ctx, msg.ChannelID, reply)
	if err != nil {
		a.logger.Error("send reply failed", "error", err)
		return
	}
	a.logger.Info("reply sent", "channel", msg.ChannelName, "channel_id", msg.ChannelID, "message_id", sentID)
}

func (a *App) interruptChannelTurn(ctx context.Context, channelID string, threadID string, messageID string) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}

	interruptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	turnID, ok, err := a.codex.InterruptActiveTurn(interruptCtx, threadID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	a.logger.Info("active channel turn interrupted", "channel_id", channelID, "thread_id", threadID, "interrupted_turn_id", turnID, "next_message_id", messageID)
	return nil
}

func (a *App) processPresence(event *discordgo.PresenceUpdate) {
	if event.User == nil || event.User.ID != a.cfg.Discord.OwnerUserID {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	previous, ok, err := a.store.LastPresence(ctx, event.User.ID)
	if err != nil {
		a.logger.Warn("load last presence failed", "error", err)
	}

	activities := make([]string, 0, len(event.Activities))
	for _, activity := range event.Activities {
		activities = append(activities, activity.Name)
	}

	now := time.Now().UTC()
	current := memory.PresenceSnapshot{
		UserID:     event.User.ID,
		Status:     string(event.Status),
		Activities: activities,
		StartedAt:  now,
	}
	if err := a.store.SavePresence(ctx, current); err != nil {
		a.logger.Warn("save presence failed", "error", err)
	}
	a.logger.Info("presence updated", "status", current.Status, "activities", strings.Join(current.Activities, ","))

	if ok && isOffline(previous.Status) && !isOffline(current.Status) && now.Sub(previous.StartedAt) >= defaultWakeSummaryThreshold {
		channelID, found, err := a.store.LatestChannelIDForAuthor(ctx, a.cfg.Discord.OwnerUserID)
		if err != nil {
			a.logger.Warn("resolve wake summary channel failed", "error", err)
			return
		}
		if !found {
			return
		}

		job := jobs.NewJob(jobID("wake-summary"), "wake_summary", "wake summary", channelID, "10s", map[string]any{
			"since": previous.StartedAt.Format(time.RFC3339Nano),
		})
		job.NextRunAt = now.Add(10 * time.Second)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			a.logger.Warn("schedule wake summary failed", "error", err)
			return
		}
		a.logger.Info("wake summary scheduled", "job_id", job.ID, "channel_id", channelID)
	}
}
