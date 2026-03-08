package logview

import (
	"bytes"
	"context"
	"encoding/json"
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

	var b strings.Builder
	b.WriteString(h.renderHeader(record.Time, record.Level, record.Message))
	for _, line := range renderAttrLines(flat, h.color) {
		b.WriteString("\n  ")
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

func renderAttrLines(attrs []slog.Attr, color bool) []string {
	if len(attrs) == 0 {
		return nil
	}
	width := 0
	for _, attr := range attrs {
		if l := len(attr.Key); l > width {
			width = l
		}
	}
	if width < 12 {
		width = 12
	}
	if width > 24 {
		width = 24
	}
	lines := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		key := attr.Key
		if color {
			key = colorize("\x1b[2m", key)
		}
		valueLines := renderBlockLines(attr.Value)
		if len(valueLines) == 0 {
			valueLines = []string{`""`}
		}
		pad := attr.Key
		if len(attr.Key) < width {
			pad += strings.Repeat(" ", width-len(attr.Key))
		}
		if color {
			pad = key
			if len(attr.Key) < width {
				pad += strings.Repeat(" ", width-len(attr.Key))
			}
		}
		lines = append(lines, pad+"  "+valueLines[0])
		indent := strings.Repeat(" ", width+2)
		for _, extra := range valueLines[1:] {
			lines = append(lines, indent+extra)
		}
	}
	return lines
}

func renderBlockLines(value slog.Value) []string {
	text := renderDisplayValue(value)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

func renderDisplayValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return formatDisplayString(value.String())
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
		return formatDisplayString(fmt.Sprint(value.Any()))
	default:
		return formatDisplayString(value.String())
	}
}

func formatDisplayString(value string) string {
	switch {
	case value == "":
		return `""`
	case looksLikeJSON(value):
		if pretty, ok := tryPrettyJSON(value); ok {
			return pretty
		}
	}
	return value
}

func looksLikeJSON(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")
}

func tryPrettyJSON(value string) (string, bool) {
	var payload any
	if json.Unmarshal([]byte(value), &payload) != nil {
		return "", false
	}
	formatted, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", false
	}
	return string(bytes.TrimSpace(formatted)), true
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
