package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) registerJobStateTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.get",
		Description: "単一 job の状態を見る",
		InputSchema: objectSchema(fieldSchema("id", "string", "job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		job, ok, err := a.store.GetJob(ctx, input.ID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return textTool("job not found"), nil
		}
		return textTool(formatJob(job)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.run_now",
		Description: "既存 job の次回実行を今に寄せる",
		InputSchema: objectSchema(fieldSchema("id", "string", "job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		job, ok, err := a.store.GetJob(ctx, input.ID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return codex.ToolResponse{}, errors.New("job not found")
		}
		now := time.Now().UTC()
		if err := a.store.UpdateJobState(ctx, job.ID, jobs.StatePending, now, "", job.LastRunAt); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("scheduled for immediate run"), nil
	})
}

func formatJob(job jobs.Job) string {
	return fmt.Sprintf("id=%s kind=%s state=%s channel=%s schedule=%s next=%s last_error=%s payload=%s",
		job.ID,
		job.Kind,
		job.State,
		job.ChannelID,
		job.ScheduleExpr,
		job.NextRunAt.Format(time.RFC3339),
		job.LastError,
		formatMap(job.Payload),
	)
}
