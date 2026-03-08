package discord

import (
	"fmt"
	"log"
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

	voiceGatewayClosePattern = regexp.MustCompile(`websocket: close ([0-9]+):\s*(.+)$`)
)

func installVoiceGatewayLogger() {
	voiceGatewayLoggerOnce.Do(func() {
		previous := discordgo.Logger
		discordgo.Logger = func(msgL, caller int, format string, a ...interface{}) {
			msg := fmt.Sprintf(format, a...)
			captureVoiceGatewayClose(msgL, msg)
			if previous != nil {
				previous(msgL, caller, format, a...)
				return
			}
			log.Printf("[DG%d] %s", msgL, msg)
		}
	})
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
