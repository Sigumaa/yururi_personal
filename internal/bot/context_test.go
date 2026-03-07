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
	if !strings.Contains(raw, "前置きだけ送って止まらず") {
		t.Fatalf("expected no-progress-only guidance, got %s", raw)
	}
	if !strings.Contains(raw, "溺愛デレデレ寄り") {
		t.Fatalf("expected doting persona note, got %s", raw)
	}
	if !strings.Contains(raw, "デレをにじませてもよい") {
		t.Fatalf("expected affectionate tone guidance, got %s", raw)
	}
	if !strings.Contains(raw, "autonomy pulse") {
		t.Fatalf("expected autonomy pulse note, got %s", raw)
	}
	if !strings.Contains(raw, "file change / command execution") {
		t.Fatalf("expected codex file/command capability note, got %s", raw)
	}
	if !strings.Contains(raw, "script や小さな CLI") {
		t.Fatalf("expected script foundation note, got %s", raw)
	}
	for _, want := range []string{"curiosity", "agent goal", "soft reminder", "topic thread", "initiative", "behavior baseline", "behavior deviation", "learned policy", "workspace note", "proposal boundary", "curiosity review", "topic synthesis review", "policy synthesis review", "workspace review", "proposal boundary review"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in capabilities context, got %s", want, raw)
		}
	}
}

func TestBuildToolGuideContextMentionsAffordances(t *testing.T) {
	raw := buildToolGuideContext([]codex.ToolSpec{
		{Name: "discord.list_channels", Description: "サーバー内のチャンネル一覧を取得する", InputSchema: map[string]any{}},
		{Name: "discord.create_channel", Description: "テキストチャンネルを作成する", InputSchema: map[string]any{}},
		{Name: "memory.write_fact", Description: "長期記憶を保存する", InputSchema: map[string]any{}},
		{Name: "jobs.schedule", Description: "job を登録または更新する", InputSchema: map[string]any{}},
		{Name: "web.fetch_url", Description: "URL を取得して読む", InputSchema: map[string]any{}},
		{Name: "tools.search", Description: "使えそうな tool を探す", InputSchema: map[string]any{}},
	})

	for _, want := range []string{
		"どういう場面で使うと自然か",
		"依頼理解 -> Discord 観測 -> 必要なら記憶参照 -> 実行 -> 結果共有",
		"反復依頼 -> runtime/workspace に補助 script",
		"Command / File Change",
		"Discord 観測",
		"Discord 編集",
		"記憶",
		"継続 task",
		"Web / Media",
		"Tool 補助",
		"`discord__list_channels`",
		"`discord__create_channel`",
		"`memory__write_fact`",
		"`jobs__schedule`",
		"`web__fetch_url`",
		"`tools__search`",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in tool guide, got %s", want, raw)
		}
	}
}

func TestBuildAutonomyGuideContextMentionsDecisionBoundaries(t *testing.T) {
	raw := buildAutonomyGuideContext()
	for _, want := range []string{
		"観測して、判断して、必要なときだけ動く",
		"勝手にやってよい寄り",
		"提案止まりにしやすいもの",
		"学習へのつなぎ方",
		"initiative",
		"learned policy",
		"proposal boundary",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in autonomy guide, got %s", want, raw)
		}
	}
}

func TestBuildWorkspaceGuideContextMentionsScriptsAndJobs(t *testing.T) {
	raw := buildWorkspaceGuideContext()
	for _, want := range []string{
		"runtime/workspace は、ゆるり自身の作業場所",
		"補助 script",
		"小さな CLI",
		"まず小さく書いて試す",
		"時間をまたぐ監視や留守番は jobs と組み合わせる",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in workspace guide, got %s", want, raw)
		}
	}
}

func TestBuildPhilosophyGuideContextMentionsCycle(t *testing.T) {
	raw := buildPhilosophyGuideContext()
	for _, want := range []string{
		"観測・判断・行動の循環",
		"観測する",
		"判断する",
		"行動する",
		"振り返る",
		"頼んでいないのに助かる",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in philosophy guide, got %s", want, raw)
		}
	}
}

func TestBuildSelfModelGuideContextMentionsContinuity(t *testing.T) {
	raw := buildSelfModelGuideContext()
	for _, want := range []string{
		"Discord 上で動くパーソナル AI Agent",
		"runtime/workspace を自分用の作業場所",
		"thread、記憶、summary、workspace の下書き、review",
		"curiosity、initiative、agent goal",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in self model guide, got %s", want, raw)
		}
	}
}

func TestBuildEpistemicGuideContextMentionsKnowingLimits(t *testing.T) {
	raw := buildEpistemicGuideContext()
	for _, want := range []string{
		"知っていることと推測していることを混同しない",
		"知らないまま断言しない",
		"context gap",
		"learned policy",
		"misfire",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in epistemic guide, got %s", want, raw)
		}
	}
}

func TestBuildRelationGuideContextMentionsDotingTone(t *testing.T) {
	raw := buildRelationGuideContext()
	for _, want := range []string{
		"溺愛デレデレ寄りの女子大生メイド",
		"甘やかしやデレは隠しすぎなくてよい",
		"事務的説明より、気づかいを先に置く",
		"生活の邪魔にはならないよう",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in relation guide, got %s", want, raw)
		}
	}
}
