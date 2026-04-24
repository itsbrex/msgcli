package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// ToolHandler executes a single tool call.
type ToolHandler func(ctx context.Context, args json.RawMessage) (ToolCallResult, error)

// Tool pairs metadata with its handler.
type Tool struct {
	Info    ToolInfo
	Handler ToolHandler
}

// Registry stores available tools for a server.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds or replaces a tool. Panics on empty name (programmer error).
func (r *Registry) Register(t Tool) {
	if t.Info.Name == "" {
		panic("mcp: tool name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Info.Name] = t
}

// List returns tool metadata sorted by name (stable for clients).
func (r *Registry) List() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		infos = append(infos, t.Info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// Call dispatches a tools/call request.
func (r *Registry) Call(ctx context.Context, name string, args json.RawMessage) (ToolCallResult, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ToolCallResult{}, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Handler(ctx, args)
}
