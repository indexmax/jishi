package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type Spec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type Result struct {
	OK      bool   `json:"ok"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Changed bool   `json:"changed,omitempty"`
}

type Handler func(ctx context.Context, args map[string]any) (Result, error)

type registeredTool struct {
	spec    Spec
	handler Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]registeredTool{}}
}

func (r *Registry) Register(spec Spec, handler Handler) error {
	if spec.Name == "" {
		return fmt.Errorf("tool name required")
	}
	if handler == nil {
		return fmt.Errorf("tool handler required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[spec.Name]; exists {
		return fmt.Errorf("tool %q already registered", spec.Name)
	}
	r.tools[spec.Name] = registeredTool{spec: spec, handler: handler}
	return nil
}

func (r *Registry) Specs() []Spec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]Spec, 0, len(r.tools))
	for _, tool := range r.tools {
		specs = append(specs, tool.spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func (r *Registry) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", name)
	}
	if args == nil {
		args = map[string]any{}
	}
	return tool.handler(ctx, args)
}

func BindArgs(args map[string]any, target any) error {
	data, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
