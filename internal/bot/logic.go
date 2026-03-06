package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/decision"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

const noReplyToken = "<NO_REPLY>"

func parseAssistantReply(raw string) decision.ReplyDecision {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "", strings.EqualFold(trimmed, noReplyToken):
		return decision.ReplyDecision{
			Action: decision.ActionIgnore,
			Reason: "codex selected silence",
		}
	default:
		return decision.ReplyDecision{
			Action:  decision.ActionReply,
			Reason:  "codex text reply",
			Message: trimmed,
		}
	}
}

func (a *App) handleReleaseWatchJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	repo, _ := job.Payload["repo"].(string)
	if repo == "" {
		repo = "openai/codex"
	}
	release, err := a.fetchLatestStableRelease(ctx, repo)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 6*time.Hour))
	if err != nil {
		return jobs.Result{NextRunAt: nextRun}, err
	}

	lastSeen, _ := job.Payload["last_seen_tag"].(string)
	if lastSeen == "" {
		job.Payload["last_seen_tag"] = release.TagName
		job.NextRunAt = nextRun
		job.UpdatedAt = time.Now().UTC()
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		return jobs.Result{NextRunAt: nextRun}, nil
	}
	if lastSeen != release.TagName {
		message := fmt.Sprintf("Codex の安定リリースが更新されました。\n- tag: %s\n- name: %s\n- published: %s\n- url: %s",
			release.TagName, release.Name, release.PublishedAt.Format(time.RFC3339), release.HTMLURL)
		if _, err := a.discord.SendMessage(ctx, job.ChannelID, message); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		job.Payload["last_seen_tag"] = release.TagName
		job.NextRunAt = nextRun
		job.UpdatedAt = time.Now().UTC()
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
	}
	return jobs.Result{NextRunAt: nextRun}, nil
}

func (a *App) handleDailySummaryJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-24 * time.Hour)
	nextRun := nextLocalClock(end, a.loc, 23, 30)
	return a.runSummaryJob(ctx, job, "daily", start.UTC(), end.UTC(), nextRun, false)
}

func (a *App) handleWeeklyReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().In(a.loc)
	start := end.Add(-7 * 24 * time.Hour)
	nextRun := nextWeekdayClock(end, a.loc, time.Sunday, 21, 0)
	return a.runSummaryJob(ctx, job, "weekly", start.UTC(), end.UTC(), nextRun, false)
}

func (a *App) handleGrowthLogJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	nextRun := nextLocalClock(time.Now().In(a.loc), a.loc, 23, 45)
	return a.runSummaryJob(ctx, job, "growth", start, end, nextRun, false)
}

func (a *App) handleWakeSummaryJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	sinceRaw, _ := job.Payload["since"].(string)
	since, err := time.Parse(time.RFC3339Nano, sinceRaw)
	if err != nil {
		return jobs.Result{Done: true}, err
	}
	return a.runSummaryJob(ctx, job, "wake", since.UTC(), time.Now().UTC(), time.Now().UTC(), true)
}

func (a *App) runSummaryJob(ctx context.Context, job jobs.Job, period string, start time.Time, end time.Time, nextRun time.Time, done bool) (jobs.Result, error) {
	messages, err := a.store.MessagesBetween(ctx, start, end, 200)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	if len(messages) == 0 {
		return jobs.Result{NextRunAt: nextRun, Done: done}, nil
	}

	summaryText, err := a.summarizeMessages(ctx, period, start, end, messages)
	if err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	if _, err := a.discord.SendMessage(ctx, job.ChannelID, summaryText); err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	if err := a.store.SaveSummary(ctx, memory.Summary{
		Period:    period,
		ChannelID: job.ChannelID,
		Content:   summaryText,
		StartsAt:  start,
		EndsAt:    end,
	}); err != nil {
		return jobs.Result{NextRunAt: nextRun, Done: done}, err
	}
	return jobs.Result{NextRunAt: nextRun, Done: done}, nil
}

func (a *App) summarizeMessages(ctx context.Context, period string, start time.Time, end time.Time, messages []memory.Message) (string, error) {
	if a.thread.ID == "" {
		return fallbackSummary(period, start, end, messages), nil
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary"},
		"properties": map[string]any{
			"summary": map[string]any{
				"type": "string",
			},
		},
	}
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		lines = append(lines, fmt.Sprintf("- [%s/%s] %s: %s", msg.CreatedAt.In(a.loc).Format("01-02 15:04"), msg.ChannelName, msg.AuthorName, msg.Content))
	}
	prompt := fmt.Sprintf(`%s のまとめを作成してください。
期間: %s - %s
出力は JSON だけにし、summary に完成文を入れてください。
daily と wake は短め、weekly と growth は少し俯瞰を入れてください。
文章は日本語で、ゆるりとしてやわらかく。

messages:
%s`, period, start.In(a.loc).Format(time.RFC3339), end.In(a.loc).Format(time.RFC3339), strings.Join(lines, "\n"))

	a.codexMu.Lock()
	defer a.codexMu.Unlock()
	raw, err := a.codex.RunJSONTurn(ctx, a.thread.ID, prompt, schema)
	if err != nil {
		a.logger.Warn("summary codex turn failed; using fallback summary", "period", period, "error", err)
		return fallbackSummary(period, start, end, messages), nil
	}
	var response struct {
		Summary string `json:"summary"`
	}
	if parseErr := json.Unmarshal([]byte(raw), &response); parseErr != nil || strings.TrimSpace(response.Summary) == "" {
		a.logger.Warn("summary codex output invalid; using fallback summary", "period", period, "error", parseErr)
		return fallbackSummary(period, start, end, messages), nil
	}
	return response.Summary, nil
}

func fallbackSummary(period string, start time.Time, end time.Time, messages []memory.Message) string {
	lines := []string{fmt.Sprintf("%s のまとめです。", period)}
	seenChannels := map[string]int{}
	for _, msg := range messages {
		seenChannels[msg.ChannelName]++
	}
	var channels []string
	for channel, count := range seenChannels {
		channels = append(channels, fmt.Sprintf("%s %d件", channel, count))
	}
	slices.Sort(channels)
	lines = append(lines, fmt.Sprintf("期間は %s から %s まで。", start.Format("01/02 15:04"), end.Format("01/02 15:04")))
	lines = append(lines, "動きがあった場所: "+strings.Join(channels, ", "))
	for _, msg := range tailMessages(messages, 5) {
		lines = append(lines, fmt.Sprintf("- [%s] %s", msg.ChannelName, msg.Content))
	}
	return strings.Join(lines, "\n")
}

func tailMessages(messages []memory.Message, n int) []memory.Message {
	if len(messages) <= n {
		return messages
	}
	return messages[len(messages)-n:]
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

func (a *App) fetchLatestStableRelease(ctx context.Context, repo string) (githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=10", repo)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", a.cfg.AppName)

	response, err := a.http.Do(request)
	if err != nil {
		return githubRelease{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return githubRelease{}, fmt.Errorf("github releases: %s", strings.TrimSpace(string(body)))
	}
	var releases []githubRelease
	if err := json.NewDecoder(response.Body).Decode(&releases); err != nil {
		return githubRelease{}, err
	}
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			continue
		}
		return release, nil
	}
	return githubRelease{}, fmt.Errorf("no stable release found for %s", repo)
}
