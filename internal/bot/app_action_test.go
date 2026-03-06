package bot

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/decision"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/bwmarrin/discordgo"
)

type discordStub struct {
	sentChannel  string
	sentContent  string
	sentMessages []sentMessage
}

type sentMessage struct {
	ChannelID string
	Content   string
}

func (d *discordStub) Open() error                                                            { return nil }
func (d *discordStub) Close() error                                                           { return nil }
func (d *discordStub) AddMessageHandler(func(*discordgo.Session, *discordgo.MessageCreate))   {}
func (d *discordStub) AddPresenceHandler(func(*discordgo.Session, *discordgo.PresenceUpdate)) {}
func (d *discordStub) SendMessage(_ context.Context, channelID string, content string) (string, error) {
	d.sentChannel = channelID
	d.sentContent = content
	d.sentMessages = append(d.sentMessages, sentMessage{ChannelID: channelID, Content: content})
	return "m-1", nil
}
func (d *discordStub) CreateTextChannel(context.Context, string, discordsvc.ChannelSpec) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordStub) EnsureTextChannel(context.Context, string, discordsvc.ChannelSpec) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordStub) EnsureCategory(context.Context, string, string) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordStub) MoveChannel(context.Context, string, string) error { return nil }
func (d *discordStub) GetChannel(context.Context, string) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordStub) RenameChannel(context.Context, string, string) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordStub) SetChannelTopic(context.Context, string, string) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordStub) RecentMessages(context.Context, string, int) ([]discordsvc.Message, error) {
	return nil, nil
}
func (d *discordStub) ListChannels(context.Context, string) ([]discordsvc.Channel, error) {
	return nil, nil
}
func (d *discordStub) CurrentPresence(context.Context, string, string) (discordsvc.Presence, error) {
	return discordsvc.Presence{}, nil
}
func (d *discordStub) SelfChannelPermissions(context.Context, string) (discordsvc.PermissionSnapshot, error) {
	return discordsvc.PermissionSnapshot{UserID: "bot", ChannelID: "c-1", ManageChannels: true, ViewChannel: true, SendMessages: true}, nil
}
func (d *discordStub) SelfUserID() string { return "" }

func TestSendActionAnnouncementUsesAnnouncementText(t *testing.T) {
	discord := &discordStub{}
	app := &App{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		discord: discord,
	}

	err := app.sendActionAnnouncement(context.Background(), "c-1", decision.ServerAction{
		Type:             "create_channel",
		AnnouncementText: "ちょっと整えてみますね。",
	})
	if err != nil {
		t.Fatalf("sendActionAnnouncement: %v", err)
	}
	if discord.sentChannel != "c-1" {
		t.Fatalf("unexpected channel: %s", discord.sentChannel)
	}
	if discord.sentContent != "ちょっと整えてみますね。" {
		t.Fatalf("unexpected content: %s", discord.sentContent)
	}
}
