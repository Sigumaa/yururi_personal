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
	tools     *codex.ToolRegistry
	codex     *codex.Client
	discord   discordsvc.Service
	scheduler *jobs.Scheduler
	http      *http.Client

	threadMu    sync.Mutex
	threadLocks map[string]*sync.Mutex
	thread      codex.ThreadSession
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
		cfg:         cfg,
		paths:       paths,
		logger:      logger,
		loc:         loc,
		store:       store,
		threadLocks: map[string]*sync.Mutex{},
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	tools := codex.NewToolRegistry()
	app.registerTools(tools)
	app.tools = tools
	app.codex = codex.NewClient(cfg, paths, logger, tools)
	app.scheduler = jobs.NewScheduler(store, mustDuration(cfg.Behavior.JobPollInterval, 30*time.Second))
	app.scheduler.SetLogger(logger)
	app.scheduler.SetObserver(app.handleJobResult)
	app.scheduler.Register("codex_release_watch", jobHandlerFunc(app.handleReleaseWatchJob))
	app.scheduler.Register("codex_background_task", jobHandlerFunc(app.handleBackgroundCodexTaskJob))
	app.scheduler.Register("url_watch", jobHandlerFunc(app.handleURLWatchJob))
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

	threadID, _, _ := a.store.GetKV(ctx, "codex.main_thread_id")
	if session, err := a.codex.EnsureThread(ctx, threadID, baseInstructions(), developerInstructions()); err != nil {
		a.logger.Warn("codex thread unavailable", "error", err)
	} else {
		a.thread = session
		_ = a.store.SetKV(ctx, "codex.main_thread_id", session.ID)
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
			"guild_id":    event.GuildID,
			"attachments": attachmentURLs(event.Attachments),
		},
	}
	a.logger.Info("message received", "channel", channelName, "channel_id", event.ChannelID, "author", event.Author.Username, "message_id", event.ID, "attachments", len(event.Attachments), "content_preview", previewText(event.Content, 240))
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
	a.logger.Info("message context ready", "channel", channelName, "recent_messages", len(recent), "related_facts", len(facts), "profile_kind", profile.Kind)
	a.logger.Debug("message context detail", "channel", channelName, "profile", previewJSON(profile, 320), "recent_preview", previewJSON(recent, 900), "fact_preview", previewJSON(facts, 900))

	decisionValue, err := a.planDecision(ctx, msg, profile, recent, facts)
	if err != nil {
		a.logger.Error("plan failed", "error", err)
		return
	}
	a.logger.Info("decision ready", "channel", channelName, "action", decisionValue.Action, "reply_len", len(strings.TrimSpace(decisionValue.Message)), "reason", decisionValue.Reason, "confidence", decisionValue.Confidence, "message_preview", previewText(decisionValue.Message, 240))
	a.logger.Debug("decision detail", "channel", channelName, "decision", previewJSON(decisionValue, 1600))

	report, err := a.executeDecision(ctx, msg, decisionValue)
	if err != nil {
		a.logger.Error("execute decision failed", "error", err)
		return
	}
	a.logger.Debug("execution report", "channel", channelName, "report", previewJSON(report, 1200))

	reply, err := a.composeDecisionReply(ctx, msg, decisionValue, report)
	if err != nil {
		a.logger.Error("compose reply failed", "error", err)
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
		threshold := mustDuration(a.cfg.Behavior.WakeSummaryThreshold, 4*time.Hour)
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

func (a *App) planDecision(ctx context.Context, msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact) (decision.ReplyDecision, error) {
	if a.thread.ID == "" {
		return decision.ReplyDecision{}, errors.New("codex thread is unavailable")
	}

	prompt := buildPlannerPrompt(msg, profile, recent, facts, a.tools.Specs(), a.discordSelfMention())
	a.logger.Info("codex turn start", "thread_id", a.thread.ID, "channel", msg.ChannelName, "message_id", msg.ID, "prompt_bytes", len(prompt))
	a.logger.Debug("codex planner prompt", "thread_id", a.thread.ID, "channel", msg.ChannelName, "message_id", msg.ID, "prompt_preview", previewText(prompt, 1600))
	raw, err := a.runThreadJSONTurn(ctx, a.thread.ID, prompt, decision.OutputSchema())
	if err != nil {
		a.logger.Warn("codex turn failed", "channel", msg.ChannelName, "message_id", msg.ID, "error", err)
		return decision.ReplyDecision{}, fmt.Errorf("run codex turn: %w", err)
	}
	a.logger.Info("codex turn completed", "channel", msg.ChannelName, "message_id", msg.ID, "response_bytes", len(raw))
	a.logger.Debug("codex planner output", "channel", msg.ChannelName, "message_id", msg.ID, "raw", previewText(raw, 1600))
	return parseDecisionPlan(raw)
}

func (a *App) executeDecision(ctx context.Context, msg memory.Message, selected decision.ReplyDecision) (executionReport, error) {
	report := executionReport{}
	a.logger.Debug("execute decision start", "channel", msg.ChannelName, "message_id", msg.ID, "memory_writes", len(selected.MemoryWrites), "actions", len(selected.Actions), "jobs", len(selected.Jobs))
	for _, write := range selected.MemoryWrites {
		a.logger.Debug("memory write start", "kind", write.Kind, "key", write.Key, "value_preview", previewText(write.Value, 240))
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            write.Kind,
			Key:             write.Key,
			Value:           write.Value,
			SourceMessageID: msg.ID,
		}); err != nil {
			return executionReport{}, err
		}
		report.MemoryWrites = append(report.MemoryWrites, fmt.Sprintf("%s/%s", write.Kind, write.Key))
	}

	for _, action := range selected.Actions {
		if err := a.sendActionAnnouncement(ctx, msg.ChannelID, action); err != nil {
			return executionReport{}, err
		}
		a.logger.Debug("server action start", "action", previewJSON(action, 600))
		summary, err := a.executeAction(ctx, action)
		if err != nil {
			return executionReport{}, err
		}
		report.Actions = append(report.Actions, summary)
	}

	for _, req := range selected.Jobs {
		a.logger.Debug("job request start", "job_request", previewJSON(req, 900))
		summary, err := a.enqueueJob(ctx, msg, req)
		if err != nil {
			return executionReport{}, err
		}
		report.Jobs = append(report.Jobs, summary)
	}
	return report, nil
}

func (a *App) sendActionAnnouncement(ctx context.Context, channelID string, action decision.ServerAction) error {
	text := strings.TrimSpace(action.AnnouncementText)
	if text == "" {
		return nil
	}
	if a.discord == nil {
		return errors.New("discord is not connected")
	}
	sentID, err := a.discord.SendMessage(ctx, channelID, text)
	if err != nil {
		return err
	}
	a.logger.Info("action announcement sent", "channel_id", channelID, "message_id", sentID, "action_type", action.Type, "text_preview", previewText(text, 240))
	return nil
}

func (a *App) executeAction(ctx context.Context, action decision.ServerAction) (string, error) {
	a.logger.Debug("execute action", "type", action.Type, "action", previewJSON(action, 600))
	switch action.Type {
	case "create_category":
		channel, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, action.Name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("created category %s (%s)", channel.Name, channel.ID), nil
	case "create_channel":
		channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
			Name:     sanitizeChannelName(action.Name),
			Topic:    action.Topic,
			ParentID: action.ParentChannelID,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("created channel %s (%s)", channel.Name, channel.ID), nil
	case "rename_channel":
		channel, err := a.discord.RenameChannel(ctx, action.TargetChannelID, sanitizeChannelName(action.Name))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("renamed channel %s (%s)", channel.Name, channel.ID), nil
	case "set_channel_topic":
		channel, err := a.discord.SetChannelTopic(ctx, action.TargetChannelID, action.Topic)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("updated topic for %s (%s)", channel.Name, channel.ID), nil
	case "move_channel":
		if err := a.discord.MoveChannel(ctx, action.TargetChannelID, action.ParentChannelID); err != nil {
			return "", err
		}
		return fmt.Sprintf("moved channel %s to %s", action.TargetChannelID, action.ParentChannelID), nil
	}
	return "", fmt.Errorf("unsupported action type: %s", action.Type)
}

func (a *App) enqueueJob(ctx context.Context, msg memory.Message, req decision.JobRequest) (string, error) {
	channelID := req.ChannelID
	if channelID == "" {
		channelID = msg.ChannelID
	}
	job := jobs.NewJob(jobID(req.Kind), req.Kind, req.Title, channelID, req.Schedule, req.Payload)
	if job.Payload == nil {
		job.Payload = map[string]any{}
	}
	if req.Schedule == "" {
		job.ScheduleExpr = a.cfg.Behavior.ReleaseWatchInterval
	}
	if req.Kind == "codex_release_watch" && job.Payload["repo"] == nil {
		job.Payload["repo"] = "openai/codex"
	}
	if req.Kind == "codex_background_task" {
		if job.Payload["prompt"] == nil {
			job.Payload["prompt"] = msg.Content
		}
		if job.Payload["goal"] == nil {
			job.Payload["goal"] = req.Title
		}
	}
	job.Payload["origin_channel_id"] = msg.ChannelID
	job.Payload["origin_message_id"] = msg.ID
	job.Payload["origin_channel_name"] = msg.ChannelName
	job.Payload["origin_author_name"] = msg.AuthorName
	a.logger.Info("job enqueued", "job_id", job.ID, "kind", job.Kind, "channel_id", job.ChannelID, "schedule", job.ScheduleExpr, "title", job.Title)
	a.logger.Debug("job payload", "job_id", job.ID, "payload", previewJSON(job.Payload, 1200))
	if err := a.store.UpsertJob(ctx, job); err != nil {
		return "", err
	}
	return fmt.Sprintf("scheduled job %s (%s)", job.ID, job.Kind), nil
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
	lower := strings.ToLower(channelName)

	if strings.Contains(lower, "monologue") || slices.Contains(a.cfg.Behavior.MonologueChannelNames, channelName) {
		kind = "monologue"
		replyAgg = 0.15
		autonomy = 0.85
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
	a.logger.Info("channel profile created", "channel", channelName, "channel_id", channelID, "kind", kind, "reply_aggressiveness", replyAgg, "autonomy_level", autonomy)
	return profile, nil
}

func (a *App) registerTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.search",
		Description: "保存済みメッセージと fact から関連情報を検索する",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "検索語"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
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
	registry.Register(codex.ToolSpec{
		Name:        "memory.write_fact",
		Description: "長期記憶として fact を 1 件保存または更新する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "fact の種別"),
			fieldSchema("key", "string", "fact の一意キー"),
			fieldSchema("value", "string", "保存する内容"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind            string `json:"kind"`
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("kind and key are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            input.Kind,
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "jobs.list",
		Description: "登録済み job の一覧を確認する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 32
		}
		due, err := a.store.DueJobs(ctx, time.Now().UTC().Add(365*24*time.Hour), input.Limit)
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
	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule",
		Description: "job を登録または更新する",
		InputSchema: objectSchema(
			fieldSchema("id", "string", "job ID"),
			fieldSchema("kind", "string", "job の種別"),
			fieldSchema("title", "string", "job の表示名"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
			fieldSchema("payload", "object", "job payload"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID        string         `json:"id"`
			Kind      string         `json:"kind"`
			Title     string         `json:"title"`
			ChannelID string         `json:"channel_id"`
			Schedule  string         `json:"schedule"`
			Payload   map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Title) == "" {
			return codex.ToolResponse{}, errors.New("kind and title are required")
		}
		if input.ID == "" {
			input.ID = jobID(input.Kind)
		}
		if input.Schedule == "" {
			input.Schedule = a.cfg.Behavior.ReleaseWatchInterval
		}
		job := jobs.NewJob(input.ID, input.Kind, input.Title, input.ChannelID, input.Schedule, input.Payload)
		if input.Kind == "codex_release_watch" && job.Payload["repo"] == nil {
			job.Payload["repo"] = "openai/codex"
		}
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "jobs.cancel",
		Description: "job を停止する",
		InputSchema: objectSchema(fieldSchema("id", "string", "停止する job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if input.ID == "" {
			return codex.ToolResponse{}, errors.New("id is required")
		}
		if err := a.store.UpdateJobState(ctx, input.ID, jobs.StateCompleted, time.Now().UTC(), "cancelled", nil); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("cancelled"), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "discord.list_channels",
		Description: "サーバー内のチャンネル一覧を取得する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(channels))
		for _, channel := range channels {
			lines = append(lines, fmt.Sprintf("- %s id=%s parent=%s type=%d", channel.Name, channel.ID, channel.ParentID, channel.Type))
		}
		if len(lines) == 0 {
			lines = append(lines, "no channels")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "discord.read_recent_messages",
		Description: "指定チャンネルの直近メッセージを読む",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
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
	registry.Register(codex.ToolSpec{
		Name:        "discord.get_member_presence",
		Description: "ユーザーの現在の presence と activity を取得する",
		InputSchema: objectSchema(fieldSchema("user_id", "string", "対象ユーザー ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
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
	registry.Register(codex.ToolSpec{
		Name:        "discord.send_message",
		Description: "指定チャンネルへメッセージを送る。進捗共有や途中経過の連投にも使える",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "送信先チャンネル ID"),
			fieldSchema("content", "string", "送信内容"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		messageID, err := a.discord.SendMessage(ctx, input.ChannelID, input.Content)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("sent %s", messageID)), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "discord.create_category",
		Description: "カテゴリチャンネルを作成する",
		InputSchema: objectSchema(fieldSchema("name", "string", "カテゴリ名")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, input.Name)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("created %s (%s)", channel.Name, channel.ID)), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "discord.create_channel",
		Description: "テキストチャンネルを作成する。parent_channel_id を省略するとルート直下に作る",
		InputSchema: objectSchema(
			fieldSchema("name", "string", "チャンネル名"),
			fieldSchema("topic", "string", "トピック"),
			fieldSchema("parent_channel_id", "string", "親カテゴリ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Name            string `json:"name"`
			Topic           string `json:"topic"`
			ParentChannelID string `json:"parent_channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
			Name:     sanitizeChannelName(input.Name),
			Topic:    input.Topic,
			ParentID: input.ParentChannelID,
		})
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("created %s (%s)", channel.Name, channel.ID)), nil
	})
	registry.Register(codex.ToolSpec{
		Name:        "discord.move_channel",
		Description: "チャンネルを別カテゴリへ移動する",
		InputSchema: objectSchema(
			fieldSchema("target_channel_id", "string", "移動対象チャンネル ID"),
			fieldSchema("parent_channel_id", "string", "移動先カテゴリ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			TargetChannelID string `json:"target_channel_id"`
			ParentChannelID string `json:"parent_channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if err := a.discord.MoveChannel(ctx, input.TargetChannelID, input.ParentChannelID); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})
	a.registerExtraTools(registry)
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

func objectSchema(fields ...map[string]any) map[string]any {
	properties := map[string]any{}
	for _, field := range fields {
		name, _ := field["name"].(string)
		if name == "" {
			continue
		}
		cloned := map[string]any{}
		for key, value := range field {
			if key == "name" {
				continue
			}
			cloned[key] = value
		}
		properties[name] = cloned
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
}

func fieldSchema(name string, fieldType string, description string) map[string]any {
	field := map[string]any{
		"name": name,
		"type": fieldType,
	}
	if description != "" {
		field["description"] = description
	}
	return field
}

func textTool(text string) codex.ToolResponse {
	return codex.ToolResponse{
		Success: true,
		ContentItems: []codex.ToolContentItem{
			{Type: "inputText", Text: text},
		},
	}
}

func attachmentURLs(attachments []*discordgo.MessageAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil || strings.TrimSpace(attachment.URL) == "" {
			continue
		}
		out = append(out, attachment.URL)
	}
	return out
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
