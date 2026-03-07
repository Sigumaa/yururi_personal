package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) UpsertFact(ctx context.Context, fact Fact) error {
	now := time.Now().UTC()
	createdAt := fact.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := fact.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO facts (kind, key, value, source_message_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(kind, key) DO UPDATE SET
			value = excluded.value,
			source_message_id = excluded.source_message_id,
			updated_at = excluded.updated_at
	`, fact.Kind, fact.Key, fact.Value, fact.SourceMessageID, createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert fact: %w", err)
	}
	return nil
}

func (s *Store) SearchFacts(ctx context.Context, query string, limit int) ([]Fact, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, key, value, source_message_id, created_at, updated_at
		FROM facts
		WHERE key LIKE ? OR value LIKE ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search facts: %w", err)
	}
	defer rows.Close()

	var out []Fact
	for rows.Next() {
		var (
			fact      Fact
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&fact.ID, &fact.Kind, &fact.Key, &fact.Value, &fact.SourceMessageID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		fact.CreatedAt = mustParseTime(createdAt)
		fact.UpdatedAt = mustParseTime(updatedAt)
		out = append(out, fact)
	}
	return out, rows.Err()
}

func (s *Store) ListFacts(ctx context.Context, kind string, limit int) ([]Fact, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, kind, key, value, source_message_id, created_at, updated_at
		FROM facts
	`
	args := []any{}
	if strings.TrimSpace(kind) != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list facts: %w", err)
	}
	defer rows.Close()

	var out []Fact
	for rows.Next() {
		var (
			fact      Fact
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&fact.ID, &fact.Kind, &fact.Key, &fact.Value, &fact.SourceMessageID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		fact.CreatedAt = mustParseTime(createdAt)
		fact.UpdatedAt = mustParseTime(updatedAt)
		out = append(out, fact)
	}
	return out, rows.Err()
}

func (s *Store) DeleteFact(ctx context.Context, kind string, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM facts WHERE kind = ? AND key = ?`, kind, key)
	if err != nil {
		return fmt.Errorf("delete fact: %w", err)
	}
	return nil
}
