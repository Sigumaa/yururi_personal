package codex

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

func TestThreadStartParamsIncludeDynamicTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolSpec{
		Name:        "discord.create_channel",
		Description: "create a channel",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}, nil)

	client := NewClient(config.Config{
		AppName: "yururi",
		Codex: config.CodexConfig{
			ApprovalPolicy: "never",
			SandboxMode:    "danger-full-access",
		},
	}, config.Paths{Workspace: "/tmp/workspace"}, slog.New(slog.NewTextHandler(io.Discard, nil)), registry)

	params := client.threadStartParams("base", "dev")
	dynamicTools, ok := params["dynamicTools"].([]map[string]any)
	if !ok {
		t.Fatalf("dynamicTools type = %T", params["dynamicTools"])
	}
	if len(dynamicTools) != 1 {
		t.Fatalf("dynamicTools len = %d", len(dynamicTools))
	}
	if dynamicTools[0]["name"] != "discord__create_channel" {
		t.Fatalf("unexpected tool name: %#v", dynamicTools[0]["name"])
	}
}

func TestDynamicToolSignatureChangesWithSpecs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{AppName: "yururi"}
	paths := config.Paths{Workspace: "/tmp/workspace"}

	registryA := NewToolRegistry()
	registryA.Register(ToolSpec{
		Name:        "discord.create_channel",
		Description: "create a channel",
		InputSchema: map[string]any{"type": "object"},
	}, nil)
	clientA := NewClient(cfg, paths, logger, registryA)

	registryB := NewToolRegistry()
	registryB.Register(ToolSpec{
		Name:        "discord.create_channel",
		Description: "create a channel",
		InputSchema: map[string]any{"type": "object"},
	}, nil)
	registryB.Register(ToolSpec{
		Name:        "discord.create_category",
		Description: "create a category",
		InputSchema: map[string]any{"type": "object"},
	}, nil)
	clientB := NewClient(cfg, paths, logger, registryB)

	if clientA.DynamicToolSignature() == "" {
		t.Fatal("empty signature")
	}
	if clientA.DynamicToolSignature() == clientB.DynamicToolSignature() {
		aRaw, _ := json.Marshal(registryA.Specs())
		bRaw, _ := json.Marshal(registryB.Specs())
		t.Fatalf("signatures should differ: a=%s b=%s", string(aRaw), string(bRaw))
	}
}

func TestDynamicToolSignatureUsesExternalToolNames(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolSpec{
		Name:        "discord.send_message",
		Description: "send a message",
		InputSchema: map[string]any{"type": "object"},
	}, nil)

	client := NewClient(config.Config{AppName: "yururi"}, config.Paths{Workspace: "/tmp/workspace"}, slog.New(slog.NewTextHandler(io.Discard, nil)), registry)

	dynamicTools := client.dynamicToolParams()
	raw, err := json.Marshal(dynamicTools)
	if err != nil {
		t.Fatalf("marshal dynamic tools: %v", err)
	}
	sum := sha256.Sum256(raw)
	want := fmt.Sprintf("%x", sum)

	if got := client.DynamicToolSignature(); got != want {
		t.Fatalf("signature mismatch: got %s want %s", got, want)
	}
}

func TestHandleNotificationMarksInterruptedTurn(t *testing.T) {
	client := NewClient(config.Config{AppName: "yururi"}, config.Paths{Workspace: "/tmp/workspace"}, slog.New(slog.NewTextHandler(io.Discard, nil)), NewToolRegistry())
	waiter := &turnWaiter{
		threadID:   "thread-1",
		turnID:     "turn-1",
		completed:  make(chan turnResult, 1),
		receivedAt: time.Now(),
	}

	client.stateMu.Lock()
	client.turns["turn-1"] = waiter
	client.stateMu.Unlock()

	client.handleNotification("turn/completed", json.RawMessage(`{"turn":{"id":"turn-1","status":"interrupted","error":null}}`))

	select {
	case result := <-waiter.completed:
		if !errors.Is(result.Error, ErrTurnInterrupted) {
			t.Fatalf("unexpected result: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("expected interrupted turn result")
	}
}

func TestNormalizeInterruptedResult(t *testing.T) {
	if err := normalizeInterruptedResult(turnResult{Error: ErrTurnInterrupted}); !errors.Is(err, ErrTurnInterrupted) {
		t.Fatalf("expected interrupted error, got %v", err)
	}

	otherErr := errors.New("other")
	if err := normalizeInterruptedResult(turnResult{Error: otherErr}); !errors.Is(err, otherErr) {
		t.Fatalf("expected original error, got %v", err)
	}

	if err := normalizeInterruptedResult(turnResult{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestDecodeIDValuePreservesJSONRPCIDType(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "number",
			raw:  json.RawMessage(`60`),
			want: `60`,
		},
		{
			name: "string",
			raw:  json.RawMessage(`"call-1"`),
			want: `"call-1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(decodeIDValue(tt.raw))
			if err != nil {
				t.Fatalf("marshal id value: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("unexpected id value: got %s want %s", string(got), tt.want)
			}
		})
	}
}

func TestTurnParamsAllowEffortOverride(t *testing.T) {
	client := NewClient(config.Config{
		AppName: "yururi",
		Codex: config.CodexConfig{
			ApprovalPolicy:   "never",
			SandboxMode:      "danger-full-access",
			ReasoningEffort:  "medium",
			ReasoningSummary: "concise",
		},
	}, config.Paths{Workspace: "/tmp/workspace"}, slog.New(slog.NewTextHandler(io.Discard, nil)), NewToolRegistry())

	params := client.turnParams("thread-1", []InputItem{TextInput("hello")}, nil, TurnOptions{Effort: "low"})
	if got, _ := params["effort"].(string); got != "low" {
		t.Fatalf("unexpected effort override: %#v", params["effort"])
	}
	if got, _ := params["summary"].(string); got != "concise" {
		t.Fatalf("expected summary to stay inherited, got %#v", params["summary"])
	}
}

func TestHandleConnectionLossFailsPendingAndTurns(t *testing.T) {
	client := NewClient(config.Config{AppName: "yururi"}, config.Paths{Workspace: "/tmp/workspace"}, slog.New(slog.NewTextHandler(io.Discard, nil)), NewToolRegistry())

	pendingCh := make(chan rpcResponse, 1)
	waiter := &turnWaiter{
		threadID:   "thread-1",
		turnID:     "turn-1",
		completed:  make(chan turnResult, 1),
		receivedAt: time.Now(),
	}

	client.stateMu.Lock()
	client.pending["1"] = pendingCh
	client.turns["turn-1"] = waiter
	client.stateMu.Unlock()

	client.handleConnectionLoss(errors.New("unexpected EOF"))

	select {
	case response := <-pendingCh:
		if response.Error == nil || !strings.Contains(response.Error.Message, "app-server connection lost") {
			t.Fatalf("unexpected pending response: %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("expected pending call to be failed")
	}

	select {
	case result := <-waiter.completed:
		if result.Error == nil || !strings.Contains(result.Error.Error(), "app-server connection lost") {
			t.Fatalf("unexpected turn result: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("expected active turn to be failed")
	}

	client.stateMu.Lock()
	defer client.stateMu.Unlock()
	if client.conn != nil {
		t.Fatal("expected connection to be cleared")
	}
	if len(client.pending) != 0 {
		t.Fatalf("expected pending map to be cleared, got %d", len(client.pending))
	}
	if len(client.turns) != 0 {
		t.Fatalf("expected turns map to be cleared, got %d", len(client.turns))
	}
}
