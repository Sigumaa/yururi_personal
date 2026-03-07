package memory

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) SaveSummary(ctx context.Context, summary Summary) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO summaries (period, channel_id, content, starts_at, ends_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, summary.Period, summary.ChannelID, summary.Content, summary.StartsAt.UTC().Format(time.RFC3339Nano), summary.EndsAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save summary: %w", err)
	}
	return nil
}

func (s *Store) RecentSummaries(ctx context.Context, period string, limit int) ([]Summary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, period, channel_id, content, starts_at, ends_at, created_at
		FROM summaries
		WHERE period = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, period, limit)
	if err != nil {
		return nil, fmt.Errorf("recent summaries: %w", err)
	}
	defer rows.Close()

	var out []Summary
	for rows.Next() {
		var (
			summary          Summary
			startsAt, endsAt string
			createdAt        string
		)
		if err := rows.Scan(&summary.ID, &summary.Period, &summary.ChannelID, &summary.Content, &startsAt, &endsAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		summary.StartsAt = mustParseTime(startsAt)
		summary.EndsAt = mustParseTime(endsAt)
		summary.CreatedAt = mustParseTime(createdAt)
		out = append(out, summary)
	}
	return out, rows.Err()
}
