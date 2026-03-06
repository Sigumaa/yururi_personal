package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type ToolHandler func(context.Context, json.RawMessage) (ToolResponse, error)

type ToolResponse struct {
	Success      bool              `json:"success"`
	ContentItems []ToolContentItem `json:"contentItems"`
}

type ToolContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type ToolRegistry struct {
	mu       sync.RWMutex
	handlers map[string]ToolHandler
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{handlers: map[string]ToolHandler{}}
}

func (r *ToolRegistry) Register(name string, handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *ToolRegistry) Call(ctx context.Context, name string, arguments json.RawMessage) (ToolResponse, error) {
	r.mu.RLock()
	handler := r.handlers[name]
	r.mu.RUnlock()
	if handler == nil {
		return ToolResponse{}, fmt.Errorf("unsupported tool: %s", name)
	}
	return handler(ctx, arguments)
}
