package codex

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
