package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) threadLock(threadID string) *sync.Mutex {
	a.threadMu.Lock()
	defer a.threadMu.Unlock()

	if a.threadLocks == nil {
		a.threadLocks = map[string]*sync.Mutex{}
	}
	lock, ok := a.threadLocks[threadID]
	if !ok {
		lock = &sync.Mutex{}
		a.threadLocks[threadID] = lock
	}
	return lock
}

func (a *App) runThreadTurn(ctx context.Context, threadID string, prompt string) (string, error) {
	lock := a.threadLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	return a.codex.RunTurn(ctx, threadID, prompt)
}

func (a *App) runThreadJSONTurn(ctx context.Context, threadID string, prompt string, schema map[string]any) (string, error) {
	lock := a.threadLock(threadID)
	lock.Lock()
	defer lock.Unlock()
	return a.codex.RunJSONTurn(ctx, threadID, prompt, schema)
}

func (a *App) ensureJobThread(ctx context.Context, job jobs.Job) (codex.ThreadSession, error) {
	if threadID, _ := job.Payload["thread_id"].(string); threadID != "" {
		return codex.ThreadSession{ID: threadID}, nil
	}

	bundle, _, err := loadBotContext(a.paths.WorkspaceContextDir)
	if err != nil {
		return codex.ThreadSession{}, fmt.Errorf("load bot context: %w", err)
	}

	session, err := a.codex.EnsureThread(ctx, "", baseInstructions(), developerInstructions())
	if err != nil {
		return codex.ThreadSession{}, err
	}
	if err := a.primeThreadContext(ctx, session.ID, bundle); err != nil {
		return codex.ThreadSession{}, fmt.Errorf("prime job thread: %w", err)
	}
	if job.Payload == nil {
		job.Payload = map[string]any{}
	}
	job.Payload["thread_id"] = session.ID
	job.UpdatedAt = time.Now().UTC()
	if err := a.store.UpsertJob(ctx, job); err != nil {
		return codex.ThreadSession{}, fmt.Errorf("persist job thread: %w", err)
	}
	a.logger.Info("job thread ready", "job_id", job.ID, "thread_id", session.ID)
	return session, nil
}
