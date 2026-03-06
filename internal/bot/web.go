package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

const (
	defaultFetchBodyLimit = 512 * 1024
	defaultFetchTextLimit = 2400
)

type urlSnapshot struct {
	URL         string
	FinalURL    string
	Title       string
	ContentType string
	StatusCode  int
	Text        string
	Excerpt     string
	Hash        string
}

var (
	htmlTitleRe  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlScriptRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	htmlTagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRe      = regexp.MustCompile(`\s+`)
)

func (a *App) fetchURLSnapshot(ctx context.Context, rawURL string, maxChars int) (urlSnapshot, error) {
	if maxChars <= 0 {
		maxChars = defaultFetchTextLimit
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("User-Agent", a.cfg.AppName)
	request.Header.Set("Accept", "text/html,application/json,text/plain;q=0.9,*/*;q=0.5")

	response, err := a.http.Do(request)
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("fetch url: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, defaultFetchBodyLimit))
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("read response body: %w", err)
	}

	contentType := response.Header.Get("Content-Type")
	title, text := extractResponseText(contentType, body)
	if title == "" {
		title = response.Request.URL.Host
	}
	text = truncateText(text, maxChars)

	sum := sha256.Sum256(body)
	snapshot := urlSnapshot{
		URL:         rawURL,
		FinalURL:    response.Request.URL.String(),
		Title:       title,
		ContentType: contentType,
		StatusCode:  response.StatusCode,
		Text:        text,
		Excerpt:     truncateText(text, 420),
		Hash:        hex.EncodeToString(sum[:]),
	}
	return snapshot, nil
}

func extractResponseText(contentType string, body []byte) (string, string) {
	raw := strings.TrimSpace(string(body))
	lowerType := strings.ToLower(contentType)

	switch {
	case strings.Contains(lowerType, "html"):
		return extractHTMLText(raw)
	case strings.Contains(lowerType, "json"):
		return "", cleanupText(raw)
	default:
		return "", cleanupText(raw)
	}
}

func extractHTMLText(raw string) (string, string) {
	title := ""
	if match := htmlTitleRe.FindStringSubmatch(raw); len(match) > 1 {
		title = cleanupText(html.UnescapeString(match[1]))
	}

	text := htmlScriptRe.ReplaceAllString(raw, " ")
	text = htmlTagRe.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return title, cleanupText(text)
}

func cleanupText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = spaceRe.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func truncateText(value string, maxChars int) string {
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	cut := value[:maxChars]
	cut = strings.TrimSpace(cut)
	if cut == "" {
		return value[:maxChars]
	}
	return cut + "..."
}

func (a *App) handleURLWatchJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	url, _ := job.Payload["url"].(string)
	if strings.TrimSpace(url) == "" {
		return jobs.Result{NextRunAt: time.Now().UTC().Add(time.Hour)}, fmt.Errorf("url is required")
	}

	snapshot, err := a.fetchURLSnapshot(ctx, url, defaultFetchTextLimit)
	nextRun := time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 6*time.Hour))
	if err != nil {
		return jobs.Result{NextRunAt: nextRun}, err
	}

	lastHash, _ := job.Payload["last_hash"].(string)
	if lastHash == "" {
		job.Payload["last_hash"] = snapshot.Hash
		job.Payload["last_title"] = snapshot.Title
		job.NextRunAt = nextRun
		job.UpdatedAt = time.Now().UTC()
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		a.logger.Info("url watch primed", "job_id", job.ID, "url", url, "title", snapshot.Title)
		return jobs.Result{NextRunAt: nextRun}, nil
	}

	if lastHash != snapshot.Hash {
		message := fmt.Sprintf("%s の更新を見つけましたよ。\n- title: %s\n- url: %s\n- status: %d\n- excerpt: %s",
			snapshot.FinalURL, snapshot.Title, snapshot.FinalURL, snapshot.StatusCode, snapshot.Excerpt)
		if _, err := a.discord.SendMessage(ctx, job.ChannelID, message); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		job.Payload["last_hash"] = snapshot.Hash
		job.Payload["last_title"] = snapshot.Title
		job.NextRunAt = nextRun
		job.UpdatedAt = time.Now().UTC()
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return jobs.Result{NextRunAt: nextRun}, err
		}
		a.logger.Info("url watch changed", "job_id", job.ID, "url", url, "title", snapshot.Title)
		return jobs.Result{
			NextRunAt:       nextRun,
			Details:         fmt.Sprintf("url changed: %s %s", snapshot.Title, snapshot.FinalURL),
			AlreadyNotified: true,
		}, nil
	}
	return jobs.Result{NextRunAt: nextRun}, nil
}
