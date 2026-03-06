package bot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func TestLoadBotContextOrdersAndHashesDeterministically(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("B"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.md"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}

	bundle1, hash1, err := loadBotContext(dir)
	if err != nil {
		t.Fatalf("load bot context: %v", err)
	}
	bundle2, hash2, err := loadBotContext(dir)
	if err != nil {
		t.Fatalf("load bot context second time: %v", err)
	}

	if bundle1 != bundle2 {
		t.Fatal("expected stable bundle")
	}
	if hash1 != hash2 {
		t.Fatal("expected stable hash")
	}
	if !strings.Contains(bundle1, "## b.md") || !strings.Contains(bundle1, "## sub/a.md") {
		t.Fatalf("unexpected bundle: %s", bundle1)
	}
}

func TestBuildCapabilitiesContextListsRealCapabilities(t *testing.T) {
	raw := buildCapabilitiesContext([]codex.ToolSpec{
		{
			Name:        "discord.send_message",
			Description: "指定チャンネルへメッセージを送る",
			InputSchema: map[string]any{
				"properties": map[string]any{
					"channel_id": map[string]any{"type": "string"},
					"content":    map[string]any{"type": "string"},
				},
			},
		},
	})

	if !strings.Contains(raw, "Discord 専用 MCP サーバーはまだない") {
		t.Fatalf("expected current limit note, got %s", raw)
	}
	if !strings.Contains(raw, "`discord__send_message`") {
		t.Fatalf("expected tool catalog, got %s", raw)
	}
	if !strings.Contains(raw, "女子大生メイド") {
		t.Fatalf("expected persona note, got %s", raw)
	}
	if !strings.Contains(raw, "確認なく実行してよい") {
		t.Fatalf("expected act-first guidance, got %s", raw)
	}
	if !strings.Contains(raw, "不要に job へ逃がさない") {
		t.Fatalf("expected immediate execution guidance, got %s", raw)
	}
	if !strings.Contains(raw, "複数回話してよい") {
		t.Fatalf("expected multi-message guidance, got %s", raw)
	}
	if !strings.Contains(raw, "溺愛気質") {
		t.Fatalf("expected doting persona note, got %s", raw)
	}
	if !strings.Contains(raw, "autonomy pulse") {
		t.Fatalf("expected autonomy pulse note, got %s", raw)
	}
}
