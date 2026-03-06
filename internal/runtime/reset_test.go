package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

func TestResetPreservesAuthByDefault(t *testing.T) {
	root := t.TempDir()
	cfg := testRuntimeConfig(root)
	paths, err := EnsureLayout(cfg)
	if err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	mustWriteFile(t, filepath.Join(paths.CodexHome, "auth.json"), `{"token":"ok"}`)
	mustWriteFile(t, filepath.Join(paths.CodexHome, "state_5.sqlite"), "state")
	mustWriteFile(t, filepath.Join(paths.DataDir, "yururi.db"), "db")
	mustWriteFile(t, filepath.Join(paths.LogDir, "bot.log"), "log")
	mustWriteFile(t, filepath.Join(paths.Workspace, "scratch.txt"), "scratch")
	mustWriteFile(t, filepath.Join(paths.CodexHome, "shell_snapshots", "x.sh"), "echo test")

	report, err := Reset(cfg, false)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if report.Full {
		t.Fatal("expected non-full reset")
	}

	assertExists(t, filepath.Join(paths.CodexHome, "auth.json"))
	assertNotExists(t, filepath.Join(paths.CodexHome, "state_5.sqlite"))
	assertNotExists(t, filepath.Join(paths.DataDir, "yururi.db"))
	assertNotExists(t, filepath.Join(paths.LogDir, "bot.log"))
	assertNotExists(t, filepath.Join(paths.Workspace, "scratch.txt"))
	assertExists(t, paths.WorkspaceAGENTSPath)
	assertExists(t, paths.WorkspaceBehaviorPath)
	assertExists(t, paths.CodexConfigPath)
	assertExists(t, paths.CodexModelPromptPath)
}

func TestResetFullRemovesAuth(t *testing.T) {
	root := t.TempDir()
	cfg := testRuntimeConfig(root)
	paths, err := EnsureLayout(cfg)
	if err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	mustWriteFile(t, filepath.Join(paths.CodexHome, "auth.json"), `{"token":"ok"}`)
	mustWriteFile(t, filepath.Join(paths.DataDir, "yururi.db"), "db")

	report, err := Reset(cfg, true)
	if err != nil {
		t.Fatalf("reset full: %v", err)
	}
	if !report.Full {
		t.Fatal("expected full reset")
	}

	assertNotExists(t, filepath.Join(paths.CodexHome, "auth.json"))
	assertExists(t, paths.CodexConfigPath)
	assertExists(t, paths.CodexModelPromptPath)
	assertExists(t, paths.WorkspaceAGENTSPath)
}

func testRuntimeConfig(root string) config.Config {
	return config.Config{
		AppName:  "yururi",
		Timezone: "Asia/Tokyo",
		Runtime: config.RuntimeConfig{
			Root: root,
		},
		Codex: config.CodexConfig{
			ApprovalPolicy: "never",
			SandboxMode:    "danger-full-access",
		},
	}
}

func mustWriteFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", path, err)
	}
}
