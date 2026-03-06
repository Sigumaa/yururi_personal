package jobs

import (
	"context"
	"testing"
	"time"
)

type stubStore struct {
	jobs []Job
}

func (s *stubStore) UpsertJob(context.Context, Job) error { return nil }
func (s *stubStore) GetJob(context.Context, string) (Job, bool, error) {
	return Job{}, false, nil
}
func (s *stubStore) DueJobs(context.Context, time.Time, int) ([]Job, error) {
	return s.jobs, nil
}
func (s *stubStore) UpdateJobState(context.Context, string, State, time.Time, string, *time.Time) error {
	return nil
}

type stubHandler struct {
	called bool
}

func (h *stubHandler) Run(context.Context, Job) (Result, error) {
	h.called = true
	return Result{NextRunAt: time.Now().Add(time.Hour)}, nil
}

func TestSchedulerTickRunsDueJob(t *testing.T) {
	handler := &stubHandler{}
	scheduler := NewScheduler(&stubStore{
		jobs: []Job{
			NewJob("job-1", "watch", "watch", "c1", "1h", nil),
		},
	}, time.Hour)
	scheduler.Register("watch", handler)

	if err := scheduler.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !handler.called {
		t.Fatal("expected handler to be called")
	}
}
