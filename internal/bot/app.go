package bot

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/config"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	runtimecfg "github.com/Sigumaa/yururi_personal/internal/runtime"
	"github.com/bwmarrin/discordgo"
)

type App struct {
	cfg    config.Config
	paths  config.Paths
	logger *slog.Logger
	loc    *time.Location

	store     *memory.Store
	tools     *codex.ToolRegistry
	codex     *codex.Client
	discord   discordsvc.Service
	scheduler *jobs.Scheduler
	http      *http.Client

	sessionMu      sync.Mutex
	channelThreads map[string]codex.ThreadSession
	threadMu       sync.Mutex
	threadLocks    map[string]*sync.Mutex
	thread         codex.ThreadSession
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	paths, err := runtimecfg.EnsureLayout(cfg)
	if err != nil {
		return nil, err
	}
	loc, err := cfg.Location()
	if err != nil {
		return nil, err
	}
	store, err := memory.Open(paths.DBPath)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:            cfg,
		paths:          paths,
		logger:         logger,
		loc:            loc,
		store:          store,
		channelThreads: map[string]codex.ThreadSession{},
		threadLocks:    map[string]*sync.Mutex{},
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	tools := codex.NewToolRegistry()
	app.registerTools(tools)
	tools.SetHooks(app.beforeToolCall, app.afterToolCall)
	app.tools = tools
	app.codex = codex.NewClient(cfg, paths, logger, tools)
	app.scheduler = jobs.NewScheduler(store, defaultSchedulerPollInterval)
	app.scheduler.SetLogger(logger)
	app.scheduler.SetObserver(app.handleJobResult)
	app.registerJobHandlers()

	return app, nil
}

func (a *App) Close() error {
	var errs []error
	if a.scheduler != nil {
		// no explicit close
	}
	if a.codex != nil {
		errs = append(errs, a.codex.Close())
	}
	if a.discord != nil {
		errs = append(errs, a.discord.Close())
	}
	if a.store != nil {
		errs = append(errs, a.store.Close())
	}
	return errors.Join(errs...)
}

func (a *App) Bootstrap(ctx context.Context) error {
	a.logger.Info("bootstrap start", "runtime_root", a.paths.Root)
	if _, err := runtimecfg.EnsureLayout(a.cfg); err != nil {
		return err
	}
	if err := a.syncBotContext(); err != nil {
		return err
	}
	if _, err := a.codex.Bootstrap(ctx); err != nil {
		a.logger.Warn("codex bootstrap skipped", "error", err)
	}
	a.logger.Info("bootstrap ready", "workspace", a.paths.Workspace, "tool_count", len(a.tools.Specs()))
	return nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.cfg.ValidateServe(); err != nil {
		return err
	}
	if err := a.Bootstrap(ctx); err != nil {
		return err
	}

	threadID := a.storedThreadID(ctx, "codex.main_thread_id")
	if session, err := a.codex.EnsureThread(ctx, threadID, baseInstructions(), developerInstructions()); err != nil {
		a.logger.Warn("codex thread unavailable", "error", err)
	} else {
		a.thread = session
		_ = a.persistThreadBinding(ctx, "codex.main_thread_id", session.ID)
		a.logger.Info("codex thread ready", "thread_id", session.ID)
		if err := a.primeBotContext(ctx); err != nil {
			a.logger.Warn("prime bot context failed", "error", err)
		}
	}

	discordClient, err := discordsvc.New(a.cfg.Discord.Token)
	if err != nil {
		return err
	}
	a.discord = discordClient
	a.discord.AddMessageHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		go a.processMessage(s, m)
	})
	a.discord.AddPresenceHandler(func(s *discordgo.Session, p *discordgo.PresenceUpdate) {
		go a.processPresence(p)
	})
	if err := a.discord.Open(); err != nil {
		return err
	}
	a.logger.Info("discord connected", "guild_id", a.cfg.Discord.GuildID, "self_user_id", a.discord.SelfUserID())
	if err := a.ensureAutonomyPulseJob(context.Background()); err != nil {
		a.logger.Warn("ensure autonomy pulse job failed", "error", err)
	}

	go func() {
		err := a.scheduler.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("scheduler failed", "error", err)
		}
	}()

	<-ctx.Done()
	a.logger.Info("run loop stopped")
	return nil
}

func (a *App) processMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	if event.Author == nil || event.Author.Bot {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	channelName := a.channelName(session, event.ChannelID)
	msg := memory.Message{
		ID:          event.ID,
		ChannelID:   event.ChannelID,
		ChannelName: channelName,
		AuthorID:    event.Author.ID,
		AuthorName:  event.Author.Username,
		Content:     event.Content,
		CreatedAt:   event.Timestamp,
		Metadata: map[string]any{
			"guild_id":    event.GuildID,
			"attachments": attachmentURLs(event.Attachments),
		},
	}
	a.logger.Info("message received", "channel", channelName, "channel_id", event.ChannelID, "author", event.Author.Username, "message_id", event.ID, "attachments", len(event.Attachments), "content_preview", previewText(event.Content, 240))
	if err := a.store.SaveMessage(ctx, msg); err != nil {
		a.logger.Error("save message failed", "error", err)
		return
	}
	if msg.AuthorID == a.cfg.Discord.OwnerUserID {
		if err := a.store.SetKV(ctx, "autonomy.last_target_channel_id", msg.ChannelID); err != nil {
			a.logger.Warn("save autonomy target channel failed", "channel_id", msg.ChannelID, "error", err)
		}
	}

	profile, err := a.resolveChannelProfile(ctx, event.ChannelID, channelName)
	if err != nil {
		a.logger.Error("resolve channel profile failed", "error", err)
		return
	}

	recent, _ := a.store.RecentMessages(ctx, event.ChannelID, 12)
	facts, _ := a.collectConversationFacts(ctx, msg, 12)
	a.logger.Info("message context ready", "channel", channelName, "recent_messages", len(recent), "related_facts", len(facts), "profile_kind", profile.Kind)
	a.logger.Debug("message context detail", "channel", channelName, "profile", previewJSON(profile, 320), "recent_preview", previewJSON(recent, 900), "fact_preview", previewJSON(facts, 900))

	threadSession, err := a.ensureChannelThread(ctx, msg.ChannelID)
	if err != nil {
		a.logger.Error("ensure channel thread failed", "channel", channelName, "channel_id", msg.ChannelID, "error", err)
		return
	}
	if err := a.interruptChannelTurn(ctx, msg.ChannelID, threadSession.ID, msg.ID); err != nil {
		a.logger.Warn("interrupt active channel turn failed", "channel", channelName, "channel_id", msg.ChannelID, "thread_id", threadSession.ID, "error", err)
	}

	reply, err := a.runConversationTurn(ctx, threadSession.ID, msg, profile, recent, facts, imageAttachmentURLs(event.Attachments))
	if err != nil {
		if errors.Is(err, codex.ErrTurnInterrupted) {
			a.logger.Info("conversation turn interrupted", "channel", channelName, "channel_id", msg.ChannelID, "message_id", msg.ID)
			return
		}
		a.logger.Error("conversation turn failed", "channel", channelName, "channel_id", msg.ChannelID, "error", err)
		return
	}
	if strings.TrimSpace(reply) == "" {
		a.logger.Info("reply skipped", "channel", channelName, "message_id", event.ID, "reason", "empty_or_no_reply")
		return
	}
	sentID, err := a.discord.SendMessage(ctx, msg.ChannelID, reply)
	if err != nil {
		a.logger.Error("send reply failed", "error", err)
		return
	}
	a.logger.Info("reply sent", "channel", msg.ChannelName, "channel_id", msg.ChannelID, "message_id", sentID)
}

func (a *App) interruptChannelTurn(ctx context.Context, channelID string, threadID string, messageID string) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}
	interruptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	turnID, ok, err := a.codex.InterruptActiveTurn(interruptCtx, threadID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	a.logger.Info("active channel turn interrupted", "channel_id", channelID, "thread_id", threadID, "interrupted_turn_id", turnID, "next_message_id", messageID)
	return nil
}

func (a *App) processPresence(event *discordgo.PresenceUpdate) {
	if event.User == nil || event.User.ID != a.cfg.Discord.OwnerUserID {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	previous, ok, err := a.store.LastPresence(ctx, event.User.ID)
	if err != nil {
		a.logger.Warn("load last presence failed", "error", err)
	}

	activities := make([]string, 0, len(event.Activities))
	for _, activity := range event.Activities {
		activities = append(activities, activity.Name)
	}
	now := time.Now().UTC()
	current := memory.PresenceSnapshot{
		UserID:     event.User.ID,
		Status:     string(event.Status),
		Activities: activities,
		StartedAt:  now,
	}
	if err := a.store.SavePresence(ctx, current); err != nil {
		a.logger.Warn("save presence failed", "error", err)
	}
	a.logger.Info("presence updated", "status", current.Status, "activities", strings.Join(current.Activities, ","))

	if ok && isOffline(previous.Status) && !isOffline(current.Status) {
		threshold := defaultWakeSummaryThreshold
		if now.Sub(previous.StartedAt) >= threshold {
			channelID, found, err := a.store.LatestChannelIDForAuthor(ctx, a.cfg.Discord.OwnerUserID)
			if err != nil {
				a.logger.Warn("resolve wake summary channel failed", "error", err)
				return
			}
			if !found {
				return
			}
			payload := map[string]any{
				"since": previous.StartedAt.Format(time.RFC3339Nano),
			}
			job := jobs.NewJob(jobID("wake-summary"), "wake_summary", "wake summary", channelID, "10s", payload)
			job.NextRunAt = now.Add(10 * time.Second)
			if err := a.store.UpsertJob(ctx, job); err != nil {
				a.logger.Warn("schedule wake summary failed", "error", err)
			} else {
				a.logger.Info("wake summary scheduled", "job_id", job.ID, "channel_id", channelID)
			}
		}
	}
}

func (a *App) resolveChannelProfile(ctx context.Context, channelID string, channelName string) (memory.ChannelProfile, error) {
	profile, ok, err := a.store.GetChannelProfile(ctx, channelID)
	if err != nil {
		return memory.ChannelProfile{}, err
	}
	if ok {
		a.logger.Debug("channel profile reused", "channel", channelName, "channel_id", channelID, "profile", previewJSON(profile, 320))
		return profile, nil
	}

	kind := "conversation"
	replyAgg := 0.75
	autonomy := 0.55
	cadence := "daily"

	profile = memory.ChannelProfile{
		ChannelID:           channelID,
		Name:                channelName,
		Kind:                kind,
		ReplyAggressiveness: replyAgg,
		AutonomyLevel:       autonomy,
		SummaryCadence:      cadence,
	}
	if err := a.store.UpsertChannelProfile(ctx, profile); err != nil {
		return memory.ChannelProfile{}, err
	}
	a.logger.Info("channel profile created", "channel", channelName, "channel_id", channelID, "kind", kind, "reply_aggressiveness", replyAgg, "autonomy_level", autonomy)
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
