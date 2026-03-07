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
)

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
