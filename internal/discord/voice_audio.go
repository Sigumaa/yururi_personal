package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type voiceRuntime struct {
	conn     *discordgo.VoiceConnection
	guildID  string
	channel  string
	packets  chan VoicePacket
	closeCh  chan struct{}
	closed   bool
	speakers map[uint32]string
	mu       sync.RWMutex
}

type voiceJoiner interface {
	ChannelVoiceJoin(guildID string, channelID string, mute bool, deaf bool) (*discordgo.VoiceConnection, error)
}

type voiceE2EEJoiner interface {
	ChannelVoiceJoinE2EE(guildID string, channelID string, mute bool, deaf bool) (*discordgo.VoiceConnection, error)
}

func newVoiceRuntime(guildID string, channelID string, conn *discordgo.VoiceConnection) *voiceRuntime {
	return &voiceRuntime{
		conn:     conn,
		guildID:  guildID,
		channel:  channelID,
		packets:  make(chan VoicePacket, 64),
		closeCh:  make(chan struct{}),
		speakers: map[uint32]string{},
	}
}

func (r *voiceRuntime) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	close(r.closeCh)
	close(r.packets)
}

func (r *voiceRuntime) setSpeaker(ssrc uint32, userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(userID) == "" {
		return
	}
	r.speakers[ssrc] = userID
}

func (r *voiceRuntime) speaker(ssrc uint32) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.speakers[ssrc]
}

func joinVoiceConnection(session any, guildID string, channelID string, mute bool, deaf bool) (*discordgo.VoiceConnection, error) {
	if session == nil {
		return nil, errors.New("discord session is not initialized")
	}
	if joiner, ok := session.(voiceE2EEJoiner); ok {
		return joiner.ChannelVoiceJoinE2EE(guildID, channelID, mute, deaf)
	}
	joiner, ok := session.(voiceJoiner)
	if !ok {
		return nil, errors.New("discord session does not support voice join")
	}
	return joiner.ChannelVoiceJoin(guildID, channelID, mute, deaf)
}

func (c *Client) JoinVoice(ctx context.Context, guildID string, channelID string, mute bool, deaf bool) (VoiceSession, error) {
	startedAt := time.Now().UTC()
	channel, channelErr := c.GetChannel(ctx, channelID)
	conn, err := joinVoiceConnection(c.session, guildID, channelID, mute, deaf)
	if err != nil {
		closeEvent, ok := latestVoiceGatewayCloseSince(startedAt)
		if channelErr == nil {
			return VoiceSession{}, classifyVoiceJoinError(err, channel, closeEvent, ok)
		}
		return VoiceSession{}, classifyVoiceJoinError(err, Channel{ID: channelID}, closeEvent, ok)
	}
	if err := c.waitForVoiceReady(ctx, conn); err != nil {
		_ = conn.Disconnect()
		closeEvent, ok := latestVoiceGatewayCloseSince(startedAt)
		if channelErr == nil {
			return VoiceSession{}, classifyVoiceJoinError(fmt.Errorf("wait for voice ready: %w", err), channel, closeEvent, ok)
		}
		return VoiceSession{}, classifyVoiceJoinError(fmt.Errorf("wait for voice ready: %w", err), Channel{ID: channelID}, closeEvent, ok)
	}
	if channelErr != nil {
		_ = conn.Disconnect()
		return VoiceSession{}, channelErr
	}
	runtime := newVoiceRuntime(guildID, channelID, conn)
	conn.AddHandler(func(vc *discordgo.VoiceConnection, update *discordgo.VoiceSpeakingUpdate) {
		if update == nil {
			return
		}
		runtime.setSpeaker(uint32(update.SSRC), update.UserID)
	})
	go c.forwardVoicePackets(runtime)

	c.voiceMu.Lock()
	if previous := c.voiceRT[guildID]; previous != nil {
		previous.close()
		delete(c.voiceRT, guildID)
	}
	if previous := c.voiceConn[guildID]; previous != nil && previous != conn {
		_ = previous.Disconnect()
	}
	c.voiceConn[guildID] = conn
	c.voiceRT[guildID] = runtime
	c.voiceMu.Unlock()
	return VoiceSession{
		GuildID:     guildID,
		ChannelID:   channelID,
		ChannelName: channel.Name,
		Connected:   conn.Ready,
		SelfMute:    mute,
		SelfDeaf:    deaf,
	}, nil
}

func (c *Client) LeaveVoice(ctx context.Context, guildID string) error {
	c.voiceMu.Lock()
	conn := c.voiceConn[guildID]
	delete(c.voiceConn, guildID)
	runtime := c.voiceRT[guildID]
	delete(c.voiceRT, guildID)
	c.voiceMu.Unlock()
	if runtime != nil {
		runtime.close()
	}
	if conn == nil {
		return nil
	}
	prepareVoiceConnectionForLeave(conn)
	if err := conn.Disconnect(); err != nil {
		return wrapDiscordError("leave voice", err)
	}
	return nil
}

func prepareVoiceConnectionForLeave(conn *discordgo.VoiceConnection) {
	if conn == nil {
		return
	}
	conn.ChannelID = ""
}

func (c *Client) VoiceAudioPackets(ctx context.Context, guildID string) (<-chan VoicePacket, error) {
	c.voiceMu.RLock()
	runtime := c.voiceRT[guildID]
	c.voiceMu.RUnlock()
	if runtime == nil {
		return nil, fmt.Errorf("voice runtime is not active")
	}
	return runtime.packets, nil
}

func (c *Client) SendVoiceOpus(ctx context.Context, guildID string, opus []byte) error {
	c.voiceMu.RLock()
	conn := c.voiceConn[guildID]
	c.voiceMu.RUnlock()
	if conn == nil {
		return fmt.Errorf("voice connection is not active")
	}
	if len(opus) == 0 {
		return nil
	}
	select {
	case conn.OpusSend <- append([]byte(nil), opus...):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) SetVoiceSpeaking(ctx context.Context, guildID string, speaking bool) error {
	c.voiceMu.RLock()
	conn := c.voiceConn[guildID]
	c.voiceMu.RUnlock()
	if conn == nil {
		return fmt.Errorf("voice connection is not active")
	}
	if err := conn.Speaking(speaking); err != nil {
		return wrapDiscordError("set voice speaking", err)
	}
	return nil
}

func (c *Client) waitForVoiceReady(ctx context.Context, conn *discordgo.VoiceConnection) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if conn != nil && conn.Ready && conn.OpusSend != nil && conn.OpusRecv != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) forwardVoicePackets(runtime *voiceRuntime) {
	for {
		select {
		case <-runtime.closeCh:
			return
		case packet, ok := <-runtime.conn.OpusRecv:
			if !ok {
				return
			}
			userID := runtime.speaker(packet.SSRC)
			username := c.lookupVoiceUsername(runtime.guildID, userID)
			out := VoicePacket{
				GuildID:   runtime.guildID,
				ChannelID: runtime.channel,
				UserID:    userID,
				Username:  username,
				SSRC:      packet.SSRC,
				Sequence:  packet.Sequence,
				Timestamp: packet.Timestamp,
				Opus:      append([]byte(nil), packet.Opus...),
			}
			select {
			case runtime.packets <- out:
			case <-runtime.closeCh:
				return
			}
		}
	}
}

func (c *Client) lookupVoiceUsername(guildID string, userID string) string {
	if strings.TrimSpace(userID) == "" {
		return ""
	}
	guild, err := c.guildState(context.Background(), guildID)
	if err != nil {
		return ""
	}
	for _, member := range guild.Members {
		if member == nil || member.User == nil || member.User.ID != userID {
			continue
		}
		return member.User.Username
	}
	return ""
}
