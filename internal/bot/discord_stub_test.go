package bot

import (
	"context"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/bwmarrin/discordgo"
)

type discordStub struct {
	sentChannel  string
	sentContent  string
	sentMessages []sentMessage
	channels     []discordsvc.Channel
}

type sentMessage struct {
	ChannelID string
	Content   string
}

func (d *discordStub) Open() error                                                            { return nil }
func (d *discordStub) Close() error                                                           { return nil }
func (d *discordStub) AddMessageHandler(func(*discordgo.Session, *discordgo.MessageCreate))   {}
func (d *discordStub) AddPresenceHandler(func(*discordgo.Session, *discordgo.PresenceUpdate)) {}
func (d *discordStub) AddVoiceStateHandler(func(*discordgo.Session, *discordgo.VoiceStateUpdate)) {
}
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
	if len(d.channels) > 0 {
		return d.channels[0], nil
	}
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
	return d.channels, nil
}
func (d *discordStub) ListVoiceChannels(context.Context, string) ([]discordsvc.VoiceChannel, error) {
	return nil, nil
}
func (d *discordStub) VoiceChannelMembers(context.Context, string, string) ([]discordsvc.VoiceMember, error) {
	return nil, nil
}
func (d *discordStub) CurrentMemberVoiceState(context.Context, string, string) (discordsvc.VoiceState, bool, error) {
	return discordsvc.VoiceState{}, false, nil
}
func (d *discordStub) JoinVoice(context.Context, string, string, bool, bool) (discordsvc.VoiceSession, error) {
	return discordsvc.VoiceSession{}, nil
}
func (d *discordStub) LeaveVoice(context.Context, string) error { return nil }
func (d *discordStub) CurrentVoiceSession(context.Context, string) (discordsvc.VoiceSession, bool, error) {
	return discordsvc.VoiceSession{}, false, nil
}
func (d *discordStub) CurrentPresence(context.Context, string, string) (discordsvc.Presence, error) {
	return discordsvc.Presence{}, nil
}
func (d *discordStub) SelfChannelPermissions(context.Context, string) (discordsvc.PermissionSnapshot, error) {
	return discordsvc.PermissionSnapshot{UserID: "bot", ChannelID: "c-1", ManageChannels: true, ViewChannel: true, SendMessages: true}, nil
}
func (d *discordStub) SelfUserID() string { return "" }
