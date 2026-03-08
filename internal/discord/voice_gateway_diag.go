package discord

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type voiceGatewayCloseEvent struct {
	Code   int
	Reason string
	Raw    string
	At     time.Time
}

var (
	voiceGatewayLoggerOnce sync.Once
	voiceGatewayCloseMu    sync.RWMutex
	lastVoiceGatewayClose  voiceGatewayCloseEvent
	discordgoLoggerMu      sync.RWMutex
	discordgoLogger        *slog.Logger

	voiceGatewayClosePattern = regexp.MustCompile(`websocket: close ([0-9]+):\s*(.+)$`)
)

func SetLibraryLogger(logger *slog.Logger) {
	discordgoLoggerMu.Lock()
	defer discordgoLoggerMu.Unlock()
	discordgoLogger = logger
}

func installVoiceGatewayLogger() {
	voiceGatewayLoggerOnce.Do(func() {
		previous := discordgo.Logger
		discordgo.Logger = func(msgL, caller int, format string, a ...interface{}) {
			msg := fmt.Sprintf(format, a...)
			captureVoiceGatewayClose(msgL, msg)
			if logger := currentLibraryLogger(); logger != nil {
				logDiscordgoMessage(logger, msgL, msg)
				return
			}
			if previous != nil {
				previous(msgL, caller, format, a...)
				return
			}
			logDiscordgoMessage(slog.Default(), msgL, msg)
		}
	})
}

func currentLibraryLogger() *slog.Logger {
	discordgoLoggerMu.RLock()
	defer discordgoLoggerMu.RUnlock()
	return discordgoLogger
}

func captureVoiceGatewayClose(msgLevel int, message string) {
	if msgLevel > discordgo.LogError {
		return
	}
	event, ok := parseVoiceGatewayCloseEvent(message)
	if !ok {
		return
	}
	voiceGatewayCloseMu.Lock()
	lastVoiceGatewayClose = event
	voiceGatewayCloseMu.Unlock()
}

func parseVoiceGatewayCloseEvent(message string) (voiceGatewayCloseEvent, bool) {
	if !strings.Contains(message, "voice endpoint") {
		return voiceGatewayCloseEvent{}, false
	}
	match := voiceGatewayClosePattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 3 {
		return voiceGatewayCloseEvent{}, false
	}
	code, err := strconv.Atoi(match[1])
	if err != nil {
		return voiceGatewayCloseEvent{}, false
	}
	return voiceGatewayCloseEvent{
		Code:   code,
		Reason: strings.TrimSpace(match[2]),
		Raw:    strings.TrimSpace(message),
		At:     time.Now().UTC(),
	}, true
}

func latestVoiceGatewayCloseSince(since time.Time) (voiceGatewayCloseEvent, bool) {
	voiceGatewayCloseMu.RLock()
	defer voiceGatewayCloseMu.RUnlock()
	if lastVoiceGatewayClose.At.IsZero() || lastVoiceGatewayClose.At.Before(since) {
		return voiceGatewayCloseEvent{}, false
	}
	return lastVoiceGatewayClose, true
}

func classifyVoiceJoinError(err error, channel Channel, closeEvent voiceGatewayCloseEvent, hasCloseEvent bool) error {
	if err == nil {
		return nil
	}
	if hasCloseEvent && closeEvent.Code == 4017 {
		channelLabel := strings.TrimSpace(channel.Name)
		if channelLabel == "" {
			channelLabel = channel.ID
		}
		return fmt.Errorf(
			"join voice: Discord voice gateway closed with code 4017 (%s) on %s; DAVE/E2EE required and the current discordgo transport is not compatible yet",
			closeEvent.Reason,
			channelLabel,
		)
	}
	return fmt.Errorf("join voice: %w", err)
}

func logDiscordgoMessage(logger *slog.Logger, msgLevel int, message string) {
	attrs := []any{"component", "discordgo", "message", strings.TrimSpace(message)}
	if event, ok := parseVoiceGatewayCloseEvent(message); ok {
		attrs = append(attrs, "voice.close_code", event.Code, "voice.close_reason", event.Reason)
	}
	logger.Log(context.Background(), discordgoLogLevel(msgLevel), "discord library", attrs...)
}

func discordgoLogLevel(level int) slog.Level {
	switch level {
	case discordgo.LogError:
		return slog.LevelError
	case discordgo.LogWarning:
		return slog.LevelWarn
	case discordgo.LogInformational:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}
