package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS raw_messages (
			id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			channel_name TEXT NOT NULL DEFAULT '',
			author_id TEXT NOT NULL,
			author_name TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE TABLE IF NOT EXISTS facts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			source_message_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(kind, key)
		);`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			title TEXT NOT NULL,
			state TEXT NOT NULL,
			channel_id TEXT NOT NULL DEFAULT '',
			schedule_expr TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL DEFAULT '{}',
			next_run_at TEXT NOT NULL,
			last_run_at TEXT,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS summaries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			period TEXT NOT NULL,
			channel_id TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			starts_at TEXT NOT NULL,
			ends_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS channel_profiles (
			channel_id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			reply_aggressiveness REAL NOT NULL,
			autonomy_level REAL NOT NULL,
			summary_cadence TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE TABLE IF NOT EXISTS presence_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			status TEXT NOT NULL,
			activities_json TEXT NOT NULL DEFAULT '[]',
			started_at TEXT NOT NULL,
			ended_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}

	if _, err := s.db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS raw_messages_fts USING fts5(message_id UNINDEXED, content, tokenize = 'unicode61');`); err == nil {
		s.ftsEnabled = true
		if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER IF NOT EXISTS raw_messages_ai AFTER INSERT ON raw_messages BEGIN INSERT INTO raw_messages_fts(message_id, content) VALUES (new.id, new.content); END;`); err != nil {
			return fmt.Errorf("create fts trigger: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER IF NOT EXISTS raw_messages_ad AFTER DELETE ON raw_messages BEGIN DELETE FROM raw_messages_fts WHERE message_id = old.id; END;`); err != nil {
			return fmt.Errorf("create fts delete trigger: %w", err)
		}
	}
	return nil
}

func emptyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func mustParseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(rows *sql.Rows) (Message, error) {
	var (
		msg     Message
		rawMeta string
		created string
	)
	if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.ChannelName, &msg.AuthorID, &msg.AuthorName, &msg.Content, &created, &rawMeta); err != nil {
		return Message{}, fmt.Errorf("scan message: %w", err)
	}
	msg.CreatedAt = mustParseTime(created)
	_ = json.Unmarshal([]byte(rawMeta), &msg.Metadata)
	return msg, nil
}
