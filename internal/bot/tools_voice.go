package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) registerVoiceDiscordTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "discord.list_voice_channels",
		Description: "サーバー内のボイスチャンネル一覧と参加メンバー数を取得する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListVoiceChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(channels))
		for _, channel := range channels {
			memberNames := make([]string, 0, len(channel.Members))
			for _, member := range channel.Members {
				memberNames = append(memberNames, member.Username)
			}
			if len(memberNames) == 0 {
				memberNames = append(memberNames, "none")
			}
			lines = append(lines, fmt.Sprintf("- %s id=%s members=%d [%s]", channel.Name, channel.ID, channel.MemberCount, strings.Join(memberNames, ", ")))
		}
		if len(lines) == 0 {
			lines = append(lines, "no voice channels")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.get_voice_channel_members",
		Description: "指定したボイスチャンネルにいるメンバー一覧を取得する",
		InputSchema: objectSchema(fieldSchema("channel_id", "string", "ボイスチャンネル ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}
		members, err := a.discord.VoiceChannelMembers(ctx, a.cfg.Discord.GuildID, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(members))
		for _, member := range members {
			lines = append(lines, fmt.Sprintf("- %s id=%s bot=%t muted=%t self_muted=%t", member.Username, member.UserID, member.Bot, member.Muted, member.SelfMuted))
		}
		if len(lines) == 0 {
			lines = append(lines, "no members")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.join_voice",
		Description: "指定したボイスチャンネルへ参加する。channel_id が無ければ user_id の現在の VC を使う",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "参加先ボイスチャンネル ID"),
			fieldSchema("user_id", "string", "このユーザーが入っている VC に参加するためのユーザー ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.voice == nil || a.discord == nil {
			return codex.ToolResponse{}, errors.New("voice is not available")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			UserID    string `json:"user_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channelID := strings.TrimSpace(input.ChannelID)
		if channelID == "" {
			userID := strings.TrimSpace(input.UserID)
			if userID == "" {
				userID = a.cfg.Discord.OwnerUserID
			}
			state, ok, err := a.discord.CurrentMemberVoiceState(ctx, a.cfg.Discord.GuildID, userID)
			if err != nil {
				return codex.ToolResponse{}, err
			}
			if !ok {
				return codex.ToolResponse{}, errors.New("target user is not in voice")
			}
			channelID = state.ChannelID
		}
		session, err := a.voice.Join(ctx, a.cfg.Discord.GuildID, channelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("joined voice channel %s (%s) realtime_connected=%t", session.ChannelName, session.ChannelID, session.Realtime.Connected)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.leave_voice",
		Description: "現在参加しているボイスチャンネルから退出する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.voice == nil {
			return codex.ToolResponse{}, errors.New("voice is not available")
		}
		if err := a.voice.Leave(ctx, a.cfg.Discord.GuildID, "tool request"); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("left voice"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.voice_session_status",
		Description: "現在の VC session 状態、参加チャンネル、メンバー、Realtime 接続状態を確認する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.voice == nil {
			return codex.ToolResponse{}, errors.New("voice is not available")
		}
		session, ok, err := a.voice.Status(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return textTool("no active voice session"), nil
		}
		memberNames := make([]string, 0, len(session.Members))
		for _, member := range session.Members {
			memberNames = append(memberNames, member.Username)
		}
		if len(memberNames) == 0 {
			memberNames = append(memberNames, "none")
		}
		return textTool(fmt.Sprintf(
			"session_id=%s\nchannel=%s (%s)\nstate=%s\nmembers=%s\nrealtime_configured=%t\nrealtime_connected=%t\nrealtime_model=%s\nrealtime_last_error=%s",
			session.ID,
			session.ChannelName,
			session.ChannelID,
			session.State,
			strings.Join(memberNames, ", "),
			session.Realtime.Configured,
			session.Realtime.Connected,
			session.Realtime.Model,
			session.Realtime.LastError,
		)), nil
	})
}
