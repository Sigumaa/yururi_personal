package bot

import (
	"context"

	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func (a *App) resolveChannelProfile(ctx context.Context, channelID string, channelName string) (memory.ChannelProfile, error) {
	profile, ok, err := a.store.GetChannelProfile(ctx, channelID)
	if err != nil {
		return memory.ChannelProfile{}, err
	}
	if ok {
		a.logger.Debug("channel profile reused", "channel", channelName, "channel_id", channelID, "profile", previewJSON(profile, 320))
		return profile, nil
	}

	profile = memory.ChannelProfile{
		ChannelID:           channelID,
		Name:                channelName,
		Kind:                "conversation",
		ReplyAggressiveness: 0.75,
		AutonomyLevel:       0.55,
		SummaryCadence:      "daily",
	}
	if err := a.store.UpsertChannelProfile(ctx, profile); err != nil {
		return memory.ChannelProfile{}, err
	}
	a.logger.Info("channel profile created", "channel", channelName, "channel_id", channelID, "kind", profile.Kind, "reply_aggressiveness", profile.ReplyAggressiveness, "autonomy_level", profile.AutonomyLevel)
	return profile, nil
}

func (a *App) channelName(session *discordgo.Session, channelID string) string {
	if session.State != nil {
		if channel, err := session.State.Channel(channelID); err == nil && channel != nil {
			return channel.Name
		}
	}
	channel, err := session.Channel(channelID)
	if err != nil || channel == nil {
		return channelID
	}
	return channel.Name
}

func (a *App) discordSelfMention() string {
	if a.discord == nil {
		return ""
	}
	if id := a.discord.SelfUserID(); id != "" {
		return "<@" + id + ">"
	}
	return ""
}
