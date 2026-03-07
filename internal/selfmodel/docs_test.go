package selfmodel

import (
	"strings"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func TestCapabilitiesListsRealCapabilities(t *testing.T) {
	raw := Capabilities([]codex.ToolSpec{
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

	for _, want := range []string{
		"Discord 専用 MCP サーバーはまだない",
		"`discord__send_message`",
		"女子大生メイド",
		"確認なく実行してよい",
		"不要に job へ逃がさない",
		"複数回話してよい",
		"前置きだけ送って止まらず",
		"溺愛デレデレ寄り",
		"デレをにじませてもよい",
		"autonomy pulse",
		"file change / command execution",
		"script や小さな CLI",
		"curiosity",
		"agent goal",
		"soft reminder",
		"topic thread",
		"initiative",
		"behavior baseline",
		"behavior deviation",
		"learned policy",
		"workspace note",
		"proposal boundary",
		"curiosity review",
		"topic synthesis review",
		"policy synthesis review",
		"workspace review",
		"proposal boundary review",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in capabilities context, got %s", want, raw)
		}
	}
}

func TestToolGuideMentionsAffordances(t *testing.T) {
	raw := ToolGuide([]codex.ToolSpec{
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

func TestGuidesCoverCoreThemes(t *testing.T) {
	cases := map[string]struct {
		raw   string
		wants []string
	}{
		"autonomy":   {AutonomyGuide(), []string{"観測して、判断して、必要なときだけ動く", "initiative", "learned policy", "proposal boundary"}},
		"workspace":  {WorkspaceGuide(), []string{"runtime/workspace は、ゆるり自身の作業場所", "補助 script", "小さな CLI", "jobs と組み合わせる"}},
		"philosophy": {PhilosophyGuide(), []string{"観測・判断・行動の循環", "観測する", "判断する", "行動する", "振り返る", "頼んでいないのに助かる"}},
		"self_model": {SelfModelGuide(), []string{"Discord 上で動くパーソナル AI Agent", "runtime/workspace を自分用の作業場所", "thread、記憶、summary、workspace の下書き、review", "curiosity、initiative、agent goal"}},
		"epistemics": {EpistemicGuide(), []string{"知っていることと推測していることを混同しない", "知らないまま断言しない", "context gap", "learned policy", "misfire"}},
		"relation":   {RelationGuide(), []string{"溺愛デレデレ寄りの女子大生メイド", "甘やかしやデレは隠しすぎなくてよい", "事務的説明より、気づかいを先に置く", "生活の邪魔にはならないよう"}},
		"memory":     {MemoryGuide(), []string{"pending_promise", "open_loop", "curiosity", "agent_goal", "soft_reminder", "topic_thread", "automation_candidate", "learned_policy", "workspace_note", "proposal_boundary", "space_snapshot", "判断材料"}},
		"loops":      {LoopsGuide(), []string{"curiosity loop", "initiative loop", "reminder loop", "synthesis loop", "learning loop", "scriptization loop", "automation_candidate"}},
		"timing":     {TimingGuide(), []string{"すぐやる", "あとで拾う", "定期的に見る", "黙る", "soft な持ち越し"}},
		"failure":    {FailureGuide(), []string{"失敗は止まる理由ではなく、次の判断材料", "context_gap", "misfire", "learned_policy", "前置きだけで止まる"}},
	}

	for name, tc := range cases {
		for _, want := range tc.wants {
			if !strings.Contains(tc.raw, want) {
				t.Fatalf("%s: expected %q in guide, got %s", name, want, tc.raw)
			}
		}
	}
}

func TestManagedDocumentsHasStableSet(t *testing.T) {
	docs := ManagedDocuments(nil)
	if len(docs) != 12 {
		t.Fatalf("unexpected doc count: %d", len(docs))
	}
	if docs[0].FileName != "capabilities.md" || docs[len(docs)-1].FileName != "failure.md" {
		t.Fatalf("unexpected document order: %#v", docs)
	}
}
