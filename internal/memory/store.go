package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	presencemodel "github.com/Sigumaa/yururi_personal/internal/presence"
	_ "modernc.org/sqlite"
)

type Store struct {
	db         *sql.DB
	ftsEnabled bool
}

type Message struct {
	ID          string
	ChannelID   string
	ChannelName string
	AuthorID    string
	AuthorName  string
	Content     string
	CreatedAt   time.Time
	Metadata    map[string]any
}

type Fact struct {
	ID              int64
	Kind            string
	Key             string
	Value           string
	SourceMessageID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ChannelProfile struct {
	ChannelID           string
	Name                string
	Kind                string
	ReplyAggressiveness float64
	AutonomyLevel       float64
	SummaryCadence      string
	Metadata            map[string]any
}

type ChannelActivity struct {
	ChannelID     string
	ChannelName   string
	MessageCount  int
	LastMessageAt time.Time
}

type Summary struct {
	ID        int64
	Period    string
	ChannelID string
	Content   string
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedAt time.Time
}

type PresenceSnapshot struct {
	ID         int64
	UserID     string
	Status     string
	Activities []presencemodel.Activity
	StartedAt  time.Time
	EndedAt    *time.Time
}

type VoiceSession struct {
	ID          string
	GuildID     string
	ChannelID   string
	ChannelName string
	State       string
	Source      string
	StartedAt   time.Time
	EndedAt     *time.Time
	Metadata    map[string]any
}

type VoiceTranscriptSegment struct {
	ID          int64
	SessionID   string
	SpeakerID   string
	SpeakerName string
	Role        string
	Content     string
	StartedAt   time.Time
	EndedAt     *time.Time
	Metadata    map[string]any
}

type VoiceEvent struct {
	ID        int64
	SessionID string
	Type      string
	UserID    string
	ChannelID string
	CreatedAt time.Time
	Metadata  map[string]any
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
