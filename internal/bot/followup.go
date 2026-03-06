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

var promiseReplyRe = regexp.MustCompile(`(?:やります|進めます|見ておきます|確認します|調べます|対応します|待っていて|待ってて|できたら|終わったら|あとで|順に手をつけます|返します|共有します)`)
var promiseFillerRe = regexp.MustCompile(`^(?:はい|了解です|承知しました|わかりました|もちろん|うん|では|それでは|いまから|今から|すぐ|大丈夫です|ありがとうございます|ごめんなさい|失礼しました|お待たせしました|この件は|その件は|まずは|ひとまず|いったん|一旦|ではまず)$`)
var promiseContentRe = regexp.MustCompile(`(?:できました|完了|登録しました|作成しました|更新しました|移動しました|送信しました|確認できました|取得できました|見えています|見えました|わかっています|つまり|例えば|詳細|理由|結果|状態|いまは|現在|について|一覧|まとめ)`)

func parseDecisionPlan(raw string) (decision.ReplyDecision, error) {
	var planned decision.ReplyDecision
	if err := json.Unmarshal([]byte(raw), &planned); err != nil {
		return decision.ReplyDecision{}, fmt.Errorf("parse decision plan: %w", err)
	}
	return planned, nil
}

func looksLikePromiseOnly(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	if promiseContentRe.MatchString(trimmed) {
		return false
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "- ") || strings.Contains(trimmed, "`") {
		return false
	}

	segments := strings.FieldsFunc(trimmed, func(r rune) bool {
		switch r {
		case '。', '！', '!', '？', '?':
			return true
		default:
			return false
		}
	})
	if len(segments) == 0 {
		segments = []string{trimmed}
	}

	hasPromise := false
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if promiseReplyRe.MatchString(segment) {
			hasPromise = true
			continue
		}
		if promiseFillerRe.MatchString(segment) {
			continue
		}
		return false
	}
	return hasPromise
}

func (a *App) composeDecisionReply(ctx context.Context, msg memory.Message, planned decision.ReplyDecision, report executionReport) (string, error) {
	draft := strings.TrimSpace(planned.Message)
	switch {
	case draft == "" && !report.HasEffects():
		a.logger.Debug("compose reply skipped", "channel", msg.ChannelName, "message_id", msg.ID, "reason", "empty_draft_no_effects")
		return "", nil
	case draft == "" && report.HasEffects():
		a.logger.Debug("compose reply from execution", "channel", msg.ChannelName, "message_id", msg.ID, "reason", "empty_draft_with_effects")
		return a.composeReplyFromExecution(ctx, msg, planned, report, "")
	default:
		a.logger.Debug("compose reply used planner draft", "channel", msg.ChannelName, "message_id", msg.ID, "draft_preview", previewText(draft, 240))
		return draft, nil
	}
}

func (a *App) composeReplyFromExecution(ctx context.Context, msg memory.Message, planned decision.ReplyDecision, report executionReport, draft string) (string, error) {
	if a.thread.ID == "" {
		return "", nil
	}

	prompt := buildExecutionReplyPrompt(msg, planned, report, draft)
	a.logger.Debug("execution reply prompt", "thread_id", a.thread.ID, "channel", msg.ChannelName, "message_id", msg.ID, "prompt_preview", previewText(prompt, 1400))
	raw, err := a.runThreadTurn(ctx, a.thread.ID, prompt)
	if err != nil {
		return "", fmt.Errorf("compose execution reply: %w", err)
	}
	a.logger.Debug("execution reply output", "thread_id", a.thread.ID, "channel", msg.ChannelName, "message_id", msg.ID, "raw_preview", previewText(raw, 800))
	reply := parseAssistantReply(raw)
	if reply.Action == decision.ActionIgnore {
		a.logger.Debug("execution reply suppressed", "channel", msg.ChannelName, "message_id", msg.ID, "reason", "assistant_ignore")
		return "", nil
	}
	return strings.TrimSpace(reply.Message), nil
}

func (a *App) handleJobResult(job jobs.Job, result jobs.Result, runErr error) {
	if a.discord == nil || strings.TrimSpace(job.ChannelID) == "" {
		a.logger.Debug("job observer skipped", "job_id", job.ID, "kind", job.Kind, "reason", "missing_discord_or_channel")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	session, err := a.ensureChannelThread(ctx, job.ChannelID)
	if err != nil {
		a.logger.Warn("job follow-up thread unavailable", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}

	a.logger.Info("job observer fired", "job_id", job.ID, "kind", job.Kind, "done", result.Done, "already_notified", result.AlreadyNotified, "details_preview", previewText(result.Details, 320), "error", runErr)
	prompt := buildJobFollowUpPrompt(job, result, runErr)
	a.logger.Debug("job follow-up prompt", "job_id", job.ID, "kind", job.Kind, "prompt_preview", previewText(prompt, 1400))
	raw, err := a.runThreadTurn(ctx, session.ID, prompt)
	if err != nil {
		a.logger.Warn("job follow-up turn failed", "job_id", job.ID, "kind", job.Kind, "error", err)
		return
	}
	a.logger.Debug("job follow-up output", "job_id", job.ID, "kind", job.Kind, "raw_preview", previewText(raw, 800))
	reply := parseAssistantReply(raw)
	if reply.Action == decision.ActionIgnore || strings.TrimSpace(reply.Message) == "" {
		a.logger.Debug("job follow-up skipped", "job_id", job.ID, "kind", job.Kind, "reason", "assistant_ignore_or_empty")
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
	prompt, _ := job.Payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return jobs.Result{Done: true}, fmt.Errorf("payload.prompt is required")
	}

	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{Done: true}, err
	}

	taskPrompt := buildBackgroundTaskPrompt(job, prompt)
	a.logger.Info("background codex task start", "job_id", job.ID, "title", job.Title, "thread_id", session.ID, "prompt_bytes", len(taskPrompt))
	raw, err := a.runThreadTurn(ctx, session.ID, taskPrompt)
	if err != nil {
		return jobs.Result{Done: true}, err
	}
	a.logger.Info("background codex task completed", "job_id", job.ID, "thread_id", session.ID, "response_bytes", len(raw))
	a.logger.Debug("background codex task output", "job_id", job.ID, "thread_id", session.ID, "response_preview", previewText(raw, 1600))
	return jobs.Result{
		NextRunAt: time.Now().UTC(),
		Done:      true,
		Details:   strings.TrimSpace(raw),
	}, nil
}
