package voice

import (
	"context"
	"fmt"
	"strings"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
)

const voiceInputSilenceWindow = 900 * time.Millisecond

func (e *Engine) consumeDiscordAudio(ctx context.Context, guildID string, runtime *runtimeSession) {
	if runtime == nil || runtime.audio == nil || e.discord == nil || e.realtime == nil {
		return
	}
	packets, err := e.discord.VoiceAudioPackets(ctx, guildID)
	if err != nil {
		e.logger.Warn("voice packet stream unavailable", "guild_id", guildID, "session_id", runtime.session.ID, "error", err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok := <-packets:
			if !ok {
				return
			}
			packet = e.resolveVoicePacketSpeaker(runtime, packet)
			if !e.shouldProcessVoicePacket(packet) {
				if packet.UserID == "" {
					e.logger.Debug("voice packet ignored", "guild_id", guildID, "session_id", runtime.session.ID, "reason", "unknown_speaker", "ssrc", packet.SSRC, "sequence", packet.Sequence, "opus_bytes", len(packet.Opus))
				}
				continue
			}
			if err := e.forwardDiscordPacket(ctx, guildID, runtime, packet); err != nil {
				e.logger.Warn("voice packet forwarding failed", "guild_id", guildID, "session_id", runtime.session.ID, "user_id", packet.UserID, "error", err)
			}
		}
	}
}

func (e *Engine) shouldProcessVoicePacket(packet discordsvc.VoicePacket) bool {
	if packet.UserID == "" {
		return false
	}
	return packet.UserID == e.ownerUserID
}

func (e *Engine) resolveVoicePacketSpeaker(runtime *runtimeSession, packet discordsvc.VoicePacket) discordsvc.VoicePacket {
	if runtime == nil || strings.TrimSpace(packet.UserID) != "" {
		return packet
	}
	ownerMembers := 0
	ownerName := ""
	for _, member := range runtime.session.Members {
		if member.Bot || member.UserID != e.ownerUserID {
			continue
		}
		ownerMembers++
		if ownerName == "" {
			ownerName = member.Username
		}
	}
	if ownerMembers != 1 {
		return packet
	}
	humanMembers := 0
	for _, member := range runtime.session.Members {
		if member.Bot {
			continue
		}
		humanMembers++
	}
	if humanMembers != 1 {
		return packet
	}
	packet.UserID = e.ownerUserID
	if strings.TrimSpace(packet.Username) == "" {
		packet.Username = ownerName
	}
	e.logger.Debug("voice packet speaker inferred", "guild_id", packet.GuildID, "channel_id", packet.ChannelID, "ssrc", packet.SSRC, "user_id", packet.UserID)
	return packet
}

func (e *Engine) forwardDiscordPacket(ctx context.Context, guildID string, runtime *runtimeSession, packet discordsvc.VoicePacket) error {
	pcm48, err := runtime.audio.decodeDiscordOpus(packet.Opus)
	if err != nil {
		return err
	}
	mono24 := downsampleDiscordToRealtime(pcm48)
	if len(mono24) == 0 {
		return nil
	}
	if runtime.session.State == SessionStateSpeaking || runtime.session.State == SessionStateThinking {
		if err := e.Interrupt(ctx, guildID, "owner_voice_activity"); err != nil {
			e.logger.Warn("voice interrupt on owner activity failed", "guild_id", guildID, "session_id", runtime.session.ID, "error", err)
		}
	}
	if err := e.realtime.AppendInputAudio(ctx, samplesToPCM16Bytes(mono24)); err != nil {
		return err
	}
	select {
	case runtime.inputActivity <- struct{}{}:
	default:
	}
	return nil
}

func (e *Engine) playRealtimeAudio(ctx context.Context, guildID string, event ServerEvent) error {
	runtime, ok := e.sessionRuntime(guildID)
	if !ok || runtime.audio == nil {
		return nil
	}
	delta := event.audioDelta()
	if delta == "" {
		e.logger.Debug("voice audio delta skipped", "guild_id", guildID, "event_type", event.Type, "reason", "empty_delta")
		return nil
	}
	samples, err := decodeAudioDelta(delta)
	if err != nil {
		return err
	}
	runtime.audio.appendRealtimeOutput(samples)
	frames, err := runtime.audio.drainOpusFrames()
	if err != nil {
		return err
	}
	e.logger.Debug("voice audio delta received", "guild_id", guildID, "event_type", event.Type, "sample_count", len(samples), "opus_frames", len(frames))
	for _, frame := range frames {
		if err := e.discord.SendVoiceOpus(ctx, guildID, frame); err != nil {
			return fmt.Errorf("send voice opus: %w", err)
		}
	}
	return nil
}

func (e *Engine) flushRealtimeAudio(ctx context.Context, guildID string) error {
	runtime, ok := e.sessionRuntime(guildID)
	if !ok || runtime.audio == nil {
		return nil
	}
	frames, err := runtime.audio.drainOpusFrames()
	if err != nil {
		return err
	}
	for _, frame := range frames {
		if err := e.discord.SendVoiceOpus(ctx, guildID, frame); err != nil {
			return fmt.Errorf("flush voice opus: %w", err)
		}
	}
	return nil
}

func (e *Engine) commitPendingVoiceTurn(guildID string, sessionID string) error {
	runtime, ok := e.sessionRuntime(guildID)
	if !ok || runtime == nil || runtime.session.ID != sessionID {
		return nil
	}
	switch runtime.session.State {
	case SessionStateThinking, SessionStateSpeaking, SessionStateLeaving:
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.realtime.CommitInputAudio(ctx); err != nil {
		return err
	}
	if err := e.realtime.CreateResponse(ctx); err != nil {
		return err
	}
	e.logger.Debug("voice input committed", "guild_id", guildID, "session_id", sessionID)
	return nil
}
