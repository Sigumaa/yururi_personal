package bot

import (
	"strings"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/decision"
)

func TestLooksLikePromiseOnly(t *testing.T) {
	if !looksLikePromiseOnly("必要そうなところから順に手をつけますね") {
		t.Fatal("expected promise-like reply to be detected")
	}
	if !looksLikePromiseOnly("承知しました。いま確認しますね。") {
		t.Fatal("expected filler plus promise reply to be detected")
	}
	if looksLikePromiseOnly("監視を登録しておきましたよ") {
		t.Fatal("expected completed factual reply to pass")
	}
	if looksLikePromiseOnly("もちろん、どっちも今ここで答えるね。つまり、今すぐ終わる作業はその場で実行して、継続監視だけ job を更新します。") {
		t.Fatal("expected explanatory reply to pass")
	}
	if looksLikePromiseOnly("了解です。\n- create\n- move") {
		t.Fatal("expected multi-line progress list to pass")
	}
}

func TestExecutionReportRender(t *testing.T) {
	report := executionReport{
		MemoryWrites: []string{"preference/tone"},
		Actions:      []string{"created channel openclaw (123)"},
		Jobs:         []string{"scheduled job watch-1 (url_watch)"},
	}
	raw := report.Render()
	if !strings.Contains(raw, "created channel openclaw") || !strings.Contains(raw, "scheduled job watch-1") {
		t.Fatalf("unexpected report: %s", raw)
	}
}

func TestDecisionSchemaAllowsBackgroundPrompt(t *testing.T) {
	schema := decision.OutputSchema()
	properties := schema["properties"].(map[string]any)
	jobsSchema := properties["jobs"].(map[string]any)
	items := jobsSchema["items"].(map[string]any)
	jobProperties := items["properties"].(map[string]any)
	payload := jobProperties["payload"].(map[string]any)
	payloadProperties := payload["properties"].(map[string]any)
	if _, ok := payloadProperties["prompt"]; !ok {
		t.Fatal("expected payload.prompt in decision schema")
	}
}
