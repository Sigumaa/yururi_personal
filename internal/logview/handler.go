package logview

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Level      slog.Leveler
	Color      bool
	TimeFormat string
}

type Handler struct {
	mu     *sync.Mutex
	w      io.Writer
	level  slog.Leveler
	color  bool
	format string
	attrs  []slog.Attr
	groups []string
}

func NewHandler(w io.Writer, opts Options) *Handler {
	level := opts.Level
	if level == nil {
		level = slog.LevelInfo
	}
	format := opts.TimeFormat
	if strings.TrimSpace(format) == "" {
		format = "2006-01-02 15:04:05.000"
	}
	return &Handler{
		mu:     &sync.Mutex{},
		w:      w,
		level:  level,
		color:  opts.Color,
		format: format,
	}
}

func DefaultColorEnabled(file *os.File) bool {
	if file == nil {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	attrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	attrs = append(attrs, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	flat := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		flat = append(flat, flattenAttr(h.groups, attr)...)
	}

	inline, block := partitionAttrs(flat)

	var b strings.Builder
	b.WriteString(h.renderHeader(record.Time, record.Level, record.Message))
	if len(inline) > 0 {
		b.WriteString(" | ")
		b.WriteString(strings.Join(inline, "  "))
	}
	for _, line := range block {
		b.WriteString("\n    ")
		b.WriteString(line)
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

func (h *Handler) clone() *Handler {
	attrs := append([]slog.Attr(nil), h.attrs...)
	groups := append([]string(nil), h.groups...)
	return &Handler{
		mu:     h.mu,
		w:      h.w,
		level:  h.level,
		color:  h.color,
		format: h.format,
		attrs:  attrs,
		groups: groups,
	}
}

func (h *Handler) renderHeader(ts time.Time, level slog.Level, msg string) string {
	timeText := ts.Format(h.format)
	levelText := levelLabel(level)
	if h.color {
		levelText = colorize(levelColor(level), levelText)
		msg = colorize("\x1b[1m", msg)
	}
	return fmt.Sprintf("%s %s %s", timeText, levelText, msg)
}

func flattenAttr(groups []string, attr slog.Attr) []slog.Attr {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return nil
	}
	if attr.Value.Kind() == slog.KindGroup {
		groupPath := groups
		if strings.TrimSpace(attr.Key) != "" {
			groupPath = append(append([]string(nil), groups...), attr.Key)
		}
		var flat []slog.Attr
		for _, child := range attr.Value.Group() {
			flat = append(flat, flattenAttr(groupPath, child)...)
		}
		return flat
	}
	if len(groups) == 0 {
		return []slog.Attr{attr}
	}
	clone := attr
	clone.Key = strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
	return []slog.Attr{clone}
}

func partitionAttrs(attrs []slog.Attr) ([]string, []string) {
	inline := make([]string, 0, len(attrs))
	block := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		value := renderValue(attr.Value)
		entry := attr.Key + "=" + value
		if shouldInline(attr.Key, value) {
			inline = append(inline, entry)
			continue
		}
		block = append(block, attr.Key+": "+renderBlockValue(attr.Value))
	}
	return inline, block
}

func shouldInline(key string, value string) bool {
	if strings.Contains(value, "\n") {
		return false
	}
	if len(value) > 72 {
		return false
	}
	for _, token := range []string{
		"preview",
		"arguments",
		"response",
		"message",
		"prompt",
		"content",
		"bundle",
		"details",
		"line",
		"error_data",
	} {
		if strings.Contains(key, token) {
			return false
		}
	}
	return true
}

func renderValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return quoteIfNeeded(value.String())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'f', -1, 64)
	case slog.KindBool:
		if value.Bool() {
			return "true"
		}
		return "false"
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		return quoteIfNeeded(fmt.Sprint(value.Any()))
	default:
		return quoteIfNeeded(value.String())
	}
}

func renderBlockValue(value slog.Value) string {
	text := renderValue(value)
	if !strings.Contains(text, "\n") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		if i == 0 {
			continue
		}
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func quoteIfNeeded(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\r\n=|") {
		return strconv.Quote(value)
	}
	return value
}

func levelLabel(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "DBG"
	case level < slog.LevelWarn:
		return "INF"
	case level < slog.LevelError:
		return "WRN"
	default:
		return "ERR"
	}
}

func levelColor(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "\x1b[36m"
	case level < slog.LevelWarn:
		return "\x1b[32m"
	case level < slog.LevelError:
		return "\x1b[33m"
	default:
		return "\x1b[31m"
	}
}

func colorize(prefix string, value string) string {
	return prefix + value + "\x1b[0m"
}
