package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

func (a *App) handleReleaseWatchJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	repo, _ := job.Payload["repo"].(string)
	if repo == "" {
		repo = "openai/codex"
	}
	a.logger.Info("release watch check", "job_id", job.ID, "repo", repo)
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
		message := fmt.Sprintf("Codex の安定リリースが更新されていましたよ。\nよかったら見てみてくださいね。\n- tag: %s\n- name: %s\n- published: %s\n- url: %s",
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
		return jobs.Result{
			NextRunAt:       nextRun,
			Details:         fmt.Sprintf("release updated: %s %s", release.TagName, release.HTMLURL),
			AlreadyNotified: true,
		}, nil
	}
	return jobs.Result{NextRunAt: nextRun}, nil
}

func (a *App) fetchLatestStableRelease(ctx context.Context, repo string) (githubRelease, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=10", repo), nil)
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
