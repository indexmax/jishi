package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAICompatible struct {
	cfg    Config
	client *http.Client
}

func NewOpenAICompatible(cfg Config) *OpenAICompatible {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	return &OpenAICompatible{cfg: cfg, client: http.DefaultClient}
}

func (p *OpenAICompatible) Chat(ctx context.Context, req Request) (Response, error) {
	body := map[string]any{
		"model":       p.cfg.Model,
		"messages":    req.Messages,
		"temperature": 0.2,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("provider status %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Response{}, err
	}
	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("provider returned no choices")
	}
	content := parsed.Choices[0].Message.Content
	calls, ok := ParseToolCalls(content)
	if ok {
		return Response{Content: content, ToolCalls: calls}, nil
	}
	return Response{Content: content}, nil
}
