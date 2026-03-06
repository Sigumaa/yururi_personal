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
	if looksLikePromiseOnly("監視を登録しておきましたよ") {
		t.Fatal("expected completed factual reply to pass")
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
