package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"q2/internal/providers"
	"q2/internal/skills"
	"q2/internal/tools"
)

type Config struct {
	Provider  providers.Provider
	Tools     *tools.Registry
	Skills    skills.Set
	MaxSteps  int
	Workspace string
}

type Runner struct {
	provider  providers.Provider
	tools     *tools.Registry
	skills    skills.Set
	maxSteps  int
	workspace string
	history   []providers.Message
}

func New(cfg Config) *Runner {
	maxSteps := cfg.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 8
	}
	r := &Runner{
		provider:  cfg.Provider,
		tools:     cfg.Tools,
		skills:    cfg.Skills,
		maxSteps:  maxSteps,
		workspace: cfg.Workspace,
	}
	r.history = append(r.history, providers.Message{
		Role:    "system",
		Content: r.systemPrompt(),
	})
	return r
}

func (r *Runner) Run(ctx context.Context, userInput string) (string, error) {
	if strings.TrimSpace(userInput) == "" {
		return "", errors.New("prompt is empty")
	}
	messages := append([]providers.Message{}, r.history...)
	messages = append(messages, providers.Message{Role: "user", Content: userInput})

	for step := 0; step < r.maxSteps; step++ {
		reply, err := r.provider.Chat(ctx, providers.Request{
			Messages: messages,
			Tools:    r.tools.Specs(),
		})
		if err != nil {
			return "", err
		}
		if len(reply.ToolCalls) == 0 {
			messages = append(messages, providers.Message{Role: "assistant", Content: reply.Content})
			r.history = compactHistory(messages)
			return reply.Content, nil
		}
		messages = append(messages, providers.Message{
			Role:    "assistant",
			Content: toolCallSummary(reply.ToolCalls),
		})
		for _, call := range reply.ToolCalls {
			result, err := r.tools.Call(ctx, call.Name, call.Arguments)
			if err != nil {
				result = tools.Result{OK: false, Error: err.Error()}
			}
			payload, _ := json.Marshal(result)
			messages = append(messages, providers.Message{
				Role:    "tool",
				Name:    call.Name,
				Content: string(payload),
			})
		}
	}
	return "", fmt.Errorf("agent stopped after %d steps without final answer", r.maxSteps)
}

func (r *Runner) systemPrompt() string {
	var b strings.Builder
	b.WriteString("You are q2-agent, a CLI coding assistant running inside a local workspace.\n")
	b.WriteString("Use tools when you need filesystem facts or command output. Keep final answers concise.\n")
	b.WriteString("When a tool is needed, return JSON: {\"tool_calls\":[{\"name\":\"tool_name\",\"arguments\":{...}}]}.\n")
	b.WriteString("When no tool is needed, answer normally.\n")
	b.WriteString("Workspace: ")
	b.WriteString(r.workspace)
	b.WriteString("\nAvailable tools:\n")
	for _, spec := range r.tools.Specs() {
		b.WriteString("- ")
		b.WriteString(spec.Name)
		b.WriteString(": ")
		b.WriteString(spec.Description)
		b.WriteString("\n")
	}
	if len(r.skills) > 0 {
		b.WriteString("Loaded skills:\n")
		for _, skill := range r.skills {
			b.WriteString("- ")
			b.WriteString(skill.Name)
			b.WriteString(": ")
			b.WriteString(skill.Summary)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func toolCallSummary(calls []providers.ToolCall) string {
	data, _ := json.Marshal(map[string]any{"tool_calls": calls})
	return string(data)
}

func compactHistory(messages []providers.Message) []providers.Message {
	if len(messages) <= 20 {
		return messages
	}
	kept := make([]providers.Message, 0, 20)
	kept = append(kept, messages[0])
	kept = append(kept, messages[len(messages)-19:]...)
	return kept
}
