package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Sigumaa/yururi_personal/internal/bot"
	"github.com/Sigumaa/yururi_personal/internal/config"
	"github.com/Sigumaa/yururi_personal/internal/logview"
	runtimecfg "github.com/Sigumaa/yururi_personal/internal/runtime"
)

func main() {
	logger := slog.New(logview.NewHandler(os.Stdout, logview.Options{
		Level: slog.LevelDebug,
		Color: logview.DefaultColorEnabled(os.Stdout),
	}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		slog.Error("yururi failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	configPath := fs.String("config", filepath.Join("config", "example.toml"), "path to bot config")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	args := fs.Args()
	command := "serve"
	subArgs := []string{}
	if len(args) > 0 {
		command = args[0]
		subArgs = args[1:]
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	switch command {
	case "reset":
		resetFS := flag.NewFlagSet("reset", flag.ContinueOnError)
		full := resetFS.Bool("full", false, "remove runtime auth and cached Codex state too")
		if err := resetFS.Parse(subArgs); err != nil {
			return err
		}
		report, err := runtimecfg.Reset(cfg, *full)
		if err != nil {
			return err
		}
		logger.Info("runtime reset completed", "root", report.Root, "full", report.Full, "removed", report.Removed)
		return nil
	case "bootstrap":
		app, err := bot.New(cfg, logger)
		if err != nil {
			return err
		}
		defer app.Close()
		return app.Bootstrap(context.Background())
	case "serve":
		app, err := bot.New(cfg, logger)
		if err != nil {
			return err
		}
		defer app.Close()
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return app.Run(ctx)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}
