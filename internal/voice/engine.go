package voice

import (
	"context"
	"log/slog"
	"sync"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

type discordService interface {
	GetChannel(context.Context, string) (discordsvc.Channel, error)
	JoinVoice(context.Context, string, string, bool, bool) (discordsvc.VoiceSession, error)
	LeaveVoice(context.Context, string) error
	CurrentVoiceSession(context.Context, string) (discordsvc.VoiceSession, bool, error)
	CurrentMemberVoiceState(context.Context, string, string) (discordsvc.VoiceState, bool, error)
	VoiceChannelMembers(context.Context, string, string) ([]discordsvc.VoiceMember, error)
	VoiceAudioPackets(context.Context, string) (<-chan discordsvc.VoicePacket, error)
	SendVoiceOpus(context.Context, string, []byte) error
	SetVoiceSpeaking(context.Context, string, bool) error
	SelfUserID() string
}

type Engine struct {
	logger      *slog.Logger
	store       *memory.Store
	discord     discordService
	realtime    RealtimeClient
	ownerUserID string

	mu       sync.RWMutex
	sessions map[string]*runtimeSession
}

func NewEngine(store *memory.Store, discord discordService, realtime RealtimeClient, ownerUserID string, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		logger:      logger,
		store:       store,
		discord:     discord,
		realtime:    realtime,
		ownerUserID: ownerUserID,
		sessions:    map[string]*runtimeSession{},
	}
}

func membersFromDiscord(items []discordsvc.VoiceMember) []Member {
	out := make([]Member, 0, len(items))
	for _, item := range items {
		out = append(out, Member{
			UserID:           item.UserID,
			Username:         item.Username,
			Bot:              item.Bot,
			ChannelID:        item.ChannelID,
			Muted:            item.Muted,
			Deafened:         item.Deafened,
			SelfMuted:        item.SelfMuted,
			SelfDeafened:     item.SelfDeafened,
			Suppressed:       item.Suppressed,
			RequestToSpeakAt: item.RequestToSpeakAt,
		})
	}
	return out
}

func statusOf(client RealtimeClient) RealtimeStatus {
	if client == nil {
		return RealtimeStatus{}
	}
	return client.Status()
}
