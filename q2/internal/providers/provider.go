package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"q2/internal/tools"
)

type Config struct {
	Model   string
	APIKey  string
	BaseURL string
}

type Message struct {
	Role    string `json:"role"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
}

type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Request struct {
	Messages []Message
	Tools    []tools.Spec
}

type Response struct {
	Content   string
	ToolCalls []ToolCall
}

type Provider interface {
	Chat(ctx context.Context, req Request) (Response, error)
}

func New(name string, cfg Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return Mock{}, nil
	case "openai", "openai-compatible":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for provider %q", name)
		}
		return NewOpenAICompatible(cfg), nil
	case "qwen", "dashscope", "bailian", "aliyun", "aliyun-bailian":
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("DASHSCOPE_API_KEY")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("DASHSCOPE_API_KEY is required for provider %q", name)
		}
		if cfg.Model == "" || cfg.Model == "mock-agent" {
			cfg.Model = "qwen-plus"
		}
		if cfg.BaseURL == "" || cfg.BaseURL == "https://api.openai.com/v1" {
			cfg.BaseURL = getenv("DASHSCOPE_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1")
		}
		return NewOpenAICompatible(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}

func ParseToolCalls(content string) ([]ToolCall, bool) {
	var envelope struct {
		ToolCalls []ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &envelope); err != nil {
		return nil, false
	}
	if len(envelope.ToolCalls) == 0 {
		return nil, false
	}
	return envelope.ToolCalls, true
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
