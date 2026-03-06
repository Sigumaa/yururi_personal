package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Sigumaa/yururi_personal/internal/bot"
	"github.com/Sigumaa/yururi_personal/internal/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("yururi failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	configPath := fs.String("config", filepath.Join("config", "example.toml"), "path to bot config")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	args := fs.Args()
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app, err := bot.New(cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	switch command {
	case "bootstrap":
		return app.Bootstrap(context.Background())
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return app.Run(ctx)
	default:
		return errors.New(fmt.Sprintf("unknown command: %s", command))
	}
}
