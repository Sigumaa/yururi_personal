package voice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (e *Engine) AppendInputAudio(ctx context.Context, guildID string, pcm []byte) error {
	if _, ok, err := e.Status(ctx, guildID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("voice session is not active")
	}
	if e.realtime == nil {
		return fmt.Errorf("realtime is not configured")
	}
	return e.realtime.AppendInputAudio(ctx, pcm)
}

func (e *Engine) CommitInputAudio(ctx context.Context, guildID string) error {
	if _, ok, err := e.Status(ctx, guildID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("voice session is not active")
	}
	if e.realtime == nil {
		return fmt.Errorf("realtime is not configured")
	}
	return e.realtime.CommitInputAudio(ctx)
}

func (e *Engine) ClearInputAudio(ctx context.Context, guildID string) error {
	if _, ok, err := e.Status(ctx, guildID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("voice session is not active")
	}
	if e.realtime == nil {
		return fmt.Errorf("realtime is not configured")
	}
	return e.realtime.ClearInputAudio(ctx)
}

func (e *Engine) RequestResponse(ctx context.Context, guildID string) error {
	if _, ok, err := e.Status(ctx, guildID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("voice session is not active")
	}
	if e.realtime == nil {
		return fmt.Errorf("realtime is not configured")
	}
	return e.realtime.CreateResponse(ctx)
}

func (e *Engine) Interrupt(ctx context.Context, guildID string, reason string) error {
	session, ok, err := e.Status(ctx, guildID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("voice session is not active")
	}
	if e.realtime != nil {
		if err := e.realtime.CancelResponse(ctx); err != nil {
			return err
		}
	}
	if runtime, ok := e.sessionRuntime(guildID); ok && runtime.audio != nil {
		runtime.audio.resetOutput()
		if runtime.playbackActive {
			if err := e.discord.SetVoiceSpeaking(ctx, guildID, false); err != nil {
				return err
			}
			runtime.playbackActive = false
		}
	}
	if err := e.store.SaveVoiceEvent(ctx, memory.VoiceEvent{
		SessionID: session.ID,
		Type:      "interrupted",
		ChannelID: session.ChannelID,
		CreatedAt: time.Now().UTC(),
		Metadata:  map[string]any{"reason": strings.TrimSpace(reason)},
	}); err != nil {
		return err
	}
	e.setSessionState(ctx, guildID, SessionStateInterrupted)
	return nil
}
