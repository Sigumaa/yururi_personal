package voice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

type runtimeSession struct {
	session Session
	cancel  context.CancelFunc
	audio   *audioRuntime

	playbackActive bool
	inferredSSRC   map[uint32]struct{}
	outputItemID   string
	outputAudioMS  int
}

func (e *Engine) sessionRuntime(guildID string) (*runtimeSession, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	runtime, ok := e.sessions[guildID]
	return runtime, ok
}

func (e *Engine) configureRealtime(ctx context.Context, session *Session) {
	session.Realtime = statusOf(e.realtime)
	if e.realtime == nil {
		return
	}
	if err := e.realtime.Connect(ctx); err != nil {
		e.logger.Warn("voice realtime connect failed", "guild_id", session.GuildID, "channel_id", session.ChannelID, "error", err)
		session.Realtime = statusOf(e.realtime)
		return
	}
	if err := e.realtime.ConfigureSession(ctx, DefaultSessionConfig(session.ChannelName)); err != nil {
		e.logger.Warn("voice realtime session configure failed", "guild_id", session.GuildID, "channel_id", session.ChannelID, "error", err)
	}
	cfg := DefaultSessionConfig(session.ChannelName)
	e.logger.Info(
		"voice realtime configured",
		"guild_id", session.GuildID,
		"channel_id", session.ChannelID,
		"voice", cfg.Voice,
		"turn_detection", cfg.TurnDetection,
		"turn_detection_eagerness", cfg.TurnDetectionEagerness,
		"create_response", cfg.CreateResponse,
		"interrupt_response", cfg.InterruptResponse,
		"input_transcription_model", cfg.InputTranscriptionModel,
	)
	session.Realtime = statusOf(e.realtime)
}

func (e *Engine) startSessionRuntime(guildID string, session Session) {
	ctx, cancel := context.WithCancel(context.Background())
	audio, err := newAudioRuntime()
	if err != nil {
		e.logger.Warn("voice audio runtime init failed", "guild_id", guildID, "session_id", session.ID, "error", err)
	}
	runtime := &runtimeSession{
		session:      session,
		cancel:       cancel,
		audio:        audio,
		inferredSSRC: map[uint32]struct{}{},
	}
	e.mu.Lock()
	if current, ok := e.sessions[guildID]; ok && current.cancel != nil {
		current.cancel()
		if current.audio != nil {
			current.audio.Close()
		}
	}
	e.sessions[guildID] = runtime
	e.mu.Unlock()
	if e.realtime == nil {
	} else {
		go e.consumeRealtimeEvents(ctx, guildID, session.ID)
	}
	go e.consumeDiscordAudio(ctx, guildID, runtime)
}

func (e *Engine) consumeRealtimeEvents(ctx context.Context, guildID string, sessionID string) {
	events := e.realtime.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := e.handleRealtimeEvent(ctx, guildID, sessionID, event); err != nil {
				e.logger.Warn("voice realtime event handling failed", "guild_id", guildID, "session_id", sessionID, "event_type", event.Type, "error", err)
			}
		}
	}
}

func (e *Engine) handleRealtimeEvent(ctx context.Context, guildID string, sessionID string, event ServerEvent) error {
	now := time.Now().UTC()
	save := func(kind string, metadata map[string]any) error {
		return e.store.SaveVoiceEvent(ctx, memory.VoiceEvent{
			SessionID: sessionID,
			Type:      kind,
			ChannelID: e.sessionChannelID(guildID),
			CreatedAt: now,
			Metadata:  metadata,
		})
	}

	switch event.Type {
	case "session.created", "session.updated":
		settings := event.sessionSettings()
		e.logger.Info(
			"voice realtime session updated",
			"guild_id", guildID,
			"session_id", sessionID,
			"event_type", event.Type,
			"voice", settings.Voice,
			"turn_detection", settings.TurnDetection,
			"turn_detection_eagerness", settings.TurnDetectionEagerness,
			"create_response", settings.CreateResponse,
			"interrupt_response", settings.InterruptResponse,
			"input_transcription_model", settings.InputTranscriptionModel,
			"instructions_preview", previewText(settings.Instructions, 160),
		)
		return save("realtime_session_updated", map[string]any{
			"raw_type":                  event.Type,
			"voice":                     settings.Voice,
			"turn_detection":            settings.TurnDetection,
			"turn_detection_eagerness":  settings.TurnDetectionEagerness,
			"create_response":           settings.CreateResponse,
			"interrupt_response":        settings.InterruptResponse,
			"input_transcription_model": settings.InputTranscriptionModel,
		})
	case "input_audio_buffer.speech_started":
		return save("user_speaking_started", map[string]any{"raw_type": event.Type})
	case "input_audio_buffer.speech_stopped":
		if err := save("user_speaking_stopped", map[string]any{"raw_type": event.Type}); err != nil {
			return err
		}
		runtime, ok := e.sessionRuntime(guildID)
		if !ok || runtime == nil || runtime.session.ID != sessionID {
			return nil
		}
		switch runtime.session.State {
		case SessionStateThinking, SessionStateSpeaking, SessionStateLeaving:
			return nil
		}
		if err := e.realtime.CreateResponse(ctx); err != nil {
			return err
		}
		if err := e.setSessionState(ctx, guildID, SessionStateThinking); err != nil {
			return err
		}
		e.logger.Debug("voice response requested", "guild_id", guildID, "session_id", sessionID, "reason", "speech_stopped")
		return nil
	case "response.created":
		if err := save("response_created", map[string]any{"raw_type": event.Type, "response_id": event.responseID()}); err != nil {
			return err
		}
		return e.setSessionState(ctx, guildID, SessionStateThinking)
	case "response.output_item.added":
		itemID, role := event.outputItem()
		if role == "assistant" && itemID != "" {
			e.mu.Lock()
			if runtime, ok := e.sessions[guildID]; ok && runtime.session.ID == sessionID {
				runtime.outputItemID = itemID
				runtime.outputAudioMS = 0
			}
			e.mu.Unlock()
		}
		return save("response_output_item_added", map[string]any{
			"raw_type": event.Type,
			"item_id":  itemID,
			"role":     role,
		})
	case "response.audio.delta", "response.output_audio.delta":
		if err := save("response_streaming", map[string]any{"raw_type": event.Type, "response_id": event.responseID()}); err != nil {
			return err
		}
		if err := e.playRealtimeAudio(ctx, guildID, event); err != nil {
			return err
		}
		return e.setSessionState(ctx, guildID, SessionStateSpeaking)
	case "response.output_audio_transcript.delta", "response.audio_transcript.delta", "response.text.delta":
		return save("response_streaming", map[string]any{"raw_type": event.Type, "response_id": event.responseID()})
	case "response.done":
		if err := e.flushRealtimeAudio(ctx, guildID); err != nil {
			return err
		}
		e.mu.Lock()
		if runtime, ok := e.sessions[guildID]; ok && runtime.session.ID == sessionID {
			runtime.outputItemID = ""
			runtime.outputAudioMS = 0
		}
		e.mu.Unlock()
		if err := save("response_completed", map[string]any{"raw_type": event.Type, "response_id": event.responseID()}); err != nil {
			return err
		}
		return e.setSessionState(ctx, guildID, SessionStateListening)
	case "conversation.item.input_audio_transcription.completed":
		text := strings.TrimSpace(event.transcript())
		if text == "" {
			return nil
		}
		if err := e.RecordTranscript(ctx, guildID, memory.VoiceTranscriptSegment{
			SessionID:   sessionID,
			SpeakerID:   e.ownerUserID,
			SpeakerName: "owner",
			Role:        "user",
			Content:     text,
			StartedAt:   now,
			Metadata: map[string]any{
				"source":   "realtime",
				"item_id":  event.conversationItemID(),
				"raw_type": event.Type,
			},
		}); err != nil {
			return err
		}
		return save("transcript_user", map[string]any{"raw_type": event.Type, "item_id": event.conversationItemID()})
	case "response.output_audio_transcript.done", "response.audio_transcript.done", "response.text.done":
		text := strings.TrimSpace(event.transcript())
		if text == "" {
			return nil
		}
		if err := e.RecordTranscript(ctx, guildID, memory.VoiceTranscriptSegment{
			SessionID:   sessionID,
			SpeakerID:   "yururi",
			SpeakerName: "ゆるり",
			Role:        "assistant",
			Content:     text,
			StartedAt:   now,
			Metadata: map[string]any{
				"source":      "realtime",
				"response_id": event.responseID(),
				"item_id":     event.conversationItemID(),
				"raw_type":    event.Type,
			},
		}); err != nil {
			return err
		}
		return save("transcript_assistant", map[string]any{"raw_type": event.Type, "response_id": event.responseID()})
	case "error":
		return save("realtime_error", map[string]any{"raw_type": event.Type, "event": string(event.Raw)})
	default:
		return save("realtime_event", map[string]any{"raw_type": event.Type})
	}
}

func (e *Engine) setSessionState(ctx context.Context, guildID string, state SessionState) error {
	e.mu.Lock()
	runtime, ok := e.sessions[guildID]
	if !ok {
		e.mu.Unlock()
		return nil
	}
	runtime.session.State = state
	session := runtime.session
	e.mu.Unlock()
	return e.store.UpsertVoiceSession(ctx, memory.VoiceSession{
		ID:          session.ID,
		GuildID:     session.GuildID,
		ChannelID:   session.ChannelID,
		ChannelName: session.ChannelName,
		State:       string(state),
		Source:      "discord_voice",
		StartedAt:   session.StartedAt,
		EndedAt:     session.EndedAt,
		Metadata: map[string]any{
			"realtime_model":     session.Realtime.Model,
			"realtime_connected": session.Realtime.Connected,
		},
	})
}

func (e *Engine) sessionChannelID(guildID string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	runtime, ok := e.sessions[guildID]
	if !ok {
		return ""
	}
	return runtime.session.ChannelID
}

func (e *Engine) mirrorTranscriptAsMessage(ctx context.Context, session Session, segment memory.VoiceTranscriptSegment) error {
	authorID := strings.TrimSpace(segment.SpeakerID)
	if authorID == "" {
		authorID = segment.Role
	}
	authorName := strings.TrimSpace(segment.SpeakerName)
	if authorName == "" {
		authorName = segment.Role
	}
	messageID := fmt.Sprintf("voice:%s:%d:%s", segment.SessionID, segment.StartedAt.UnixNano(), segment.Role)
	return e.store.SaveMessage(ctx, memory.Message{
		ID:          messageID,
		ChannelID:   session.ChannelID,
		ChannelName: "[voice] " + session.ChannelName,
		AuthorID:    authorID,
		AuthorName:  authorName,
		Content:     segment.Content,
		CreatedAt:   segment.StartedAt,
		Metadata: map[string]any{
			"source":        "voice_transcript",
			"voice_session": segment.SessionID,
			"voice_role":    segment.Role,
		},
	})
}
