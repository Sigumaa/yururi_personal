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

	for _, path := range []string{paths.CodexConfigPath, paths.CodexModelPromptPath, paths.WorkspaceAGENTSPath} {
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
}

func TestEnsureLayoutSyncsReferenceDocs(t *testing.T) {
	projectRoot := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if err := os.MkdirAll(filepath.Join(projectRoot, "any"), 0o755); err != nil {
		t.Fatalf("mkdir any: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "any", "要望.md"), []byte("やりたいこと"), 0o644); err != nil {
		t.Fatalf("write any doc: %v", err)
	}

	cfg := config.Config{
		Runtime: config.RuntimeConfig{
			Root: filepath.Join(projectRoot, "runtime"),
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

	raw, err := os.ReadFile(filepath.Join(paths.WorkspaceAnyDir, "要望.md"))
	if err != nil {
		t.Fatalf("read synced reference doc: %v", err)
	}
	if string(raw) != "やりたいこと" {
		t.Fatalf("unexpected synced content: %s", string(raw))
	}
}
