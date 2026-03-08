package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *Store) UpsertVoiceSession(ctx context.Context, session VoiceSession) error {
	metadata, err := json.Marshal(emptyMap(session.Metadata))
	if err != nil {
		return fmt.Errorf("marshal voice session metadata: %w", err)
	}
	var endedAt any
	if session.EndedAt != nil {
		endedAt = session.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO voice_sessions (id, guild_id, channel_id, channel_name, state, source, started_at, ended_at, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			guild_id = excluded.guild_id,
			channel_id = excluded.channel_id,
			channel_name = excluded.channel_name,
			state = excluded.state,
			source = excluded.source,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at,
			metadata_json = excluded.metadata_json
	`, session.ID, session.GuildID, session.ChannelID, session.ChannelName, session.State, session.Source, session.StartedAt.UTC().Format(time.RFC3339Nano), endedAt, string(metadata))
	if err != nil {
		return fmt.Errorf("upsert voice session: %w", err)
	}
	return nil
}

func (s *Store) EndVoiceSession(ctx context.Context, sessionID string, endedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE voice_sessions
		SET ended_at = ?, state = ?
		WHERE id = ?
	`, endedAt.UTC().Format(time.RFC3339Nano), "ended", sessionID)
	if err != nil {
		return fmt.Errorf("end voice session: %w", err)
	}
	return nil
}

func (s *Store) ActiveVoiceSession(ctx context.Context, guildID string) (VoiceSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, guild_id, channel_id, channel_name, state, source, started_at, ended_at, metadata_json
		FROM voice_sessions
		WHERE guild_id = ? AND ended_at IS NULL
		ORDER BY started_at DESC
		LIMIT 1
	`, guildID)
	session, err := scanVoiceSession(row)
	if errorsIsNoRows(err) {
		return VoiceSession{}, false, nil
	}
	if err != nil {
		return VoiceSession{}, false, err
	}
	return session, true, nil
}

func (s *Store) RecentVoiceSessions(ctx context.Context, limit int) ([]VoiceSession, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, guild_id, channel_id, channel_name, state, source, started_at, ended_at, metadata_json
		FROM voice_sessions
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent voice sessions: %w", err)
	}
	defer rows.Close()
	var out []VoiceSession
	for rows.Next() {
		item, err := scanVoiceSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveVoiceTranscript(ctx context.Context, segment VoiceTranscriptSegment) error {
	metadata, err := json.Marshal(emptyMap(segment.Metadata))
	if err != nil {
		return fmt.Errorf("marshal voice transcript metadata: %w", err)
	}
	var endedAt any
	if segment.EndedAt != nil {
		endedAt = segment.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO voice_transcripts (session_id, speaker_id, speaker_name, role, content, started_at, ended_at, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, segment.SessionID, segment.SpeakerID, segment.SpeakerName, segment.Role, segment.Content, segment.StartedAt.UTC().Format(time.RFC3339Nano), endedAt, string(metadata))
	if err != nil {
		return fmt.Errorf("save voice transcript: %w", err)
	}
	return nil
}

func (s *Store) ListVoiceTranscripts(ctx context.Context, sessionID string, limit int) ([]VoiceTranscriptSegment, error) {
	if limit <= 0 {
		limit = 32
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, speaker_id, speaker_name, role, content, started_at, ended_at, metadata_json
		FROM voice_transcripts
		WHERE session_id = ?
		ORDER BY started_at ASC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list voice transcripts: %w", err)
	}
	defer rows.Close()
	var out []VoiceTranscriptSegment
	for rows.Next() {
		item, err := scanVoiceTranscript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RecentVoiceTranscripts(ctx context.Context, limit int) ([]VoiceTranscriptSegment, error) {
	if limit <= 0 {
		limit = 32
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, speaker_id, speaker_name, role, content, started_at, ended_at, metadata_json
		FROM voice_transcripts
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent voice transcripts: %w", err)
	}
	defer rows.Close()
	var out []VoiceTranscriptSegment
	for rows.Next() {
		item, err := scanVoiceTranscript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SearchVoiceTranscripts(ctx context.Context, query string, limit int) ([]VoiceTranscriptSegment, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, speaker_id, speaker_name, role, content, started_at, ended_at, metadata_json
		FROM voice_transcripts
		WHERE content LIKE ?
		ORDER BY started_at DESC
		LIMIT ?
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search voice transcripts: %w", err)
	}
	defer rows.Close()
	var out []VoiceTranscriptSegment
	for rows.Next() {
		item, err := scanVoiceTranscript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveVoiceEvent(ctx context.Context, event VoiceEvent) error {
	metadata, err := json.Marshal(emptyMap(event.Metadata))
	if err != nil {
		return fmt.Errorf("marshal voice event metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO voice_events (session_id, type, user_id, channel_id, created_at, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, event.SessionID, event.Type, event.UserID, event.ChannelID, event.CreatedAt.UTC().Format(time.RFC3339Nano), string(metadata))
	if err != nil {
		return fmt.Errorf("save voice event: %w", err)
	}
	return nil
}

func (s *Store) ListVoiceEvents(ctx context.Context, sessionID string, limit int) ([]VoiceEvent, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, type, user_id, channel_id, created_at, metadata_json
		FROM voice_events
		WHERE session_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list voice events: %w", err)
	}
	defer rows.Close()
	var out []VoiceEvent
	for rows.Next() {
		item, err := scanVoiceEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanVoiceSession(row scanner) (VoiceSession, error) {
	var (
		item      VoiceSession
		startedAt string
		endedAt   sql.NullString
		rawMeta   string
	)
	if err := row.Scan(&item.ID, &item.GuildID, &item.ChannelID, &item.ChannelName, &item.State, &item.Source, &startedAt, &endedAt, &rawMeta); err != nil {
		return VoiceSession{}, err
	}
	item.StartedAt = mustParseTime(startedAt)
	if endedAt.Valid {
		ended := mustParseTime(endedAt.String)
		item.EndedAt = &ended
	}
	_ = json.Unmarshal([]byte(rawMeta), &item.Metadata)
	return item, nil
}

func scanVoiceTranscript(row scanner) (VoiceTranscriptSegment, error) {
	var (
		item      VoiceTranscriptSegment
		startedAt string
		endedAt   sql.NullString
		rawMeta   string
	)
	if err := row.Scan(&item.ID, &item.SessionID, &item.SpeakerID, &item.SpeakerName, &item.Role, &item.Content, &startedAt, &endedAt, &rawMeta); err != nil {
		return VoiceTranscriptSegment{}, err
	}
	item.StartedAt = mustParseTime(startedAt)
	if endedAt.Valid {
		ended := mustParseTime(endedAt.String)
		item.EndedAt = &ended
	}
	_ = json.Unmarshal([]byte(rawMeta), &item.Metadata)
	return item, nil
}

func scanVoiceEvent(row scanner) (VoiceEvent, error) {
	var (
		item      VoiceEvent
		createdAt string
		rawMeta   string
	)
	if err := row.Scan(&item.ID, &item.SessionID, &item.Type, &item.UserID, &item.ChannelID, &createdAt, &rawMeta); err != nil {
		return VoiceEvent{}, err
	}
	item.CreatedAt = mustParseTime(createdAt)
	_ = json.Unmarshal([]byte(rawMeta), &item.Metadata)
	return item, nil
}
