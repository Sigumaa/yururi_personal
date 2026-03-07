package bot

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (a *App) processVoiceState(event *discordgo.VoiceStateUpdate) {
	if a.voice == nil || event == nil || event.VoiceState == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.voice.HandleVoiceStateUpdate(ctx, event); err != nil {
		a.logger.Warn("voice state update failed", "guild_id", event.GuildID, "user_id", event.UserID, "channel_id", event.ChannelID, "error", err)
		return
	}

	if event.UserID != a.cfg.Discord.OwnerUserID {
		return
	}
	beforeChannelID := ""
	if event.BeforeUpdate != nil {
		beforeChannelID = strings.TrimSpace(event.BeforeUpdate.ChannelID)
	}
	afterChannelID := strings.TrimSpace(event.ChannelID)
	a.logger.Info("owner voice state updated", "before_channel_id", beforeChannelID, "after_channel_id", afterChannelID)
}
