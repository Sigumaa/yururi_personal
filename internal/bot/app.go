package bot

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/config"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	runtimecfg "github.com/Sigumaa/yururi_personal/internal/runtime"
	"github.com/Sigumaa/yururi_personal/internal/voice"
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
	voice     *voice.Engine
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
		http:           &http.Client{Timeout: 10 * time.Second},
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
	if a.codex != nil {
		errs = append(errs, a.codex.Close())
	}
	if a.voice != nil {
		errs = append(errs, a.voice.Shutdown(context.Background()))
	}
	if a.discord != nil {
		errs = append(errs, a.discord.Close())
	}
	if a.store != nil {
		errs = append(errs, a.store.Close())
	}
	return errors.Join(errs...)
}

func (a *App) buildVoiceEngine() *voice.Engine {
	return voice.NewEngine(
		a.store,
		a.discord,
		voice.NewRealtimeClient(voice.RealtimeOptions{
			APIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
			Model:  voice.DefaultRealtimeModel,
		}),
		a.cfg.Discord.OwnerUserID,
		a.logger,
	)
}
