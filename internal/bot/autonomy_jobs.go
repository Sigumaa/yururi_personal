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
		return jobs.Result{Done: true}, errors.New("payload.content is required")
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
