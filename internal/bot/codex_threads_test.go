package bot

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func TestThreadLockSerializesSameThreadOnly(t *testing.T) {
	app := &App{}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		lock := app.threadLock("thread-main")
		lock.Lock()
		close(firstEntered)
		<-releaseFirst
		lock.Unlock()
		close(firstDone)
	}()

	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first lock holder did not start")
	}

	sameThreadRan := make(chan struct{})
	go func() {
		lock := app.threadLock("thread-main")
		lock.Lock()
		lock.Unlock()
		close(sameThreadRan)
	}()

	select {
	case <-sameThreadRan:
		t.Fatal("same thread lock should block until released")
	case <-time.After(100 * time.Millisecond):
	}

	otherThreadRan := make(chan struct{})
	go func() {
		lock := app.threadLock("thread-background")
		lock.Lock()
		lock.Unlock()
		close(otherThreadRan)
	}()

	select {
	case <-otherThreadRan:
	case <-time.After(time.Second):
		t.Fatal("different thread lock should not be blocked")
	}

	close(releaseFirst)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first lock holder did not finish")
	}
	select {
	case <-sameThreadRan:
	case <-time.After(time.Second):
		t.Fatal("same thread lock should run after release")
	}
}

func TestEnsureJobThreadUsesStoredThreadID(t *testing.T) {
	app := &App{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	job := jobs.Job{
		Payload: map[string]any{
			"thread_id": "thread-background",
		},
	}

	session, err := app.ensureJobThread(context.Background(), job)
	if err != nil {
		t.Fatalf("ensureJobThread: %v", err)
	}
	if session.ID != "thread-background" {
		t.Fatalf("unexpected session id: %s", session.ID)
	}
}
