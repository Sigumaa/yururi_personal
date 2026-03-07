package bot

import (
	"context"
	"errors"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/runtime"
	"github.com/bwmarrin/discordgo"
)

func (a *App) Bootstrap(ctx context.Context) error {
	a.logger.Info("bootstrap start", "runtime_root", a.paths.Root)
	if _, err := runtime.EnsureLayout(a.cfg); err != nil {
		return err
	}
	if err := a.syncBotContext(); err != nil {
		return err
	}
	if err := a.codex.Bootstrap(ctx); err != nil {
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
