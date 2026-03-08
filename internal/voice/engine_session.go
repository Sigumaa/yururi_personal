package voice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func (e *Engine) Join(ctx context.Context, guildID string, channelID string) (Session, error) {
	now := time.Now().UTC()
	channel, err := e.discord.GetChannel(ctx, channelID)
	if err != nil {
		return Session{}, err
	}
	joined, err := e.discord.JoinVoice(ctx, guildID, channelID, false, false)
	if err != nil {
		return Session{}, err
	}
	members, err := e.discord.VoiceChannelMembers(ctx, guildID, channelID)
	if err != nil {
		return Session{}, err
	}
	session := Session{
		ID:          fmt.Sprintf("voice-%d", now.UnixNano()),
		GuildID:     guildID,
		ChannelID:   channelID,
		ChannelName: channel.Name,
		State:       SessionStateListening,
		StartedAt:   now,
		Members:     membersFromDiscord(members),
	}
	e.configureRealtime(ctx, &session)
	if err := e.store.UpsertVoiceSession(ctx, memory.VoiceSession{
		ID:          session.ID,
		GuildID:     session.GuildID,
		ChannelID:   session.ChannelID,
		ChannelName: session.ChannelName,
		State:       string(session.State),
		Source:      "discord_voice",
		StartedAt:   session.StartedAt,
		Metadata: map[string]any{
			"connected": joined.Connected,
			"self_mute": joined.SelfMute,
			"self_deaf": joined.SelfDeaf,
		},
	}); err != nil {
		return Session{}, err
	}
	e.startSessionRuntime(guildID, session)
	if err := e.store.SaveVoiceEvent(ctx, memory.VoiceEvent{
		SessionID: session.ID,
		Type:      "join",
		ChannelID: session.ChannelID,
		CreatedAt: now,
		Metadata: map[string]any{
			"channel_name": session.ChannelName,
			"realtime":     session.Realtime.Connected,
		},
	}); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (e *Engine) Leave(ctx context.Context, guildID string, reason string) error {
	runtime, ok := e.sessionRuntime(guildID)
	if ok {
		e.setSessionState(ctx, guildID, SessionStateLeaving)
	}
	if err := e.discord.LeaveVoice(ctx, guildID); err != nil {
		return err
	}
	if e.realtime != nil {
		_ = e.realtime.Close()
	}
	if !ok {
		return nil
	}
	runtime.cancel()
	now := time.Now().UTC()
	e.mu.Lock()
	delete(e.sessions, guildID)
	e.mu.Unlock()
	if err := e.store.EndVoiceSession(ctx, runtime.session.ID, now); err != nil {
		return err
	}
	return e.store.SaveVoiceEvent(ctx, memory.VoiceEvent{
		SessionID: runtime.session.ID,
		Type:      "leave",
		ChannelID: runtime.session.ChannelID,
		CreatedAt: now,
		Metadata:  map[string]any{"reason": strings.TrimSpace(reason)},
	})
}

func (e *Engine) Status(ctx context.Context, guildID string) (Session, bool, error) {
	e.mu.RLock()
	runtime, ok := e.sessions[guildID]
	e.mu.RUnlock()
	if ok {
		session := runtime.session
		session.Realtime = statusOf(e.realtime)
		return session, true, nil
	}
	active, ok, err := e.store.ActiveVoiceSession(ctx, guildID)
	if err != nil || !ok {
		return Session{}, false, err
	}
	out := Session{
		ID:          active.ID,
		GuildID:     active.GuildID,
		ChannelID:   active.ChannelID,
		ChannelName: active.ChannelName,
		State:       SessionState(active.State),
		StartedAt:   active.StartedAt,
		EndedAt:     active.EndedAt,
		Realtime:    statusOf(e.realtime),
	}
	if current, ok, err := e.discord.CurrentVoiceSession(ctx, guildID); err == nil && ok {
		out.ChannelID = current.ChannelID
		out.ChannelName = current.ChannelName
		out.State = SessionStateListening
	}
	members, err := e.discord.VoiceChannelMembers(ctx, guildID, out.ChannelID)
	if err == nil {
		out.Members = membersFromDiscord(members)
	}
	return out, true, nil
}

func (e *Engine) HandleVoiceStateUpdate(ctx context.Context, event *discordgo.VoiceStateUpdate) error {
	if event == nil || event.VoiceState == nil {
		return nil
	}
	if event.UserID == e.ownerUserID {
		if strings.TrimSpace(event.ChannelID) == "" {
			if err := e.store.SetKV(ctx, "voice.last_owner_channel_id", ""); err != nil {
				return err
			}
		} else if err := e.store.SetKV(ctx, "voice.last_owner_channel_id", event.ChannelID); err != nil {
			return err
		}
	}

	session, ok, err := e.Status(ctx, event.GuildID)
	if err != nil || !ok {
		return err
	}
	beforeChannelID := ""
	if event.BeforeUpdate != nil {
		beforeChannelID = strings.TrimSpace(event.BeforeUpdate.ChannelID)
	}
	if strings.TrimSpace(event.ChannelID) != session.ChannelID && beforeChannelID != session.ChannelID {
		return nil
	}
	members, err := e.discord.VoiceChannelMembers(ctx, event.GuildID, session.ChannelID)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if runtime, ok := e.sessions[event.GuildID]; ok {
		runtime.session.Members = membersFromDiscord(members)
	}
	e.mu.Unlock()
	eventType := "member_updated"
	switch {
	case beforeChannelID == "" && strings.TrimSpace(event.ChannelID) != "":
		eventType = "member_joined"
	case beforeChannelID != "" && strings.TrimSpace(event.ChannelID) == "":
		eventType = "member_left"
	case beforeChannelID != strings.TrimSpace(event.ChannelID):
		eventType = "member_moved"
	}
	return e.store.SaveVoiceEvent(ctx, memory.VoiceEvent{
		SessionID: session.ID,
		Type:      eventType,
		UserID:    event.UserID,
		ChannelID: event.ChannelID,
		CreatedAt: time.Now().UTC(),
	})
}

func (e *Engine) RecordTranscript(ctx context.Context, guildID string, segment memory.VoiceTranscriptSegment) error {
	session, ok, err := e.Status(ctx, guildID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("voice session is not active")
	}
	if strings.TrimSpace(segment.SessionID) == "" {
		segment.SessionID = session.ID
	}
	if segment.StartedAt.IsZero() {
		segment.StartedAt = time.Now().UTC()
	}
	if err := e.store.SaveVoiceTranscript(ctx, segment); err != nil {
		return err
	}
	return e.mirrorTranscriptAsMessage(ctx, session, segment)
}

func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.RLock()
	guildIDs := make([]string, 0, len(e.sessions))
	for guildID := range e.sessions {
		guildIDs = append(guildIDs, guildID)
	}
	e.mu.RUnlock()
	for _, guildID := range guildIDs {
		if err := e.Leave(ctx, guildID, "shutdown"); err != nil {
			return err
		}
	}
	if e.realtime != nil {
		return e.realtime.Close()
	}
	return nil
}
