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
	return a.runReviewJob(ctx, job, "open_loop", 72*time.Hour, func(ctx context.Context) (string, bool, error) {
		loops, err := a.store.ListFacts(ctx, "open_loop", 12)
		if err != nil {
			return "", false, err
		}
		if len(loops) == 0 {
			return "", false, nil
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildOpenLoopReviewPrompt(loops, recentOwnerMessages), true, nil
	})
}

func (a *App) handleCuriosityReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "curiosity_review", 24*time.Hour, func(ctx context.Context) (string, bool, error) {
		curiosities, err := a.store.ListFacts(ctx, "curiosity", 12)
		if err != nil {
			return "", false, err
		}
		openLoops, err := a.store.ListFacts(ctx, "open_loop", 8)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 12)
		if err != nil {
			return "", false, err
		}
		return buildCuriosityReviewPrompt(curiosities, openLoops, recentOwnerMessages), true, nil
	})
}

func (a *App) handleInitiativeReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "initiative_review", 48*time.Hour, func(ctx context.Context) (string, bool, error) {
		initiatives, err := a.store.ListFacts(ctx, "initiative", 12)
		if err != nil {
			return "", false, err
		}
		candidates, err := a.store.ListFacts(ctx, "automation_candidate", 12)
		if err != nil {
			return "", false, err
		}
		openLoops, err := a.store.ListFacts(ctx, "open_loop", 8)
		if err != nil {
			return "", false, err
		}
		contextGaps, err := a.store.ListFacts(ctx, "context_gap", 8)
		if err != nil {
			return "", false, err
		}
		return buildInitiativeReviewPrompt(initiatives, candidates, openLoops, contextGaps), true, nil
	})
}

func (a *App) handleSoftReminderReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "soft_reminder_review", 24*time.Hour, func(ctx context.Context) (string, bool, error) {
		reminders, err := a.store.ListFacts(ctx, "soft_reminder", 12)
		if err != nil {
			return "", false, err
		}
		routines, err := a.store.ListFacts(ctx, "routine", 8)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildSoftReminderReviewPrompt(reminders, routines, recentOwnerMessages), true, nil
	})
}

func (a *App) handleTopicSynthesisReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "topic_synthesis_review", 72*time.Hour, func(ctx context.Context) (string, bool, error) {
		topics, err := a.store.ListFacts(ctx, "topic_thread", 12)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 16)
		if err != nil {
			return "", false, err
		}
		summaries, err := a.store.RecentSummaries(ctx, "weekly", 4)
		if err != nil {
			return "", false, err
		}
		return buildTopicSynthesisReviewPrompt(topics, recentOwnerMessages, summaries), true, nil
	})
}

func (a *App) handleBaselineReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "baseline_review", 72*time.Hour, func(ctx context.Context) (string, bool, error) {
		baselines, err := a.store.ListFacts(ctx, "behavior_baseline", 12)
		if err != nil {
			return "", false, err
		}
		deviations, err := a.store.ListFacts(ctx, "behavior_deviation", 12)
		if err != nil {
			return "", false, err
		}
		routines, err := a.store.ListFacts(ctx, "routine", 8)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildBaselineReviewPrompt(baselines, deviations, routines, recentOwnerMessages), true, nil
	})
}

func (a *App) handlePolicySynthesisReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "policy_synthesis_review", 96*time.Hour, func(ctx context.Context) (string, bool, error) {
		learnedPolicies, err := a.store.ListFacts(ctx, "learned_policy", 16)
		if err != nil {
			return "", false, err
		}
		decisions, err := a.store.ListFacts(ctx, "decision", 12)
		if err != nil {
			return "", false, err
		}
		misfires, err := a.store.ListFacts(ctx, "misfire", 12)
		if err != nil {
			return "", false, err
		}
		reflections, err := a.store.RecentSummaries(ctx, "reflection", 8)
		if err != nil {
			return "", false, err
		}
		return buildPolicySynthesisReviewPrompt(learnedPolicies, decisions, misfires, reflections), true, nil
	})
}

func (a *App) handleWorkspaceReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "workspace_review", 48*time.Hour, func(ctx context.Context) (string, bool, error) {
		workspaceNotes, err := a.store.ListFacts(ctx, "workspace_note", 16)
		if err != nil {
			return "", false, err
		}
		initiatives, err := a.store.ListFacts(ctx, "initiative", 12)
		if err != nil {
			return "", false, err
		}
		topics, err := a.store.ListFacts(ctx, "topic_thread", 12)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 12)
		if err != nil {
			return "", false, err
		}
		return buildWorkspaceReviewPrompt(workspaceNotes, initiatives, topics, recentOwnerMessages), true, nil
	})
}

func (a *App) handleProposalBoundaryReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "proposal_boundary_review", 96*time.Hour, func(ctx context.Context) (string, bool, error) {
		proposalBoundaries, err := a.store.ListFacts(ctx, "proposal_boundary", 16)
		if err != nil {
			return "", false, err
		}
		initiatives, err := a.store.ListFacts(ctx, "initiative", 12)
		if err != nil {
			return "", false, err
		}
		decisions, err := a.store.ListFacts(ctx, "decision", 12)
		if err != nil {
			return "", false, err
		}
		misfires, err := a.store.ListFacts(ctx, "misfire", 12)
		if err != nil {
			return "", false, err
		}
		contextGaps, err := a.store.ListFacts(ctx, "context_gap", 8)
		if err != nil {
			return "", false, err
		}
		return buildProposalBoundaryReviewPrompt(proposalBoundaries, initiatives, decisions, misfires, contextGaps), true, nil
	})
}

func (a *App) handleChannelCurationJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "space", 168*time.Hour, func(ctx context.Context) (string, bool, error) {
		if a.discord == nil {
			return "", false, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return "", false, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return "", false, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-14*24*time.Hour), 256)
		if err != nil {
			return "", false, err
		}
		return buildChannelCurationPrompt(channels, profiles, activity), true, nil
	})
}

func (a *App) handleDecisionReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "decision_review", 120*time.Hour, func(ctx context.Context) (string, bool, error) {
		decisions, err := a.store.ListFacts(ctx, "decision", 16)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildDecisionReviewPrompt(decisions, recentOwnerMessages), true, nil
	})
}

func (a *App) handleSelfImprovementReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "self_improvement", 168*time.Hour, func(ctx context.Context) (string, bool, error) {
		candidates, err := a.store.ListFacts(ctx, "automation_candidate", 16)
		if err != nil {
			return "", false, err
		}
		reflections, err := a.store.RecentSummaries(ctx, "reflection", 8)
		if err != nil {
			return "", false, err
		}
		growth, err := a.store.RecentSummaries(ctx, "growth", 8)
		if err != nil {
			return "", false, err
		}
		return buildSelfImprovementReviewPrompt(candidates, reflections, growth), true, nil
	})
}

func (a *App) handleChannelRoleReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "channel_role_review", 168*time.Hour, func(ctx context.Context) (string, bool, error) {
		if a.discord == nil {
			return "", false, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return "", false, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return "", false, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-14*24*time.Hour), 256)
		if err != nil {
			return "", false, err
		}
		return buildChannelRoleReviewPrompt(channels, profiles, activity), true, nil
	})
}

func (a *App) runReviewJob(ctx context.Context, job jobs.Job, period string, defaultInterval time.Duration, build func(context.Context) (string, bool, error)) (jobs.Result, error) {
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, defaultInterval))
	prompt, ok, err := build(ctx)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun}, err
	}
	if !ok {
		return jobs.Result{NextRunAt: nextRun}, nil
	}
	return a.runNarrativeJob(ctx, job, period, prompt, nextRun, false)
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

func buildCuriosityReviewPrompt(curiosities []memory.Fact, openLoops []memory.Fact, recentOwnerMessages []memory.Message) string {
	curiosityLines := make([]string, 0, len(curiosities))
	for _, item := range curiosities {
		curiosityLines = append(curiosityLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(curiosityLines) == 0 {
		curiosityLines = append(curiosityLines, "- none")
	}
	openLoopLines := make([]string, 0, len(openLoops))
	for _, item := range openLoops {
		openLoopLines = append(openLoopLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(openLoopLines) == 0 {
		openLoopLines = append(openLoopLines, "- none")
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 180)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	return fmt.Sprintf(`気になっている疑問を見直して、自分で調べておいたら役に立ちそうなものだけを 1 から 3 個、日本語で短くまとめてください。
ここでは即実行ではなく、調べる価値のある問い、軽い調査の向き先、寝かせてもよい問いを見分けてください。
必要性が薄ければ %s だけを返してください。

curiosities:
%s

open loops:
%s

recent owner messages:
%s`, noReplyToken, strings.Join(curiosityLines, "\n"), strings.Join(openLoopLines, "\n"), strings.Join(messageLines, "\n"))
}

func buildInitiativeReviewPrompt(initiatives []memory.Fact, candidates []memory.Fact, openLoops []memory.Fact, contextGaps []memory.Fact) string {
	initiativeLines := make([]string, 0, len(initiatives))
	for _, item := range initiatives {
		initiativeLines = append(initiativeLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(initiativeLines) == 0 {
		initiativeLines = append(initiativeLines, "- none")
	}
	candidateLines := make([]string, 0, len(candidates))
	for _, item := range candidates {
		candidateLines = append(candidateLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(candidateLines) == 0 {
		candidateLines = append(candidateLines, "- none")
	}
	openLoopLines := make([]string, 0, len(openLoops))
	for _, item := range openLoops {
		openLoopLines = append(openLoopLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(openLoopLines) == 0 {
		openLoopLines = append(openLoopLines, "- none")
	}
	gapLines := make([]string, 0, len(contextGaps))
	for _, item := range contextGaps {
		gapLines = append(gapLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(gapLines) == 0 {
		gapLines = append(gapLines, "- none")
	}
	return fmt.Sprintf(`いまの流れで自分からやる価値があることを見直してください。
分類は、1. 勝手に整えてよい軽い下ごしらえ 2. 提案だけに留めるべきこと 3. まだ様子見 の 3 つです。
必要性が薄ければ %s だけを返してください。

initiatives:
%s

automation candidates:
%s

open loops:
%s

context gaps:
%s`, noReplyToken, strings.Join(initiativeLines, "\n"), strings.Join(candidateLines, "\n"), strings.Join(openLoopLines, "\n"), strings.Join(gapLines, "\n"))
}

func buildSoftReminderReviewPrompt(reminders []memory.Fact, routines []memory.Fact, recentOwnerMessages []memory.Message) string {
	reminderLines := make([]string, 0, len(reminders))
	for _, item := range reminders {
		reminderLines = append(reminderLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(reminderLines) == 0 {
		reminderLines = append(reminderLines, "- none")
	}
	routineLines := make([]string, 0, len(routines))
	for _, item := range routines {
		routineLines = append(routineLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(routineLines) == 0 {
		routineLines = append(routineLines, "- none")
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 180)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	return fmt.Sprintf(`曖昧な未来メモを見直して、そろそろ声をかけても自然そうなものだけを短くまとめてください。
来月、そのうち、あとで、のような曖昧さをそのまま扱い、今はまだ早そうなものは寝かせてください。
必要性が薄ければ %s だけを返してください。

soft reminders:
%s

routines:
%s

recent owner messages:
%s`, noReplyToken, strings.Join(reminderLines, "\n"), strings.Join(routineLines, "\n"), strings.Join(messageLines, "\n"))
}

func buildTopicSynthesisReviewPrompt(topics []memory.Fact, recentOwnerMessages []memory.Message, summaries []memory.Summary) string {
	topicLines := make([]string, 0, len(topics))
	for _, item := range topics {
		topicLines = append(topicLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(topicLines) == 0 {
		topicLines = append(topicLines, "- none")
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 160)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	summaryLines := make([]string, 0, len(summaries))
	for _, item := range summaries {
		summaryLines = append(summaryLines, fmt.Sprintf("- %s", truncateText(item.Content, 180)))
	}
	if len(summaryLines) == 0 {
		summaryLines = append(summaryLines, "- none")
	}
	return fmt.Sprintf(`散らばった話題を束ね直して、今まとまり始めているテーマがあるかを短くまとめてください。
単なる日報ではなく、別チャンネルや別の日に散らばった断片が、どの話題へ寄っているかを見る観点で書いてください。
必要性が薄ければ %s だけを返してください。

topic threads:
%s

recent owner messages:
%s

recent weekly summaries:
%s`, noReplyToken, strings.Join(topicLines, "\n"), strings.Join(messageLines, "\n"), strings.Join(summaryLines, "\n"))
}

func buildBaselineReviewPrompt(baselines []memory.Fact, deviations []memory.Fact, routines []memory.Fact, recentOwnerMessages []memory.Message) string {
	baselineLines := make([]string, 0, len(baselines))
	for _, item := range baselines {
		baselineLines = append(baselineLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(baselineLines) == 0 {
		baselineLines = append(baselineLines, "- none")
	}
	deviationLines := make([]string, 0, len(deviations))
	for _, item := range deviations {
		deviationLines = append(deviationLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(deviationLines) == 0 {
		deviationLines = append(deviationLines, "- none")
	}
	routineLines := make([]string, 0, len(routines))
	for _, item := range routines {
		routineLines = append(routineLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(routineLines) == 0 {
		routineLines = append(routineLines, "- none")
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 160)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	return fmt.Sprintf(`いつもと違う気配を見直して、口に出すほどではない観測と、軽く寄り添ったほうがよさそうな違いを分けて短くまとめてください。
過剰に踏み込みすぎず、見守り寄りの観測として扱ってください。
必要性が薄ければ %s だけを返してください。

behavior baselines:
%s

behavior deviations:
%s

routines:
%s

recent owner messages:
%s`, noReplyToken, strings.Join(baselineLines, "\n"), strings.Join(deviationLines, "\n"), strings.Join(routineLines, "\n"), strings.Join(messageLines, "\n"))
}

func buildPolicySynthesisReviewPrompt(learnedPolicies []memory.Fact, decisions []memory.Fact, misfires []memory.Fact, reflections []memory.Summary) string {
	policyLines := make([]string, 0, len(learnedPolicies))
	for _, item := range learnedPolicies {
		policyLines = append(policyLines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
	}
	if len(policyLines) == 0 {
		policyLines = append(policyLines, "- none")
	}
	decisionLines := make([]string, 0, len(decisions))
	for _, item := range decisions {
		decisionLines = append(decisionLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(decisionLines) == 0 {
		decisionLines = append(decisionLines, "- none")
	}
	misfireLines := make([]string, 0, len(misfires))
	for _, item := range misfires {
		misfireLines = append(misfireLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(misfireLines) == 0 {
		misfireLines = append(misfireLines, "- none")
	}
	reflectionLines := make([]string, 0, len(reflections))
	for _, item := range reflections {
		reflectionLines = append(reflectionLines, fmt.Sprintf("- %s", truncateText(item.Content, 180)))
	}
	if len(reflectionLines) == 0 {
		reflectionLines = append(reflectionLines, "- none")
	}
	return fmt.Sprintf(`経験からにじんだ learned policy を見直してください。
続けるとよさそうな方針、弱めたほうがよさそうな方針、まだ仮置きにしておく方針を 1 から 3 個だけ短くまとめてください。
基底人格や固定ルールを書き換えるのではなく、最近の成功・失敗から学べる軽い方針として扱ってください。
必要性が薄ければ %s だけを返してください。

learned policies:
%s

recent decisions:
%s

recent misfires:
%s

recent reflections:
%s`, noReplyToken, strings.Join(policyLines, "\n"), strings.Join(decisionLines, "\n"), strings.Join(misfireLines, "\n"), strings.Join(reflectionLines, "\n"))
}

func buildWorkspaceReviewPrompt(workspaceNotes []memory.Fact, initiatives []memory.Fact, topics []memory.Fact, recentOwnerMessages []memory.Message) string {
	workspaceNoteLines := make([]string, 0, len(workspaceNotes))
	for _, item := range workspaceNotes {
		workspaceNoteLines = append(workspaceNoteLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(workspaceNoteLines) == 0 {
		workspaceNoteLines = append(workspaceNoteLines, "- none")
	}
	initiativeLines := make([]string, 0, len(initiatives))
	for _, item := range initiatives {
		initiativeLines = append(initiativeLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(initiativeLines) == 0 {
		initiativeLines = append(initiativeLines, "- none")
	}
	topicLines := make([]string, 0, len(topics))
	for _, item := range topics {
		topicLines = append(topicLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(topicLines) == 0 {
		topicLines = append(topicLines, "- none")
	}
	messageLines := make([]string, 0, len(recentOwnerMessages))
	for _, msg := range recentOwnerMessages {
		messageLines = append(messageLines, fmt.Sprintf("- [%s/%s] %s", msg.CreatedAt.Format("01-02 15:04"), msg.ChannelName, truncateText(msg.Content, 180)))
	}
	if len(messageLines) == 0 {
		messageLines = append(messageLines, "- none")
	}
	return fmt.Sprintf(`自分用の workspace note や途中メモを見直してください。
下書きとして育てる価値があるもの、そろそろ片づけてよいもの、チャンネルや作業場所として形にしたほうがよさそうなものを短くまとめてください。
必要性が薄ければ %s だけを返してください。

workspace notes:
%s

initiatives:
%s

topic threads:
%s

recent owner messages:
%s`, noReplyToken, strings.Join(workspaceNoteLines, "\n"), strings.Join(initiativeLines, "\n"), strings.Join(topicLines, "\n"), strings.Join(messageLines, "\n"))
}

func buildProposalBoundaryReviewPrompt(proposalBoundaries []memory.Fact, initiatives []memory.Fact, decisions []memory.Fact, misfires []memory.Fact, contextGaps []memory.Fact) string {
	boundaryLines := make([]string, 0, len(proposalBoundaries))
	for _, item := range proposalBoundaries {
		boundaryLines = append(boundaryLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(boundaryLines) == 0 {
		boundaryLines = append(boundaryLines, "- none")
	}
	initiativeLines := make([]string, 0, len(initiatives))
	for _, item := range initiatives {
		initiativeLines = append(initiativeLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(initiativeLines) == 0 {
		initiativeLines = append(initiativeLines, "- none")
	}
	decisionLines := make([]string, 0, len(decisions))
	for _, item := range decisions {
		decisionLines = append(decisionLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(decisionLines) == 0 {
		decisionLines = append(decisionLines, "- none")
	}
	misfireLines := make([]string, 0, len(misfires))
	for _, item := range misfires {
		misfireLines = append(misfireLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(misfireLines) == 0 {
		misfireLines = append(misfireLines, "- none")
	}
	contextGapLines := make([]string, 0, len(contextGaps))
	for _, item := range contextGaps {
		contextGapLines = append(contextGapLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 180)))
	}
	if len(contextGapLines) == 0 {
		contextGapLines = append(contextGapLines, "- none")
	}
	return fmt.Sprintf(`自分から勝手にやること、提案に留めること、観測だけしておくことの境界を見直してください。
固定ルールではなく、最近の成功・失敗・迷いから学べる境界メモとして 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

proposal boundaries:
%s

initiatives:
%s

recent decisions:
%s

recent misfires:
%s

context gaps:
%s`, noReplyToken, strings.Join(boundaryLines, "\n"), strings.Join(initiativeLines, "\n"), strings.Join(decisionLines, "\n"), strings.Join(misfireLines, "\n"), strings.Join(contextGapLines, "\n"))
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
