package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) ensureAutonomyThread(ctx context.Context) (codex.ThreadSession, error) {
	if a.thread.ID != "" {
		return a.thread, nil
	}

	bundle, _, err := loadBotContext(a.paths.WorkspaceContextDir)
	if err != nil {
		return codex.ThreadSession{}, fmt.Errorf("load bot context: %w", err)
	}

	storedID, _, _ := a.store.GetKV(ctx, "codex.main_thread_id")
	session, err := a.codex.EnsureThread(ctx, storedID, baseInstructions(), developerInstructions())
	if err != nil {
		return codex.ThreadSession{}, err
	}
	if err := a.primeThreadContext(ctx, session.ID, bundle); err != nil {
		return codex.ThreadSession{}, fmt.Errorf("prime autonomy thread: %w", err)
	}
	a.thread = session
	if err := a.store.SetKV(ctx, "codex.main_thread_id", session.ID); err != nil {
		return codex.ThreadSession{}, err
	}
	a.logger.Info("autonomy thread ready", "thread_id", session.ID)
	return session, nil
}

func (a *App) ensureChannelThread(ctx context.Context, channelID string) (codex.ThreadSession, error) {
	a.sessionMu.Lock()
	if session, ok := a.channelThreads[channelID]; ok && session.ID != "" {
		a.sessionMu.Unlock()
		a.logger.Debug("channel thread reused", "channel_id", channelID, "thread_id", session.ID)
		return session, nil
	}
	a.sessionMu.Unlock()

	bundle, _, err := loadBotContext(a.paths.WorkspaceContextDir)
	if err != nil {
		return codex.ThreadSession{}, fmt.Errorf("load bot context: %w", err)
	}

	key := "codex.channel_thread." + channelID
	storedID, _, _ := a.store.GetKV(ctx, key)
	session, err := a.codex.EnsureThread(ctx, storedID, baseInstructions(), developerInstructions())
	if err != nil {
		return codex.ThreadSession{}, err
	}
	if err := a.primeThreadContext(ctx, session.ID, bundle); err != nil {
		return codex.ThreadSession{}, fmt.Errorf("prime channel thread: %w", err)
	}
	if err := a.store.SetKV(ctx, key, session.ID); err != nil {
		return codex.ThreadSession{}, err
	}

	a.sessionMu.Lock()
	a.channelThreads[channelID] = session
	a.sessionMu.Unlock()
	a.logger.Info("channel thread ready", "channel_id", channelID, "thread_id", session.ID)
	return session, nil
}

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
	a.logger.Debug("thread lock wait", "thread_id", threadID, "mode", "text", "prompt_preview", previewText(prompt, 800))
	lock.Lock()
	defer lock.Unlock()
	a.logger.Debug("thread lock acquired", "thread_id", threadID, "mode", "text")
	return a.codex.RunTurn(ctx, threadID, prompt)
}

func (a *App) runThreadInputTurn(ctx context.Context, threadID string, input []codex.InputItem) (string, error) {
	lock := a.threadLock(threadID)
	a.logger.Debug("thread lock wait", "thread_id", threadID, "mode", "input", "input_preview", previewJSON(input, 800))
	lock.Lock()
	defer lock.Unlock()
	a.logger.Debug("thread lock acquired", "thread_id", threadID, "mode", "input")
	return a.codex.RunInputTurn(ctx, threadID, input)
}

func (a *App) runThreadJSONTurn(ctx context.Context, threadID string, prompt string, schema map[string]any) (string, error) {
	lock := a.threadLock(threadID)
	a.logger.Debug("thread lock wait", "thread_id", threadID, "mode", "json", "prompt_preview", previewText(prompt, 800), "schema_preview", previewJSON(schema, 600))
	lock.Lock()
	defer lock.Unlock()
	a.logger.Debug("thread lock acquired", "thread_id", threadID, "mode", "json")
	return a.codex.RunJSONTurn(ctx, threadID, prompt, schema)
}

func (a *App) ensureJobThread(ctx context.Context, job jobs.Job) (codex.ThreadSession, error) {
	if threadID, _ := job.Payload["thread_id"].(string); threadID != "" {
		a.logger.Debug("job thread reused", "job_id", job.ID, "thread_id", threadID)
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
