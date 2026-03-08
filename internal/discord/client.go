package discord

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	presencemodel "github.com/Sigumaa/yururi_personal/internal/presence"
	"github.com/bwmarrin/discordgo"
)

type Service interface {
	Open() error
	Close() error
	AddMessageHandler(func(*discordgo.Session, *discordgo.MessageCreate))
	AddPresenceHandler(func(*discordgo.Session, *discordgo.PresenceUpdate))
	AddVoiceStateHandler(func(*discordgo.Session, *discordgo.VoiceStateUpdate))
	SendMessage(context.Context, string, string) (string, error)
	CreateTextChannel(context.Context, string, ChannelSpec) (Channel, error)
	EnsureTextChannel(context.Context, string, ChannelSpec) (Channel, error)
	EnsureCategory(context.Context, string, string) (Channel, error)
	MoveChannel(context.Context, string, string) error
	GetChannel(context.Context, string) (Channel, error)
	RenameChannel(context.Context, string, string) (Channel, error)
	SetChannelTopic(context.Context, string, string) (Channel, error)
	RecentMessages(context.Context, string, int) ([]Message, error)
	ListChannels(context.Context, string) ([]Channel, error)
	ListVoiceChannels(context.Context, string) ([]VoiceChannel, error)
	VoiceChannelMembers(context.Context, string, string) ([]VoiceMember, error)
	CurrentMemberVoiceState(context.Context, string, string) (VoiceState, bool, error)
	JoinVoice(context.Context, string, string, bool, bool) (VoiceSession, error)
	LeaveVoice(context.Context, string) error
	CurrentVoiceSession(context.Context, string) (VoiceSession, bool, error)
	VoiceAudioPackets(context.Context, string) (<-chan VoicePacket, error)
	SendVoiceOpus(context.Context, string, []byte) error
	SetVoiceSpeaking(context.Context, string, bool) error
	CurrentPresence(context.Context, string, string) (Presence, error)
	SelfChannelPermissions(context.Context, string) (PermissionSnapshot, error)
	SelfUserID() string
}

type Client struct {
	session   *discordgo.Session
	voiceMu   sync.RWMutex
	voiceConn map[string]*discordgo.VoiceConnection
	voiceRT   map[string]*voiceRuntime
}

type ChannelSpec struct {
	Name     string
	Topic    string
	ParentID string
}

type Channel struct {
	ID       string
	Name     string
	Topic    string
	ParentID string
	Position int
	Type     discordgo.ChannelType
}

type Message struct {
	ID          string
	ChannelID   string
	AuthorID    string
	AuthorName  string
	Content     string
	CreatedAt   time.Time
	ChannelName string
}

type Presence struct {
	UserID     string
	Status     string
	Activities []presencemodel.Activity
}

type PermissionSnapshot struct {
	UserID         string
	ChannelID      string
	Raw            int64
	ViewChannel    bool
	SendMessages   bool
	ManageChannels bool
}

type VoiceMember struct {
	UserID           string
	Username         string
	Bot              bool
	ChannelID        string
	Muted            bool
	Deafened         bool
	SelfMuted        bool
	SelfDeafened     bool
	Suppressed       bool
	RequestToSpeakAt *time.Time
}

type VoiceState struct {
	UserID           string
	Username         string
	Bot              bool
	ChannelID        string
	Muted            bool
	Deafened         bool
	SelfMuted        bool
	SelfDeafened     bool
	Suppressed       bool
	RequestToSpeakAt *time.Time
}

type VoiceChannel struct {
	ID          string
	Name        string
	ParentID    string
	Type        discordgo.ChannelType
	MemberCount int
	Members     []VoiceMember
}

type VoiceSession struct {
	GuildID     string
	ChannelID   string
	ChannelName string
	Connected   bool
	SelfMute    bool
	SelfDeaf    bool
}

type VoicePacket struct {
	GuildID   string
	ChannelID string
	UserID    string
	Username  string
	SSRC      uint32
	Sequence  uint16
	Timestamp uint32
	Opus      []byte
}

func New(token string) (*Client, error) {
	installVoiceGatewayLogger()
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	session.LogLevel = discordgo.LogInformational
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildPresences |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuildVoiceStates |
		discordgo.IntentsMessageContent
	return &Client{
		session:   session,
		voiceConn: map[string]*discordgo.VoiceConnection{},
		voiceRT:   map[string]*voiceRuntime{},
	}, nil
}

func (c *Client) Open() error {
	return c.session.Open()
}

func (c *Client) Close() error {
	c.voiceMu.Lock()
	for guildID, conn := range c.voiceConn {
		if runtime := c.voiceRT[guildID]; runtime != nil {
			runtime.close()
			delete(c.voiceRT, guildID)
		}
		_ = conn.Disconnect()
		delete(c.voiceConn, guildID)
	}
	c.voiceMu.Unlock()
	return c.session.Close()
}

func (c *Client) AddMessageHandler(handler func(*discordgo.Session, *discordgo.MessageCreate)) {
	c.session.AddHandler(handler)
}

func (c *Client) AddPresenceHandler(handler func(*discordgo.Session, *discordgo.PresenceUpdate)) {
	c.session.AddHandler(handler)
}

func (c *Client) AddVoiceStateHandler(handler func(*discordgo.Session, *discordgo.VoiceStateUpdate)) {
	c.session.AddHandler(handler)
}

func (c *Client) SendMessage(ctx context.Context, channelID string, content string) (string, error) {
	msg, err := c.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
	})
	if err != nil {
		return "", wrapDiscordError("send message", err)
	}
	return msg.ID, nil
}

func (c *Client) CreateTextChannel(ctx context.Context, guildID string, spec ChannelSpec) (Channel, error) {
	channel, err := c.session.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     spec.Name,
		Type:     discordgo.ChannelTypeGuildText,
		Topic:    spec.Topic,
		ParentID: spec.ParentID,
	})
	if err != nil {
		return Channel{}, wrapDiscordError("create text channel", err)
	}
	return toChannel(channel), nil
}

func (c *Client) EnsureTextChannel(ctx context.Context, guildID string, spec ChannelSpec) (Channel, error) {
	channels, err := c.ListChannels(ctx, guildID)
	if err != nil {
		return Channel{}, err
	}
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildText && strings.EqualFold(channel.Name, spec.Name) {
			if spec.ParentID != "" && channel.ParentID != spec.ParentID {
				if err := c.MoveChannel(ctx, channel.ID, spec.ParentID); err != nil {
					return Channel{}, err
				}
				channel.ParentID = spec.ParentID
			}
			return channel, nil
		}
	}
	return c.CreateTextChannel(ctx, guildID, spec)
}

func (c *Client) EnsureCategory(ctx context.Context, guildID string, name string) (Channel, error) {
	channels, err := c.ListChannels(ctx, guildID)
	if err != nil {
		return Channel{}, err
	}
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory && strings.EqualFold(channel.Name, name) {
			return channel, nil
		}
	}
	created, err := c.session.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name: name,
		Type: discordgo.ChannelTypeGuildCategory,
	})
	if err != nil {
		return Channel{}, wrapDiscordError("create category", err)
	}
	return toChannel(created), nil
}

func (c *Client) MoveChannel(ctx context.Context, channelID string, parentID string) error {
	_, err := c.session.ChannelEditComplex(channelID, &discordgo.ChannelEdit{
		ParentID: parentID,
	})
	if err != nil {
		return wrapDiscordError("move channel", err)
	}
	return nil
}

func (c *Client) GetChannel(ctx context.Context, channelID string) (Channel, error) {
	channel, err := c.session.Channel(channelID)
	if err != nil {
		return Channel{}, wrapDiscordError("get channel", err)
	}
	return toChannel(channel), nil
}

func (c *Client) RenameChannel(ctx context.Context, channelID string, name string) (Channel, error) {
	channel, err := c.session.ChannelEditComplex(channelID, &discordgo.ChannelEdit{
		Name: name,
	})
	if err != nil {
		return Channel{}, wrapDiscordError("rename channel", err)
	}
	return toChannel(channel), nil
}

func (c *Client) SetChannelTopic(ctx context.Context, channelID string, topic string) (Channel, error) {
	channel, err := c.session.ChannelEditComplex(channelID, &discordgo.ChannelEdit{
		Topic: topic,
	})
	if err != nil {
		return Channel{}, wrapDiscordError("set channel topic", err)
	}
	return toChannel(channel), nil
}

func (c *Client) RecentMessages(ctx context.Context, channelID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	msgs, err := c.session.ChannelMessages(channelID, limit, "", "", "")
	if err != nil {
		return nil, wrapDiscordError("fetch messages", err)
	}
	channelName := ""
	if channel, err := c.session.State.Channel(channelID); err == nil && channel != nil {
		channelName = channel.Name
	}

	out := make([]Message, 0, len(msgs))
	slices.Reverse(msgs)
	for _, msg := range msgs {
		out = append(out, Message{
			ID:          msg.ID,
			ChannelID:   msg.ChannelID,
			AuthorID:    msg.Author.ID,
			AuthorName:  msg.Author.Username,
			Content:     msg.Content,
			CreatedAt:   msg.Timestamp,
			ChannelName: channelName,
		})
	}
	return out, nil
}

func (c *Client) ListChannels(ctx context.Context, guildID string) ([]Channel, error) {
	channels, err := c.session.GuildChannels(guildID)
	if err != nil {
		return nil, wrapDiscordError("list channels", err)
	}
	out := make([]Channel, 0, len(channels))
	for _, channel := range channels {
		out = append(out, toChannel(channel))
	}
	return out, nil
}

func (c *Client) ListVoiceChannels(ctx context.Context, guildID string) ([]VoiceChannel, error) {
	channels, err := c.ListChannels(ctx, guildID)
	if err != nil {
		return nil, err
	}
	out := make([]VoiceChannel, 0, len(channels))
	for _, channel := range channels {
		if channel.Type != discordgo.ChannelTypeGuildVoice && channel.Type != discordgo.ChannelTypeGuildStageVoice {
			continue
		}
		members, err := c.VoiceChannelMembers(ctx, guildID, channel.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, VoiceChannel{
			ID:          channel.ID,
			Name:        channel.Name,
			ParentID:    channel.ParentID,
			Type:        channel.Type,
			MemberCount: len(members),
			Members:     members,
		})
	}
	return out, nil
}

func (c *Client) VoiceChannelMembers(ctx context.Context, guildID string, channelID string) ([]VoiceMember, error) {
	guild, err := c.guildState(ctx, guildID)
	if err != nil {
		return nil, err
	}
	out := make([]VoiceMember, 0, len(guild.VoiceStates))
	for _, state := range guild.VoiceStates {
		if state == nil || state.ChannelID != channelID {
			continue
		}
		out = append(out, c.voiceMemberFromState(guild, state))
	}
	slices.SortFunc(out, func(a VoiceMember, b VoiceMember) int {
		switch {
		case strings.ToLower(a.Username) < strings.ToLower(b.Username):
			return -1
		case strings.ToLower(a.Username) > strings.ToLower(b.Username):
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (c *Client) CurrentMemberVoiceState(ctx context.Context, guildID string, userID string) (VoiceState, bool, error) {
	guild, err := c.guildState(ctx, guildID)
	if err != nil {
		return VoiceState{}, false, err
	}
	for _, state := range guild.VoiceStates {
		if state == nil || state.UserID != userID || strings.TrimSpace(state.ChannelID) == "" {
			continue
		}
		member := c.voiceMemberFromState(guild, state)
		return VoiceState(member), true, nil
	}
	return VoiceState{}, false, nil
}

func (c *Client) CurrentVoiceSession(ctx context.Context, guildID string) (VoiceSession, bool, error) {
	c.voiceMu.RLock()
	conn := c.voiceConn[guildID]
	c.voiceMu.RUnlock()
	if conn == nil {
		return VoiceSession{}, false, nil
	}
	channel, err := c.GetChannel(ctx, conn.ChannelID)
	if err != nil {
		return VoiceSession{}, false, err
	}
	return VoiceSession{
		GuildID:     guildID,
		ChannelID:   conn.ChannelID,
		ChannelName: channel.Name,
		Connected:   conn.Ready,
		SelfMute:    false,
		SelfDeaf:    false,
	}, true, nil
}

func (c *Client) CurrentPresence(ctx context.Context, guildID string, userID string) (Presence, error) {
	presence, err := c.session.State.Presence(guildID, userID)
	if err != nil {
		return Presence{
			UserID:     userID,
			Status:     string(discordgo.StatusOffline),
			Activities: nil,
		}, nil
	}
	out := Presence{
		UserID: userID,
		Status: string(presence.Status),
	}
	out.Activities = ActivitiesFromGateway(presence.Activities)
	return out, nil
}

func ActivitiesFromGateway(items []*discordgo.Activity) []presencemodel.Activity {
	if len(items) == 0 {
		return nil
	}
	out := make([]presencemodel.Activity, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, presencemodel.Activity{
			Name:          strings.TrimSpace(item.Name),
			Type:          activityTypeName(item.Type),
			URL:           strings.TrimSpace(item.URL),
			ApplicationID: strings.TrimSpace(item.ApplicationID),
			State:         strings.TrimSpace(item.State),
			Details:       strings.TrimSpace(item.Details),
			LargeText:     strings.TrimSpace(item.Assets.LargeText),
			SmallText:     strings.TrimSpace(item.Assets.SmallText),
			StartAt:       toActivityTime(item.Timestamps.StartTimestamp),
			EndAt:         toActivityTime(item.Timestamps.EndTimestamp),
		})
	}
	return out
}

func activityTypeName(kind discordgo.ActivityType) string {
	switch kind {
	case discordgo.ActivityTypeGame:
		return "playing"
	case discordgo.ActivityTypeStreaming:
		return "streaming"
	case discordgo.ActivityTypeListening:
		return "listening"
	case discordgo.ActivityTypeWatching:
		return "watching"
	case discordgo.ActivityTypeCustom:
		return "custom"
	case discordgo.ActivityTypeCompeting:
		return "competing"
	default:
		return fmt.Sprintf("type_%d", kind)
	}
}

func toActivityTime(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}

func (c *Client) SelfChannelPermissions(ctx context.Context, channelID string) (PermissionSnapshot, error) {
	userID := c.SelfUserID()
	if strings.TrimSpace(userID) == "" {
		return PermissionSnapshot{}, fmt.Errorf("self user is unavailable")
	}
	raw, err := c.session.UserChannelPermissions(userID, channelID)
	if err != nil {
		return PermissionSnapshot{}, wrapDiscordError("self channel permissions", err)
	}
	return PermissionSnapshot{
		UserID:         userID,
		ChannelID:      channelID,
		Raw:            raw,
		ViewChannel:    raw&discordgo.PermissionViewChannel != 0,
		SendMessages:   raw&discordgo.PermissionSendMessages != 0,
		ManageChannels: raw&discordgo.PermissionManageChannels != 0,
	}, nil
}

func (c *Client) SelfUserID() string {
	if c.session.State != nil && c.session.State.User != nil {
		return c.session.State.User.ID
	}
	return ""
}

func wrapDiscordError(operation string, err error) error {
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) {
		if restErr.Message != nil {
			return fmt.Errorf("%s: http=%s discord_code=%d discord_message=%s", operation, restErr.Response.Status, restErr.Message.Code, strings.TrimSpace(restErr.Message.Message))
		}
		body := strings.TrimSpace(string(restErr.ResponseBody))
		if body != "" {
			return fmt.Errorf("%s: http=%s body=%s", operation, restErr.Response.Status, truncateDiscordErrorBody(body, 240))
		}
		return fmt.Errorf("%s: http=%s", operation, restErr.Response.Status)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (c *Client) guildState(ctx context.Context, guildID string) (*discordgo.Guild, error) {
	if c.session.State != nil {
		if guild, err := c.session.State.Guild(guildID); err == nil && guild != nil {
			return guild, nil
		}
	}
	guild, err := c.session.Guild(guildID)
	if err != nil {
		return nil, wrapDiscordError("get guild", err)
	}
	return guild, nil
}

func (c *Client) voiceMemberFromState(guild *discordgo.Guild, state *discordgo.VoiceState) VoiceMember {
	member := VoiceMember{
		UserID:       state.UserID,
		ChannelID:    state.ChannelID,
		Muted:        state.Mute,
		Deafened:     state.Deaf,
		SelfMuted:    state.SelfMute,
		SelfDeafened: state.SelfDeaf,
		Suppressed:   state.Suppress,
	}
	if state.RequestToSpeakTimestamp != nil {
		requestedAt := state.RequestToSpeakTimestamp.UTC()
		member.RequestToSpeakAt = &requestedAt
	}
	if guild == nil {
		return member
	}
	for _, guildMember := range guild.Members {
		if guildMember == nil || guildMember.User == nil || guildMember.User.ID != state.UserID {
			continue
		}
		member.Username = guildMember.User.Username
		member.Bot = guildMember.User.Bot
		break
	}
	return member
}

func truncateDiscordErrorBody(value string, maxChars int) string {
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	return strings.TrimSpace(value[:maxChars]) + "..."
}

func toChannel(channel *discordgo.Channel) Channel {
	return Channel{
		ID:       channel.ID,
		Name:     channel.Name,
		Topic:    channel.Topic,
		ParentID: channel.ParentID,
		Position: channel.Position,
		Type:     channel.Type,
	}
}
