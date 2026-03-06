package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

func TestEnsureLayoutCreatesManagedFiles(t *testing.T) {
	cfg := config.Config{
		Runtime: config.RuntimeConfig{
			Root: t.TempDir(),
		},
		Codex: config.CodexConfig{
			ApprovalPolicy: "never",
			SandboxMode:    "danger-full-access",
		},
	}

	paths, err := EnsureLayout(cfg)
	if err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	for _, path := range []string{paths.CodexConfigPath, paths.CodexModelPromptPath, paths.WorkspaceAGENTSPath, paths.WorkspaceBehaviorPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %s: %v", path, err)
		}
	}

	raw, err := os.ReadFile(paths.CodexConfigPath)
	if err != nil {
		t.Fatalf("read codex config: %v", err)
	}
	if !strings.Contains(string(raw), "model_instructions_file") {
		t.Fatalf("expected model_instructions_file in codex config, got %s", string(raw))
	}
	raw, err = os.ReadFile(paths.CodexModelPromptPath)
	if err != nil {
		t.Fatalf("read model prompt: %v", err)
	}
	if !strings.Contains(string(raw), "女子大生メイド") {
		t.Fatalf("expected persona prompt to mention 女子大生メイド, got %s", string(raw))
	}
	if !strings.Contains(string(raw), "溺愛デレデレ寄り") {
		t.Fatalf("expected persona prompt to mention 溺愛デレデレ寄り, got %s", string(raw))
	}
	if !strings.Contains(string(raw), "確認なく進めてよい") {
		t.Fatalf("expected model prompt to prefer act-first, got %s", string(raw))
	}
}

func TestEnsureLayoutRemovesLegacyAnyDir(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "workspace", "any")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "要望.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write legacy doc: %v", err)
	}

	cfg := config.Config{
		Runtime: config.RuntimeConfig{
			Root: root,
		},
		Codex: config.CodexConfig{
			ApprovalPolicy: "never",
			SandboxMode:    "danger-full-access",
		},
	}

	paths, err := EnsureLayout(cfg)
	if err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.Workspace, "any")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy any dir to be removed, got err=%v", err)
	}
}
