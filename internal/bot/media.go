package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/bwmarrin/discordgo"
)

const (
	defaultImageFetchTimeout = 20 * time.Second
	defaultImageFetchLimit   = 8 * 1024 * 1024
)

func imageAttachmentURLs(attachments []*discordgo.MessageAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if !looksLikeImageAttachment(attachment) {
			continue
		}
		if strings.TrimSpace(attachment.URL) == "" {
			continue
		}
		out = append(out, attachment.URL)
	}
	return out
}

func looksLikeImageAttachment(attachment *discordgo.MessageAttachment) bool {
	if attachment == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(attachment.ContentType))
	if strings.HasPrefix(contentType, "image/") {
		return true
	}
	return inferImageContentType(attachment.URL, attachment.Filename) != ""
}

func (a *App) buildImageInputs(ctx context.Context, urls []string) ([]codex.InputItem, []string) {
	items := make([]codex.InputItem, 0, len(urls))
	notes := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		imageURL, mode, err := a.resolveImageInputURL(ctx, rawURL)
		if err != nil {
			a.logger.Warn("image fetch fallback", "url", rawURL, "error", err)
			imageURL = rawURL
			mode = "remote-fallback"
		}
		items = append(items, codex.ImageInput(imageURL))
		notes = append(notes, fmt.Sprintf("- %s (%s)", rawURL, mode))
	}
	return items, notes
}

func (a *App) resolveImageInputURL(ctx context.Context, rawURL string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(rawURL), "data:image/") {
		return rawURL, "data-url", nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, defaultImageFetchTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("build image request: %w", err)
	}
	request.Header.Set("User-Agent", a.cfg.AppName)
	request.Header.Set("Accept", "image/*,*/*;q=0.5")

	response, err := a.http.Do(request)
	if err != nil {
		return "", "", fmt.Errorf("fetch image: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("fetch image status=%d", response.StatusCode)
	}

	contentType := normalizeImageContentType(response.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = inferImageContentType(rawURL, "")
	}
	if contentType == "" {
		return "", "", fmt.Errorf("non-image content type: %s", response.Header.Get("Content-Type"))
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, defaultImageFetchLimit+1))
	if err != nil {
		return "", "", fmt.Errorf("read image: %w", err)
	}
	if len(body) == 0 {
		return "", "", fmt.Errorf("empty image body")
	}
	if len(body) > defaultImageFetchLimit {
		return "", "", fmt.Errorf("image too large: %d bytes", len(body))
	}

	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(body))
	a.logger.Debug("image fetched for turn", "url", rawURL, "bytes", len(body), "content_type", contentType)
	return dataURL, "embedded-data", nil
}

func normalizeImageContentType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if index := strings.Index(value, ";"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	if strings.HasPrefix(value, "image/") {
		return value
	}
	return ""
}

func inferImageContentType(rawURL string, filename string) string {
	if strings.TrimSpace(filename) != "" {
		if guessed := mime.TypeByExtension(strings.ToLower(path.Ext(filename))); strings.HasPrefix(guessed, "image/") {
			return guessed
		}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if guessed := mime.TypeByExtension(strings.ToLower(path.Ext(parsed.Path))); strings.HasPrefix(guessed, "image/") {
		return guessed
	}
	return ""
}
