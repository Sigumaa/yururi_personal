package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) registerJobAutonomyTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_review",
		Description: "open loop、curiosity、initiative、soft reminder、topic synthesis、baseline、policy synthesis、workspace、proposal boundary、decision、self improvement、channel role、channel curation の review job を作る",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "open_loop_review, curiosity_review, initiative_review, soft_reminder_review, topic_synthesis_review, baseline_review, policy_synthesis_review, workspace_review, proposal_boundary_review, decision_review, self_improvement_review, channel_role_review, channel_curation"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration。省略時は kind ごとの既定値"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind      string `json:"kind"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		switch input.Kind {
		case "open_loop_review":
			if input.Schedule == "" {
				input.Schedule = "72h"
			}
		case "curiosity_review":
			if input.Schedule == "" {
				input.Schedule = "24h"
			}
		case "initiative_review":
			if input.Schedule == "" {
				input.Schedule = "48h"
			}
		case "soft_reminder_review":
			if input.Schedule == "" {
				input.Schedule = "24h"
			}
		case "topic_synthesis_review":
			if input.Schedule == "" {
				input.Schedule = "72h"
			}
		case "baseline_review":
			if input.Schedule == "" {
				input.Schedule = "72h"
			}
		case "policy_synthesis_review":
			if input.Schedule == "" {
				input.Schedule = "96h"
			}
		case "workspace_review":
			if input.Schedule == "" {
				input.Schedule = "48h"
			}
		case "proposal_boundary_review":
			if input.Schedule == "" {
				input.Schedule = "96h"
			}
		case "channel_curation":
			if input.Schedule == "" {
				input.Schedule = "168h"
			}
		case "decision_review":
			if input.Schedule == "" {
				input.Schedule = "120h"
			}
		case "self_improvement_review":
			if input.Schedule == "" {
				input.Schedule = "168h"
			}
		case "channel_role_review":
			if input.Schedule == "" {
				input.Schedule = "168h"
			}
		default:
			return codex.ToolResponse{}, errors.New("unsupported review kind")
		}
		job := jobs.NewJob(jobID(strings.ReplaceAll(input.Kind, "_", "-")), input.Kind, input.Kind, input.ChannelID, input.Schedule, nil)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})
}
