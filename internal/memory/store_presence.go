package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Store) SavePresence(ctx context.Context, snapshot PresenceSnapshot) error {
	activities, err := json.Marshal(snapshot.Activities)
	if err != nil {
		return fmt.Errorf("marshal activities: %w", err)
	}
	var endedAt any
	if snapshot.EndedAt != nil {
		endedAt = snapshot.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO presence_snapshots (user_id, status, activities_json, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?)
	`, snapshot.UserID, snapshot.Status, string(activities), snapshot.StartedAt.UTC().Format(time.RFC3339Nano), endedAt)
	if err != nil {
		return fmt.Errorf("save presence: %w", err)
	}
	return nil
}

func (s *Store) LastPresence(ctx context.Context, userID string) (PresenceSnapshot, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, status, activities_json, started_at, ended_at
		FROM presence_snapshots
		WHERE user_id = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, userID)
	var (
		snapshot  PresenceSnapshot
		rawActs   string
		startedAt string
		endedAt   sql.NullString
	)
	err := row.Scan(&snapshot.ID, &snapshot.UserID, &snapshot.Status, &rawActs, &startedAt, &endedAt)
	if errorsIsNoRows(err) {
		return PresenceSnapshot{}, false, nil
	}
	if err != nil {
		return PresenceSnapshot{}, false, fmt.Errorf("last presence: %w", err)
	}
	snapshot.StartedAt = mustParseTime(startedAt)
	if endedAt.Valid {
		t := mustParseTime(endedAt.String)
		snapshot.EndedAt = &t
	}
	_ = json.Unmarshal([]byte(rawActs), &snapshot.Activities)
	return snapshot, true, nil
}

func (s *Store) SetKV(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO kv (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	if err != nil {
		return fmt.Errorf("set kv: %w", err)
	}
	return nil
}

func (s *Store) GetKV(ctx context.Context, key string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key = ?`, key)
	var value string
	err := row.Scan(&value)
	if errorsIsNoRows(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get kv: %w", err)
	}
	return value, true, nil
}
