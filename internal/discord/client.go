package discord

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Service interface {
	Open() error
	Close() error
	AddMessageHandler(func(*discordgo.Session, *discordgo.MessageCreate))
	AddPresenceHandler(func(*discordgo.Session, *discordgo.PresenceUpdate))
	SendMessage(context.Context, string, string) (string, error)
	CreateTextChannel(context.Context, string, ChannelSpec) (Channel, error)
	EnsureTextChannel(context.Context, string, ChannelSpec) (Channel, error)
	EnsureCategory(context.Context, string, string) (Channel, error)
	MoveChannel(context.Context, string, string) error
	RecentMessages(context.Context, string, int) ([]Message, error)
	ListChannels(context.Context, string) ([]Channel, error)
	CurrentPresence(context.Context, string, string) (Presence, error)
	SelfUserID() string
}

type Client struct {
	session *discordgo.Session
}

type ChannelSpec struct {
	Name     string
	Topic    string
	ParentID string
}

type Channel struct {
	ID       string
	Name     string
	ParentID string
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
	Activities []string
}

func New(token string) (*Client, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildPresences |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsMessageContent
	return &Client{session: session}, nil
}

func (c *Client) Open() error {
	return c.session.Open()
}

func (c *Client) Close() error {
	return c.session.Close()
}

func (c *Client) AddMessageHandler(handler func(*discordgo.Session, *discordgo.MessageCreate)) {
	c.session.AddHandler(handler)
}

func (c *Client) AddPresenceHandler(handler func(*discordgo.Session, *discordgo.PresenceUpdate)) {
	c.session.AddHandler(handler)
}

func (c *Client) SendMessage(ctx context.Context, channelID string, content string) (string, error) {
	msg, err := c.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
	})
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
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
		return Channel{}, fmt.Errorf("create text channel: %w", err)
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
		return Channel{}, fmt.Errorf("create category: %w", err)
	}
	return toChannel(created), nil
}

func (c *Client) MoveChannel(ctx context.Context, channelID string, parentID string) error {
	_, err := c.session.ChannelEditComplex(channelID, &discordgo.ChannelEdit{
		ParentID: parentID,
	})
	if err != nil {
		return fmt.Errorf("move channel: %w", err)
	}
	return nil
}

func (c *Client) RecentMessages(ctx context.Context, channelID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	msgs, err := c.session.ChannelMessages(channelID, limit, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
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
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]Channel, 0, len(channels))
	for _, channel := range channels {
		out = append(out, toChannel(channel))
	}
	return out, nil
}

func (c *Client) CurrentPresence(ctx context.Context, guildID string, userID string) (Presence, error) {
	presence, err := c.session.State.Presence(guildID, userID)
	if err != nil {
		member, memberErr := c.session.GuildMember(guildID, userID)
		if memberErr != nil {
			return Presence{}, fmt.Errorf("presence lookup: %w", err)
		}
		return Presence{
			UserID: userID,
			Status: string(discordgo.StatusOffline),
			Activities: []string{
				member.User.Username,
			},
		}, nil
	}
	out := Presence{
		UserID: userID,
		Status: string(presence.Status),
	}
	for _, activity := range presence.Activities {
		out.Activities = append(out.Activities, activity.Name)
	}
	return out, nil
}

func (c *Client) SelfUserID() string {
	if c.session.State != nil && c.session.State.User != nil {
		return c.session.State.User.ID
	}
	return ""
}

func toChannel(channel *discordgo.Channel) Channel {
	return Channel{
		ID:       channel.ID,
		Name:     channel.Name,
		ParentID: channel.ParentID,
		Type:     channel.Type,
	}
}
