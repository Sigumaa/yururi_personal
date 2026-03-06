package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
)

type ToolHandler func(context.Context, json.RawMessage) (ToolResponse, error)

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

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
	specs    map[string]ToolSpec
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		handlers: map[string]ToolHandler{},
		specs:    map[string]ToolSpec{},
	}
}

func (r *ToolRegistry) Register(spec ToolSpec, handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[spec.Name] = handler
	r.specs[spec.Name] = spec
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

func (r *ToolRegistry) Specs() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ToolSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		out = append(out, spec)
	}
	slices.SortFunc(out, func(a ToolSpec, b ToolSpec) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		default:
			return 0
		}
	})
	return out
}
