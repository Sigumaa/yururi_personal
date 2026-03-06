package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) handleReminderJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.discord == nil {
		return jobs.Result{Done: true}, errors.New("discord is not connected")
	}
	content, _ := job.Payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		content, _ = job.Payload["message"].(string)
	}
	if strings.TrimSpace(content) == "" {
		return jobs.Result{Done: true}, errors.New("payload.content or payload.message is required")
	}
	if strings.TrimSpace(job.ChannelID) == "" {
		return jobs.Result{Done: true}, errors.New("channel_id is required")
	}
	if _, err := a.discord.SendMessage(ctx, job.ChannelID, strings.TrimSpace(content)); err != nil {
		return jobs.Result{Done: true}, err
	}
	return jobs.Result{
		NextRunAt:       time.Now().UTC(),
		Done:            true,
		Details:         "reminder sent",
		AlreadyNotified: true,
	}, nil
}

func (a *App) handleMonthlyReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-30 * 24 * time.Hour)
	nextRun := nextMonthClock(end, a.loc, 1, 21, 30)
	return a.runSummaryJob(ctx, job, "monthly", start.UTC(), end.UTC(), nextRun, false)
}

func (a *App) handleOpenLoopReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	loops, err := a.store.ListFacts(ctx, "open_loop", 12)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 72*time.Hour))}, err
	}
	if len(loops) == 0 {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 72*time.Hour))}, nil
	}
	recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 72*time.Hour))}, err
	}
	prompt := buildOpenLoopReviewPrompt(loops, recentOwnerMessages)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 72*time.Hour))
	return a.runNarrativeJob(ctx, job, "open_loop", prompt, nextRun, false)
}

func (a *App) handleChannelCurationJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.discord == nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, errors.New("discord is not connected")
	}
	channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	profiles, err := a.store.ListChannelProfiles(ctx)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-14*24*time.Hour), 256)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	prompt := buildChannelCurationPrompt(channels, profiles, activity)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))
	return a.runNarrativeJob(ctx, job, "space", prompt, nextRun, false)
}

func (a *App) handleDecisionReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	decisions, err := a.store.ListFacts(ctx, "decision", 16)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 120*time.Hour))}, err
	}
	recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 120*time.Hour))}, err
	}
	prompt := buildDecisionReviewPrompt(decisions, recentOwnerMessages)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 120*time.Hour))
	return a.runNarrativeJob(ctx, job, "decision_review", prompt, nextRun, false)
}

func (a *App) handleSelfImprovementReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	candidates, err := a.store.ListFacts(ctx, "automation_candidate", 16)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	reflections, err := a.store.RecentSummaries(ctx, "reflection", 8)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	growth, err := a.store.RecentSummaries(ctx, "growth", 8)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	prompt := buildSelfImprovementReviewPrompt(candidates, reflections, growth)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))
	return a.runNarrativeJob(ctx, job, "self_improvement", prompt, nextRun, false)
}

func (a *App) handleChannelRoleReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.discord == nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, errors.New("discord is not connected")
	}
	channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	profiles, err := a.store.ListChannelProfiles(ctx)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-14*24*time.Hour), 256)
	if err != nil {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))}, err
	}
	prompt := buildChannelRoleReviewPrompt(channels, profiles, activity)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 168*time.Hour))
	return a.runNarrativeJob(ctx, job, "channel_role_review", prompt, nextRun, false)
}

func (a *App) runNarrativeJob(ctx context.Context, job jobs.Job, period string, prompt string, nextRun time.Time, done bool) (jobs.Result, error) {
	session, err := a.ensureJobThread(ctx, job)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}

	raw, err := a.runThreadTurn(ctx, session.ID, prompt)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	reply := parseAssistantReply(raw)
	if reply.Action == "ignore" || strings.TrimSpace(reply.Message) == "" {
		return jobs.Result{NextRunAt: nextRun, Done: done}, nil
	}
	text := strings.TrimSpace(reply.Message)
	if strings.TrimSpace(job.ChannelID) != "" && a.discord != nil {
		if _, err := a.discord.SendMessage(ctx, job.ChannelID, text); err != nil {
			return jobs.Result{NextRunAt: nextRun, Done: done}, err
		}
	}
	now := time.Now().UTC()
	if err := a.store.SaveSummary(ctx, memory.Summary{
		Period:    period,
		ChannelID: job.ChannelID,
		Content:   text,
		StartsAt:  now,
		EndsAt:    now,
		CreatedAt: now,
	}); err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	return jobs.Result{
		NextRunAt:       nextRun,
		Done:            done,
		Details:         fmt.Sprintf("%s narrative saved", period),
		AlreadyNotified: strings.TrimSpace(job.ChannelID) != "",
	}, nil
}

func buildOpenLoopReviewPrompt(loops []memory.Fact, recentOwnerMessages []memory.Message) string {
	loopLines := make([]string, 0, len(loops))
	for _, loop := range loops {
		loopLines = append(loopLines, fmt.Sprintf("- %s: %s", loop.Key, loop.Value))
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 180)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	return fmt.Sprintf(`未解決の open loop を見直して、いま声をかける価値があるものだけを日本語で短くまとめてください。
単なる一覧ではなく、いま気にしておいたほうがよいこと、優先度、軽い次の一歩をやわらかく添えてください。
必要性が薄ければ %s だけを返してください。

open loops:
%s

recent owner messages:
%s`, noReplyToken, strings.Join(loopLines, "\n"), strings.Join(messageLines, "\n"))
}

func buildChannelCurationPrompt(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity) string {
	serverView := describeServer(channels, profiles, activity, time.UTC)
	return fmt.Sprintf(`Discord 空間の整理観点を 1 回だけ提案してください。
やることは即実行ではなく、いま気づいた整理候補の提示です。
次の 3 点を短く出してください。
1. 最近育っている場所
2. 静かで整理候補になりそうな場所
3. 次に 1 つだけやるなら何か
必要な提案が特に無ければ %s だけを返してください。

server snapshot:
%s`, noReplyToken, serverView)
}

func buildDecisionReviewPrompt(decisions []memory.Fact, recentOwnerMessages []memory.Message) string {
	decisionLines := make([]string, 0, len(decisions))
	for _, item := range decisions {
		decisionLines = append(decisionLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(decisionLines) == 0 {
		decisionLines = append(decisionLines, "- none")
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 180)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	return fmt.Sprintf(`最近の decision を軽く見直して、いまの会話傾向に対して効きそうなものだけを短くまとめてください。
古い判断を盲信せず、続けるもの、弱めるもの、あとで見直すものを柔らかく示してください。
必要性が薄ければ %s だけを返してください。

recent decisions:
%s

recent owner messages:
%s`, noReplyToken, strings.Join(decisionLines, "\n"), strings.Join(messageLines, "\n"))
}

func buildSelfImprovementReviewPrompt(candidates []memory.Fact, reflections []memory.Summary, growth []memory.Summary) string {
	candidateLines := make([]string, 0, len(candidates))
	for _, item := range candidates {
		candidateLines = append(candidateLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(candidateLines) == 0 {
		candidateLines = append(candidateLines, "- none")
	}
	reflectionLines := make([]string, 0, len(reflections))
	for _, item := range reflections {
		reflectionLines = append(reflectionLines, fmt.Sprintf("- %s", truncateText(item.Content, 180)))
	}
	if len(reflectionLines) == 0 {
		reflectionLines = append(reflectionLines, "- none")
	}
	growthLines := make([]string, 0, len(growth))
	for _, item := range growth {
		growthLines = append(growthLines, fmt.Sprintf("- %s", truncateText(item.Content, 180)))
	}
	if len(growthLines) == 0 {
		growthLines = append(growthLines, "- none")
	}
	return fmt.Sprintf(`自分の振る舞い改善を 1 回だけ見直してください。
自動化候補、reflection、growth から、次に効きそうな改善の種を 1 から 3 個だけ短くまとめてください。
実装を決め打ちせず、観測、整備、記録の観点で書いてください。
必要性が薄ければ %s だけを返してください。

automation candidates:
%s

reflections:
%s

growth:
%s`, noReplyToken, strings.Join(candidateLines, "\n"), strings.Join(reflectionLines, "\n"), strings.Join(growthLines, "\n"))
}

func buildChannelRoleReviewPrompt(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity) string {
	serverView := describeServer(channels, profiles, activity, time.UTC)
	return fmt.Sprintf(`Discord 空間の channel role を見直してください。
今ある profile を固定前提にせず、最近の活動とカテゴリ構造から、役割 drift が起きていそうな場所だけを短く示してください。
必要なら profile をどう寄せるとよさそうかを 1 行ずつ添えてください。
必要性が薄ければ %s だけを返してください。

server snapshot:
%s`, noReplyToken, serverView)
}
