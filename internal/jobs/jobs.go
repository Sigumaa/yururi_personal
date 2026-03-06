package jobs

import (
	"context"
	"time"
)

type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

type Job struct {
	ID           string
	Kind         string
	Title        string
	State        State
	ChannelID    string
	ScheduleExpr string
	Payload      map[string]any
	NextRunAt    time.Time
	LastRunAt    *time.Time
	LastError    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Store interface {
	UpsertJob(context.Context, Job) error
	DueJobs(context.Context, time.Time, int) ([]Job, error)
	UpdateJobState(context.Context, string, State, time.Time, string, *time.Time) error
	GetJob(context.Context, string) (Job, bool, error)
}

type Scheduler struct {
	store    Store
	interval time.Duration
	handlers map[string]Handler
}

type Handler interface {
	Run(context.Context, Job) (Result, error)
}

type Result struct {
	NextRunAt time.Time
	Done      bool
}

func NewScheduler(store Store, interval time.Duration) *Scheduler {
	return &Scheduler{
		store:    store,
		interval: interval,
		handlers: map[string]Handler{},
	}
}

func (s *Scheduler) Register(kind string, handler Handler) {
	s.handlers[kind] = handler
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		if err := s.tick(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) error {
	now := time.Now().UTC()
	due, err := s.store.DueJobs(ctx, now, 16)
	if err != nil {
		return err
	}
	for _, job := range due {
		handler, ok := s.handlers[job.Kind]
		if !ok {
			nextRun := now.Add(30 * time.Minute)
			if err := s.store.UpdateJobState(ctx, job.ID, StateFailed, nextRun, "missing handler", nil); err != nil {
				return err
			}
			continue
		}
		lastRunAt := now
		if err := s.store.UpdateJobState(ctx, job.ID, StateRunning, now, "", &lastRunAt); err != nil {
			return err
		}
		result, err := handler.Run(ctx, job)
		if err != nil {
			nextRun := nextRun(job, now)
			if err := s.store.UpdateJobState(ctx, job.ID, StateFailed, nextRun, err.Error(), &lastRunAt); err != nil {
				return err
			}
			continue
		}
		state := StatePending
		if result.Done {
			state = StateCompleted
		}
		if err := s.store.UpdateJobState(ctx, job.ID, state, result.NextRunAt, "", &lastRunAt); err != nil {
			return err
		}
	}
	return nil
}

func nextRun(job Job, now time.Time) time.Time {
	duration, err := time.ParseDuration(job.ScheduleExpr)
	if err != nil || duration <= 0 {
		return now.Add(1 * time.Hour)
	}
	return now.Add(duration)
}

func NewJob(id string, kind string, title string, channelID string, schedule string, payload map[string]any) Job {
	now := time.Now().UTC()
	return Job{
		ID:           id,
		Kind:         kind,
		Title:        title,
		State:        StatePending,
		ChannelID:    channelID,
		ScheduleExpr: schedule,
		Payload:      payload,
		NextRunAt:    now.Add(mustDuration(schedule)),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func mustDuration(value string) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return time.Hour
	}
	return d
}
