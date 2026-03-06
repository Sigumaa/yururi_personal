package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/config"
	"github.com/Sigumaa/yururi_personal/internal/decision"
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
	codex     *codex.Client
	discord   discordsvc.Service
	scheduler *jobs.Scheduler
	http      *http.Client

	codexMu sync.Mutex
	stateMu sync.RWMutex
	thread  codex.ThreadSession
	managed managedChannels
}

type managedChannels struct {
	Category      discordsvc.Channel
	Ops           discordsvc.Channel
	Notifications discordsvc.Channel
	DailyLog      discordsvc.Channel
	GrowthLog     discordsvc.Channel
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
		cfg:    cfg,
		paths:  paths,
		logger: logger,
		loc:    loc,
		store:  store,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	tools := codex.NewToolRegistry()
	app.registerTools(tools)
	app.codex = codex.NewClient(cfg, paths, logger, tools)
	app.scheduler = jobs.NewScheduler(store, mustDuration(cfg.Behavior.JobPollInterval, 30*time.Second))
	app.scheduler.Register("codex_release_watch", jobHandlerFunc(app.handleReleaseWatchJob))
	app.scheduler.Register("daily_summary", jobHandlerFunc(app.handleDailySummaryJob))
	app.scheduler.Register("weekly_review", jobHandlerFunc(app.handleWeeklyReviewJob))
	app.scheduler.Register("growth_log", jobHandlerFunc(app.handleGrowthLogJob))
	app.scheduler.Register("wake_summary", jobHandlerFunc(app.handleWakeSummaryJob))

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
	if _, err := runtimecfg.EnsureLayout(a.cfg); err != nil {
		return err
	}
	if _, err := a.codex.Bootstrap(ctx); err != nil {
		a.logger.Warn("codex bootstrap skipped", "error", err)
	}
	return nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.cfg.ValidateServe(); err != nil {
		return err
	}
	if err := a.Bootstrap(ctx); err != nil {
		return err
	}

	threadID, _, _ := a.store.GetKV(ctx, "codex.main_thread_id")
	if session, err := a.codex.EnsureThread(ctx, threadID, baseInstructions(), developerInstructions()); err != nil {
		a.logger.Warn("codex thread unavailable; fallback mode only", "error", err)
	} else {
		a.thread = session
		_ = a.store.SetKV(ctx, "codex.main_thread_id", session.ID)
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

	if err := a.ensureManagedSpace(ctx); err != nil {
		return err
	}
	if err := a.ensureDefaultJobs(ctx); err != nil {
		return err
	}

	go func() {
		err := a.scheduler.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("scheduler failed", "error", err)
		}
	}()

	<-ctx.Done()
	return nil
}

func (a *App) processMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	if event.Author == nil || event.Author.Bot {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
			"guild_id": event.GuildID,
		},
	}
	if err := a.store.SaveMessage(ctx, msg); err != nil {
		a.logger.Error("save message failed", "error", err)
		return
	}

	profile, err := a.resolveChannelProfile(ctx, event.ChannelID, channelName)
	if err != nil {
		a.logger.Error("resolve channel profile failed", "error", err)
		return
	}

	recent, _ := a.store.RecentMessages(ctx, event.ChannelID, 12)
	facts, _ := a.store.SearchFacts(ctx, channelName, 8)

	decisionValue, err := a.decide(ctx, msg, profile, recent, facts)
	if err != nil {
		a.logger.Error("triage failed", "error", err)
		return
	}
	if err := a.applyDecision(ctx, msg, profile, decisionValue); err != nil {
		a.logger.Error("apply decision failed", "error", err)
	}
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

	if ok && isOffline(previous.Status) && !isOffline(current.Status) {
		threshold := mustDuration(a.cfg.Behavior.WakeSummaryThreshold, 4*time.Hour)
		if now.Sub(previous.StartedAt) >= threshold {
			payload := map[string]any{
				"since": previous.StartedAt.Format(time.RFC3339Nano),
			}
			job := jobs.NewJob(jobID("wake-summary"), "wake_summary", "wake summary", a.managed.Ops.ID, "10s", payload)
			job.NextRunAt = now.Add(10 * time.Second)
			if err := a.store.UpsertJob(ctx, job); err != nil {
				a.logger.Warn("schedule wake summary failed", "error", err)
			}
		}
	}
}

func (a *App) decide(ctx context.Context, msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact) (decision.ReplyDecision, error) {
	if fallback, ok := fallbackDecision(msg, profile, a.discordSelfMention()); ok {
		return fallback, nil
	}
	if a.thread.ID == "" {
		return fallbackDecisionOnly(msg, profile, a.discordSelfMention()), nil
	}

	prompt := buildTriagePrompt(msg, profile, recent, facts, a.managed, a.discordSelfMention())
	a.codexMu.Lock()
	defer a.codexMu.Unlock()
	raw, err := a.codex.RunJSONTurn(ctx, a.thread.ID, prompt, decision.OutputSchema())
	if err != nil {
		return fallbackDecisionOnly(msg, profile, a.discordSelfMention()), nil
	}
	var parsed decision.ReplyDecision
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fallbackDecisionOnly(msg, profile, a.discordSelfMention()), nil
	}
	if parsed.Action == "" {
		return fallbackDecisionOnly(msg, profile, a.discordSelfMention()), nil
	}
	return parsed, nil
}

func (a *App) applyDecision(ctx context.Context, msg memory.Message, profile memory.ChannelProfile, selected decision.ReplyDecision) error {
	for _, write := range selected.MemoryWrites {
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            write.Kind,
			Key:             write.Key,
			Value:           write.Value,
			SourceMessageID: msg.ID,
		}); err != nil {
			return err
		}
	}

	for _, action := range selected.Actions {
		if err := a.executeAction(ctx, msg, action); err != nil {
			return err
		}
	}

	for _, req := range selected.Jobs {
		if err := a.enqueueJob(ctx, msg, req); err != nil {
			return err
		}
	}

	if selected.Action != decision.ActionIgnore && strings.TrimSpace(selected.Message) != "" {
		if _, err := a.discord.SendMessage(ctx, msg.ChannelID, selected.Message); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) executeAction(ctx context.Context, msg memory.Message, action decision.ServerAction) error {
	switch action.Type {
	case "create_channel":
		parentID := action.ParentChannelID
		if parentID == "" {
			parentID = a.managed.Category.ID
		}
		channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
			Name:     sanitizeChannelName(action.Name),
			Topic:    action.Topic,
			ParentID: parentID,
		})
		if err != nil {
			return err
		}
		if strings.TrimSpace(action.AnnouncementText) != "" {
			if _, err := a.discord.SendMessage(ctx, msg.ChannelID, action.AnnouncementText+"\n<#"+channel.ID+"> を用意しました。"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) enqueueJob(ctx context.Context, msg memory.Message, req decision.JobRequest) error {
	channelID := req.ChannelID
	if channelID == "" {
		switch req.Kind {
		case "codex_release_watch":
			channelID = a.managed.Notifications.ID
		case "daily_summary":
			channelID = a.managed.DailyLog.ID
		case "growth_log":
			channelID = a.managed.GrowthLog.ID
		default:
			channelID = msg.ChannelID
		}
	}
	job := jobs.NewJob(jobID(req.Kind), req.Kind, req.Title, channelID, req.Schedule, req.Payload)
	if req.Schedule == "" {
		job.ScheduleExpr = a.cfg.Behavior.ReleaseWatchInterval
	}
	if req.Kind == "codex_release_watch" && job.Payload["repo"] == nil {
		job.Payload["repo"] = "openai/codex"
	}
	return a.store.UpsertJob(ctx, job)
}

func (a *App) ensureManagedSpace(ctx context.Context) error {
	category, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, a.cfg.Runtime.CategoryName)
	if err != nil {
		return err
	}
	ops, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
		Name:     a.cfg.Runtime.OpsChannelName,
		ParentID: category.ID,
		Topic:    "ゆるり運用ログ",
	})
	if err != nil {
		return err
	}
	notifications, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
		Name:     a.cfg.Runtime.NotificationsChannelName,
		ParentID: category.ID,
		Topic:    "ゆるり通知",
	})
	if err != nil {
		return err
	}
	daily, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
		Name:     a.cfg.Runtime.DailyLogChannelName,
		ParentID: category.ID,
		Topic:    "日次まとめ",
	})
	if err != nil {
		return err
	}
	growth, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
		Name:     a.cfg.Runtime.GrowthLogChannelName,
		ParentID: category.ID,
		Topic:    "成長日記",
	})
	if err != nil {
		return err
	}
	a.stateMu.Lock()
	a.managed = managedChannels{
		Category:      category,
		Ops:           ops,
		Notifications: notifications,
		DailyLog:      daily,
		GrowthLog:     growth,
	}
	a.stateMu.Unlock()
	return nil
}

func (a *App) ensureDefaultJobs(ctx context.Context) error {
	defaults := []jobs.Job{
		{
			ID:           "daily-summary",
			Kind:         "daily_summary",
			Title:        "daily summary",
			State:        jobs.StatePending,
			ChannelID:    a.managed.DailyLog.ID,
			ScheduleExpr: "24h",
			NextRunAt:    nextLocalClock(time.Now().In(a.loc), a.loc, 23, 30),
		},
		{
			ID:           "weekly-review",
			Kind:         "weekly_review",
			Title:        "weekly review",
			State:        jobs.StatePending,
			ChannelID:    a.managed.DailyLog.ID,
			ScheduleExpr: "168h",
			NextRunAt:    nextWeekdayClock(time.Now().In(a.loc), a.loc, time.Sunday, 21, 0),
		},
		{
			ID:           "growth-log",
			Kind:         "growth_log",
			Title:        "growth log",
			State:        jobs.StatePending,
			ChannelID:    a.managed.GrowthLog.ID,
			ScheduleExpr: "24h",
			NextRunAt:    nextLocalClock(time.Now().In(a.loc), a.loc, 23, 45),
		},
	}

	for _, job := range defaults {
		if _, exists, err := a.store.GetJob(ctx, job.ID); err != nil {
			return err
		} else if exists {
			continue
		}
		job.Payload = map[string]any{}
		job.CreatedAt = time.Now().UTC()
		job.UpdatedAt = job.CreatedAt
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) resolveChannelProfile(ctx context.Context, channelID string, channelName string) (memory.ChannelProfile, error) {
	profile, ok, err := a.store.GetChannelProfile(ctx, channelID)
	if err != nil {
		return memory.ChannelProfile{}, err
	}
	if ok {
		return profile, nil
	}

	kind := "conversation"
	replyAgg := 0.75
	autonomy := 0.55
	cadence := "daily"
	lower := strings.ToLower(channelName)

	if slices.Contains(a.cfg.Behavior.MonologueChannelNames, channelName) || strings.Contains(lower, "monologue") {
		kind = "monologue"
		replyAgg = 0.15
		autonomy = 0.85
	}
	if channelName == a.cfg.Runtime.NotificationsChannelName || slices.Contains(a.cfg.Behavior.NotificationNames, channelName) {
		kind = "notifications"
		replyAgg = 0.05
		autonomy = 0.9
		cadence = "none"
	}
	if channelName == a.cfg.Runtime.OpsChannelName {
		kind = "ops"
		replyAgg = 0.4
		autonomy = 0.95
	}
	if channelName == a.cfg.Runtime.DailyLogChannelName {
		kind = "daily-log"
		replyAgg = 0.05
		autonomy = 1
	}
	if channelName == a.cfg.Runtime.GrowthLogChannelName {
		kind = "growth-log"
		replyAgg = 0.05
		autonomy = 1
	}

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
	return profile, nil
}

func (a *App) registerTools(registry *codex.ToolRegistry) {
	registry.Register("memory.search", func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit == 0 {
			input.Limit = 5
		}
		messages, err := a.store.SearchMessages(ctx, input.Query, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		facts, err := a.store.SearchFacts(ctx, input.Query, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{"messages:"}
		for _, msg := range messages {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", msg.ChannelName, msg.AuthorName, msg.Content))
		}
		lines = append(lines, "facts:")
		for _, fact := range facts {
			lines = append(lines, fmt.Sprintf("- %s/%s: %s", fact.Kind, fact.Key, fact.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
	registry.Register("jobs.list", func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		due, err := a.store.DueJobs(ctx, time.Now().UTC().Add(365*24*time.Hour), 32)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(due))
		for _, job := range due {
			lines = append(lines, fmt.Sprintf("- %s %s next=%s", job.Kind, job.ID, job.NextRunAt.Format(time.RFC3339)))
		}
		if len(lines) == 0 {
			lines = append(lines, "no jobs")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
	registry.Register("discord.read_recent_messages", func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit == 0 {
			input.Limit = 10
		}
		messages, err := a.discord.RecentMessages(ctx, input.ChannelID, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(messages))
		for _, msg := range messages {
			lines = append(lines, fmt.Sprintf("- %s: %s", msg.AuthorName, msg.Content))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
	registry.Register("discord.get_member_presence", func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal(raw, &input)
		presence, err := a.discord.CurrentPresence(ctx, a.cfg.Discord.GuildID, input.UserID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("status=%s activities=%s", presence.Status, strings.Join(presence.Activities, ", "))), nil
	})
	registry.Register("discord.create_channel", func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Name  string `json:"name"`
			Topic string `json:"topic"`
		}
		_ = json.Unmarshal(raw, &input)
		channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
			Name:     sanitizeChannelName(input.Name),
			Topic:    input.Topic,
			ParentID: a.managed.Category.ID,
		})
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("created %s (%s)", channel.Name, channel.ID)), nil
	})
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

type jobHandlerFunc func(context.Context, jobs.Job) (jobs.Result, error)

func (f jobHandlerFunc) Run(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return f(ctx, job)
}

func textTool(text string) codex.ToolResponse {
	return codex.ToolResponse{
		Success: true,
		ContentItems: []codex.ToolContentItem{
			{Type: "inputText", Text: text},
		},
	}
}

func mustDuration(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func nextLocalClock(now time.Time, loc *time.Location, hour int, minute int) time.Time {
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.UTC()
}

func nextWeekdayClock(now time.Time, loc *time.Location, weekday time.Weekday, hour int, minute int) time.Time {
	offset := (7 + int(weekday) - int(now.Weekday())) % 7
	if offset == 0 {
		offset = 7
	}
	target := time.Date(now.Year(), now.Month(), now.Day()+offset, hour, minute, 0, 0, loc)
	return target.UTC()
}

func jobID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func isOffline(status string) bool {
	switch status {
	case string(discordgo.StatusOffline), string(discordgo.StatusInvisible), "":
		return true
	default:
		return false
	}
}

var channelNameRe = regexp.MustCompile(`[^a-z0-9-]+`)
var repeatedDashRe = regexp.MustCompile(`-+`)

func sanitizeChannelName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = channelNameRe.ReplaceAllString(value, "-")
	value = repeatedDashRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "new-topic"
	}
	if len(value) > 90 {
		return value[:90]
	}
	return value
}
