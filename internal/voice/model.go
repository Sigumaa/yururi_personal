package voice

import "time"

type SessionState string

const (
	SessionStateIdle        SessionState = "idle"
	SessionStateJoining     SessionState = "joining"
	SessionStateListening   SessionState = "listening"
	SessionStateThinking    SessionState = "thinking"
	SessionStateSpeaking    SessionState = "speaking"
	SessionStateInterrupted SessionState = "interrupted"
	SessionStateLeaving     SessionState = "leaving"
)

type Session struct {
	ID          string
	GuildID     string
	ChannelID   string
	ChannelName string
	State       SessionState
	StartedAt   time.Time
	EndedAt     *time.Time
	Members     []Member
	Realtime    RealtimeStatus
}

type Member struct {
	UserID           string
	Username         string
	Bot              bool
	ChannelID        string
	Muted            bool
	Deafened         bool
	SelfMuted        bool
	SelfDeafened     bool
	Suppressed       bool
	RequestToSpeakAt *time.Time
}

type RealtimeStatus struct {
	Configured  bool
	Connected   bool
	Model       string
	URL         string
	LastError   string
	ConnectedAt *time.Time
}
