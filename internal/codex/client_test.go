package codex

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

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
	if dynamicTools[0]["name"] != "discord.create_channel" {
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
