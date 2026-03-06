package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
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
	Activities []string
	StartedAt  time.Time
	EndedAt    *time.Time
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

	var rows *sql.Rows
	var err error
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

func (s *Store) UpsertChannelProfile(ctx context.Context, profile ChannelProfile) error {
	meta, err := json.Marshal(emptyMap(profile.Metadata))
	if err != nil {
		return fmt.Errorf("marshal profile metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO channel_profiles
		(channel_id, name, kind, reply_aggressiveness, autonomy_level, summary_cadence, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			reply_aggressiveness = excluded.reply_aggressiveness,
			autonomy_level = excluded.autonomy_level,
			summary_cadence = excluded.summary_cadence,
			metadata_json = excluded.metadata_json
	`, profile.ChannelID, profile.Name, profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel, profile.SummaryCadence, string(meta))
	if err != nil {
		return fmt.Errorf("upsert channel profile: %w", err)
	}
	return nil
}

func (s *Store) GetChannelProfile(ctx context.Context, channelID string) (ChannelProfile, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT channel_id, name, kind, reply_aggressiveness, autonomy_level, summary_cadence, metadata_json
		FROM channel_profiles
		WHERE channel_id = ?
	`, channelID)
	var (
		profile ChannelProfile
		rawMeta string
	)
	err := row.Scan(&profile.ChannelID, &profile.Name, &profile.Kind, &profile.ReplyAggressiveness, &profile.AutonomyLevel, &profile.SummaryCadence, &rawMeta)
	if errorsIsNoRows(err) {
		return ChannelProfile{}, false, nil
	}
	if err != nil {
		return ChannelProfile{}, false, fmt.Errorf("get channel profile: %w", err)
	}
	_ = json.Unmarshal([]byte(rawMeta), &profile.Metadata)
	return profile, true, nil
}

func (s *Store) ListChannelProfiles(ctx context.Context) ([]ChannelProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT channel_id, name, kind, reply_aggressiveness, autonomy_level, summary_cadence, metadata_json
		FROM channel_profiles
		ORDER BY name ASC, channel_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list channel profiles: %w", err)
	}
	defer rows.Close()

	var out []ChannelProfile
	for rows.Next() {
		var (
			profile ChannelProfile
			rawMeta string
		)
		if err := rows.Scan(&profile.ChannelID, &profile.Name, &profile.Kind, &profile.ReplyAggressiveness, &profile.AutonomyLevel, &profile.SummaryCadence, &rawMeta); err != nil {
			return nil, fmt.Errorf("scan channel profile: %w", err)
		}
		_ = json.Unmarshal([]byte(rawMeta), &profile.Metadata)
		out = append(out, profile)
	}
	return out, rows.Err()
}

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

func (s *Store) ChannelActivitySince(ctx context.Context, start time.Time, limit int) ([]ChannelActivity, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT channel_id, channel_name, COUNT(*) AS message_count, MAX(created_at) AS last_message_at
		FROM raw_messages
		WHERE created_at >= ?
		GROUP BY channel_id, channel_name
		ORDER BY message_count DESC, last_message_at DESC
		LIMIT ?
	`, start.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("channel activity since: %w", err)
	}
	defer rows.Close()

	var out []ChannelActivity
	for rows.Next() {
		var (
			activity    ChannelActivity
			lastMessage string
		)
		if err := rows.Scan(&activity.ChannelID, &activity.ChannelName, &activity.MessageCount, &lastMessage); err != nil {
			return nil, fmt.Errorf("scan channel activity: %w", err)
		}
		activity.LastMessageAt = mustParseTime(lastMessage)
		out = append(out, activity)
	}
	return out, rows.Err()
}

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

func (s *Store) UpsertJob(ctx context.Context, job jobs.Job) error {
	payload, err := json.Marshal(emptyMap(job.Payload))
	if err != nil {
		return fmt.Errorf("marshal job payload: %w", err)
	}
	now := time.Now().UTC()
	createdAt := job.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := job.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	var lastRun any
	if job.LastRunAt != nil {
		lastRun = job.LastRunAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO jobs
		(id, kind, title, state, channel_id, schedule_expr, payload_json, next_run_at, last_run_at, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			title = excluded.title,
			state = excluded.state,
			channel_id = excluded.channel_id,
			schedule_expr = excluded.schedule_expr,
			payload_json = excluded.payload_json,
			next_run_at = excluded.next_run_at,
			last_run_at = excluded.last_run_at,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at
	`, job.ID, job.Kind, job.Title, string(job.State), job.ChannelID, job.ScheduleExpr, string(payload), job.NextRunAt.UTC().Format(time.RFC3339Nano), lastRun, job.LastError, createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert job: %w", err)
	}
	return nil
}

func (s *Store) DueJobs(ctx context.Context, now time.Time, limit int) ([]jobs.Job, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, title, state, channel_id, schedule_expr, payload_json, next_run_at, last_run_at, last_error, created_at, updated_at
		FROM jobs
		WHERE state IN ('pending', 'failed', 'running')
		  AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT ?
	`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("query due jobs: %w", err)
	}
	defer rows.Close()

	var out []jobs.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *Store) UpdateJobState(ctx context.Context, id string, state jobs.State, nextRun time.Time, lastError string, lastRunAt *time.Time) error {
	var lastRun any
	if lastRunAt != nil {
		lastRun = lastRunAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET state = ?, next_run_at = ?, last_error = ?, last_run_at = ?, updated_at = ?
		WHERE id = ?
	`, string(state), nextRun.UTC().Format(time.RFC3339Nano), lastError, lastRun, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update job state: %w", err)
	}
	return nil
}

func (s *Store) GetJob(ctx context.Context, id string) (jobs.Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, title, state, channel_id, schedule_expr, payload_json, next_run_at, last_run_at, last_error, created_at, updated_at
		FROM jobs
		WHERE id = ?
	`, id)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, false, nil
	}
	if err != nil {
		return jobs.Job{}, false, err
	}
	return job, true, nil
}

func (s *Store) ListJobs(ctx context.Context, kind string, state jobs.State, channelID string, limit int) ([]jobs.Job, error) {
	if limit <= 0 {
		limit = 32
	}

	query := `
		SELECT id, kind, title, state, channel_id, schedule_expr, payload_json, next_run_at, last_run_at, last_error, created_at, updated_at
		FROM jobs
		WHERE 1 = 1
	`
	args := []any{}
	if strings.TrimSpace(kind) != "" {
		query += ` AND kind = ?`
		args = append(args, kind)
	}
	if strings.TrimSpace(string(state)) != "" {
		query += ` AND state = ?`
		args = append(args, string(state))
	}
	if strings.TrimSpace(channelID) != "" {
		query += ` AND channel_id = ?`
		args = append(args, channelID)
	}
	query += ` ORDER BY next_run_at ASC, created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var out []jobs.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
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

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(row scanner) (jobs.Job, error) {
	var (
		job                             jobs.Job
		payloadJSON                     string
		nextRunAt, createdAt, updatedAt string
		lastRunAt                       sql.NullString
		state                           string
	)
	if err := row.Scan(&job.ID, &job.Kind, &job.Title, &state, &job.ChannelID, &job.ScheduleExpr, &payloadJSON, &nextRunAt, &lastRunAt, &job.LastError, &createdAt, &updatedAt); err != nil {
		return jobs.Job{}, err
	}
	job.State = jobs.State(state)
	job.NextRunAt = mustParseTime(nextRunAt)
	job.CreatedAt = mustParseTime(createdAt)
	job.UpdatedAt = mustParseTime(updatedAt)
	if lastRunAt.Valid {
		t := mustParseTime(lastRunAt.String)
		job.LastRunAt = &t
	}
	_ = json.Unmarshal([]byte(payloadJSON), &job.Payload)
	return job, nil
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
	return err == sql.ErrNoRows
}
