package bot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
