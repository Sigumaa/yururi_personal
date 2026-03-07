package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	AppName  string        `toml:"app_name"`
	Timezone string        `toml:"timezone"`
	Discord  DiscordConfig `toml:"discord"`
	Runtime  RuntimeConfig `toml:"runtime"`
	Codex    CodexConfig   `toml:"codex"`
}

type DiscordConfig struct {
	Token       string `toml:"token"`
	GuildID     string `toml:"guild_id"`
	OwnerUserID string `toml:"owner_user_id"`
}

type RuntimeConfig struct {
	Root string `toml:"root"`
}

type CodexConfig struct {
	Command string `toml:"command"`
}

type Paths struct {
	Root                      string
	CodexHome                 string
	CodexConfigPath           string
	CodexModelPromptPath      string
	Workspace                 string
	WorkspaceAGENTSPath       string
	WorkspaceContextDir       string
	WorkspaceBehaviorPath     string
	WorkspaceCapabilitiesPath string
	DataDir                   string
	LogDir                    string
	DBPath                    string
}

func Load(path string) (Config, error) {
	cfg := defaultConfig()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err == nil {
			if err := toml.Unmarshal(raw, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse config: %w", err)
			}
		}
	}

	overrideFromEnv(&cfg)
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone: %w", err)
	}
	return loc, nil
}

func (c Config) ResolvePaths() Paths {
	root := c.Runtime.Root
	return Paths{
		Root:                      root,
		CodexHome:                 filepath.Join(root, "codex-home"),
		CodexConfigPath:           filepath.Join(root, "codex-home", "config.toml"),
		CodexModelPromptPath:      filepath.Join(root, "codex-home", "model_instructions.md"),
		Workspace:                 filepath.Join(root, "workspace"),
		WorkspaceAGENTSPath:       filepath.Join(root, "workspace", "AGENTS.md"),
		WorkspaceContextDir:       filepath.Join(root, "workspace", "context"),
		WorkspaceBehaviorPath:     filepath.Join(root, "workspace", "context", "behavior.md"),
		WorkspaceCapabilitiesPath: filepath.Join(root, "workspace", "context", "capabilities.md"),
		DataDir:                   filepath.Join(root, "data"),
		LogDir:                    filepath.Join(root, "logs"),
		DBPath:                    filepath.Join(root, "data", "yururi.db"),
	}
}

func defaultConfig() Config {
	return Config{
		AppName:  "yururi",
		Timezone: "Asia/Tokyo",
		Runtime: RuntimeConfig{
			Root: "./runtime",
		},
		Codex: CodexConfig{
			Command: DefaultCodexCommand,
		},
	}
}

const (
	DefaultCodexCommand          = "codex"
	DefaultCodexListenHost       = "127.0.0.1"
	DefaultCodexApprovalPolicy   = "never"
	DefaultCodexSandboxMode      = "danger-full-access"
	DefaultCodexReasoningEffort  = "medium"
	DefaultCodexReasoningSummary = "concise"
)

func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("YURURI_APP_NAME"); v != "" {
		cfg.AppName = v
	}
	if v := os.Getenv("YURURI_TIMEZONE"); v != "" {
		cfg.Timezone = v
	}
	if v := os.Getenv("YURURI_DISCORD_TOKEN"); v != "" {
		cfg.Discord.Token = v
	}
	if v := os.Getenv("YURURI_GUILD_ID"); v != "" {
		cfg.Discord.GuildID = v
	}
	if v := os.Getenv("YURURI_OWNER_USER_ID"); v != "" {
		cfg.Discord.OwnerUserID = v
	}
	if v := os.Getenv("YURURI_RUNTIME_ROOT"); v != "" {
		cfg.Runtime.Root = v
	}
}

func (c *Config) normalize() error {
	absRoot, err := filepath.Abs(c.Runtime.Root)
	if err != nil {
		return fmt.Errorf("resolve runtime root: %w", err)
	}
	c.Runtime.Root = absRoot
	return nil
}

func (c Config) ValidateServe() error {
	if c.Discord.Token == "" {
		return errors.New("discord token is required")
	}
	if c.Discord.GuildID == "" {
		return errors.New("discord guild_id is required")
	}
	if c.Discord.OwnerUserID == "" {
		return errors.New("discord owner_user_id is required")
	}
	return nil
}
