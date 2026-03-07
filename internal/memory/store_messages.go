package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *Store) SaveMessage(ctx context.Context, msg Message) error {
	meta, err := json.Marshal(emptyMap(msg.Metadata))
	if err != nil {
		return fmt.Errorf("marshal message metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO raw_messages
		(id, channel_id, channel_name, author_id, author_name, content, created_at, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, msg.ChannelID, msg.ChannelName, msg.AuthorID, msg.AuthorName, msg.Content, msg.CreatedAt.UTC().Format(time.RFC3339Nano), string(meta))
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

func (s *Store) RecentMessages(ctx context.Context, channelID string, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, channel_id, channel_name, author_id, author_name, content, created_at, metadata_json
		FROM raw_messages
		WHERE channel_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *Store) LatestChannelIDForAuthor(ctx context.Context, authorID string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT channel_id
		FROM raw_messages
		WHERE author_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, authorID)

	var channelID string
	if err := row.Scan(&channelID); errorsIsNoRows(err) {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("latest channel for author: %w", err)
	}
	return channelID, true, nil
}

func (s *Store) RecentMessagesByAuthor(ctx context.Context, authorID string, channelID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, channel_id, channel_name, author_id, author_name, content, created_at, metadata_json
		FROM raw_messages
		WHERE author_id = ?
	`
	args := []any{authorID}
	if strings.TrimSpace(channelID) != "" {
		query += ` AND channel_id = ?`
		args = append(args, channelID)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("recent messages by author: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *Store) SearchMessages(ctx context.Context, query string, limit int) ([]Message, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var (
		rows *sql.Rows
		err  error
	)
	if s.ftsEnabled {
		rows, err = s.db.QueryContext(ctx, `
			SELECT m.id, m.channel_id, m.channel_name, m.author_id, m.author_name, m.content, m.created_at, m.metadata_json
			FROM raw_messages m
			JOIN raw_messages_fts f ON f.message_id = m.id
			WHERE raw_messages_fts MATCH ?
			ORDER BY m.created_at DESC
			LIMIT ?
		`, query, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, channel_id, channel_name, author_id, author_name, content, created_at, metadata_json
			FROM raw_messages
			WHERE content LIKE ?
			ORDER BY created_at DESC
			LIMIT ?
		`, "%"+query+"%", limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *Store) MessagesBetween(ctx context.Context, start time.Time, end time.Time, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, channel_id, channel_name, author_id, author_name, content, created_at, metadata_json
		FROM raw_messages
		WHERE created_at >= ? AND created_at < ?
		ORDER BY created_at ASC
		LIMIT ?
	`, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("messages between: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}
