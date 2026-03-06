package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/decision"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

type executionReport struct {
	MemoryWrites []string
	Actions      []string
	Jobs         []string
}

func (r executionReport) HasEffects() bool {
	return len(r.Actions) > 0 || len(r.Jobs) > 0
}

func (r executionReport) Render() string {
	lines := []string{"memory_writes:"}
	if len(r.MemoryWrites) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range r.MemoryWrites {
			lines = append(lines, "- "+item)
		}
	}

	lines = append(lines, "actions:")
	if len(r.Actions) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range r.Actions {
			lines = append(lines, "- "+item)
		}
	}

	lines = append(lines, "jobs:")
	if len(r.Jobs) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, item := range r.Jobs {
			lines = append(lines, "- "+item)
		}
	}
	return strings.Join(lines, "\n")
}

var promiseReplyRe = regexp.MustCompile(`(?:やります|進めます|見ておきます|確認します|調べます|対応します|待っていて|待ってて|できたら|終わったら|あとで|順に手をつけます)`)

func parseDecisionPlan(raw string) (decision.ReplyDecision, error) {
	var planned decision.ReplyDecision
	if err := json.Unmarshal([]byte(raw), &planned); err != nil {
		return decision.ReplyDecision{}, fmt.Errorf("parse decision plan: %w", err)
	}
	return planned, nil
}

func looksLikePromiseOnly(message string) bool {
	return promiseReplyRe.MatchString(strings.TrimSpace(message))
}

func (a *App) composeDecisionReply(ctx context.Context, msg memory.Message, planned decision.ReplyDecision, report executionReport) (string, error) {
	draft := strings.TrimSpace(planned.Message)
	switch {
	case draft == "" && !report.HasEffects():
		return "", nil
	case draft == "" && report.HasEffects():
		return a.composeReplyFromExecution(ctx, msg, planned, report, "")
	case looksLikePromiseOnly(draft):
		return a.composeReplyFromExecution(ctx, msg, planned, report, draft)
	default:
		return draft, nil
	}
}

func (a *App) composeReplyFromExecution(ctx context.Context, msg memory.Message, planned decision.ReplyDecision, report executionReport, draft string) (string, error) {
	if a.thread.ID == "" {
		return "", nil
	}

	prompt := buildExecutionReplyPrompt(msg, planned, report, draft)
	raw, err := a.runThreadTurn(ctx, a.thread.ID, prompt)
	if err != nil {
		return "", fmt.Errorf("compose execution reply: %w", err)
	}
	reply := parseAssistantReply(raw)
	if reply.Action == decision.ActionIgnore {
		return "", nil
	}
	if looksLikePromiseOnly(reply.Message) {
		return "", nil
	}
	return strings.TrimSpace(reply.Message), nil
}

func (a *App) handleJobResult(job jobs.Job, result jobs.Result, runErr error) {
	if a.thread.ID == "" || a.discord == nil || strings.TrimSpace(job.ChannelID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	prompt := buildJobFollowUpPrompt(job, result, runErr)
	raw, err := a.runThreadTurn(ctx, a.thread.ID, prompt)
	if err != nil {
		a.logger.Warn("job follow-up turn failed", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}
	reply := parseAssistantReply(raw)
	if reply.Action == decision.ActionIgnore || strings.TrimSpace(reply.Message) == "" {
		return
	}
	if looksLikePromiseOnly(reply.Message) {
		a.logger.Warn("job follow-up suppressed promise-only reply", "job_id", job.ID, "kind", job.Kind, "reply", reply.Message)
		return
	}
	sentID, err := a.discord.SendMessage(ctx, job.ChannelID, strings.TrimSpace(reply.Message))
	if err != nil {
		a.logger.Warn("job follow-up send failed", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}
	a.logger.Info("job follow-up sent", "job_id", job.ID, "kind", job.Kind, "message_id", sentID)
}

func (a *App) handleBackgroundCodexTaskJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.thread.ID == "" {
		return jobs.Result{Done: true}, fmt.Errorf("codex thread is unavailable")
	}

	prompt, _ := job.Payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return jobs.Result{Done: true}, fmt.Errorf("payload.prompt is required")
	}

	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{Done: true}, err
	}

	a.logger.Info("background codex task start", "job_id", job.ID, "title", job.Title, "thread_id", session.ID)
	raw, err := a.runThreadTurn(ctx, session.ID, prompt)
	if err != nil {
		return jobs.Result{Done: true}, err
	}
	a.logger.Info("background codex task completed", "job_id", job.ID, "thread_id", session.ID, "response_bytes", len(raw))
	return jobs.Result{
		NextRunAt: time.Now().UTC(),
		Done:      true,
		Details:   strings.TrimSpace(raw),
	}, nil
}
