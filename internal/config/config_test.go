package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndEnvOverride(t *testing.T) {
	t.Setenv("YURURI_DISCORD_TOKEN", "env-token")
	t.Setenv("YURURI_GUILD_ID", "guild")
	t.Setenv("YURURI_OWNER_USER_ID", "owner")
	t.Setenv("YURURI_RUNTIME_ROOT", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "bot.toml")
	if err := os.WriteFile(configPath, []byte(`
app_name = "test-yururi"
[discord]
token = "file-token"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Discord.Token != "env-token" {
		t.Fatalf("expected env token override, got %q", cfg.Discord.Token)
	}
	if !filepath.IsAbs(cfg.Runtime.Root) {
		t.Fatalf("expected absolute runtime root, got %q", cfg.Runtime.Root)
	}
}

func TestValidateServe(t *testing.T) {
	cfg := defaultConfig()
	if err := cfg.ValidateServe(); err == nil {
		t.Fatal("expected validate serve to fail without discord credentials")
	}

	cfg.Discord.Token = "token"
	cfg.Discord.GuildID = "guild"
	cfg.Discord.OwnerUserID = "owner"
	if err := cfg.ValidateServe(); err != nil {
		t.Fatalf("expected validate serve to pass, got %v", err)
	}
}
