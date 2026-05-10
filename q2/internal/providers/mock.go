package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Mock struct{}

func (Mock) Chat(ctx context.Context, req Request) (Response, error) {
	select {
	case <-ctx.Done():
		return Response{}, ctx.Err()
	default:
	}
	last := lastMessage(req.Messages)
	if last.Role == "tool" {
		return Response{Content: "工具执行结果：\n" + last.Content}, nil
	}
	text := strings.TrimSpace(last.Content)
	lower := strings.ToLower(text)

	switch {
	case strings.HasPrefix(lower, "read "):
		path := strings.TrimSpace(text[len("read "):])
		return Response{ToolCalls: []ToolCall{{Name: "read_file", Arguments: map[string]any{"path": path}}}}, nil
	case strings.HasPrefix(lower, "list "):
		path := strings.TrimSpace(text[len("list "):])
		return Response{ToolCalls: []ToolCall{{Name: "list_dir", Arguments: map[string]any{"path": path}}}}, nil
	case strings.HasPrefix(lower, "shell "):
		command := strings.TrimSpace(text[len("shell "):])
		return Response{ToolCalls: []ToolCall{{Name: "shell", Arguments: map[string]any{"command": command}}}}, nil
	case strings.HasPrefix(lower, "write "):
		payload := strings.TrimSpace(text[len("write "):])
		parts := strings.SplitN(payload, " ", 2)
		if len(parts) != 2 {
			return Response{Content: "用法：write <path> <content>"}, nil
		}
		return Response{ToolCalls: []ToolCall{{Name: "write_file", Arguments: map[string]any{"path": parts[0], "content": parts[1]}}}}, nil
	default:
		names := make([]string, 0, len(req.Tools))
		for _, spec := range req.Tools {
			names = append(names, spec.Name)
		}
		return Response{Content: fmt.Sprintf("mock provider 已收到：%s\n可用工具：%s", text, strings.Join(names, ", "))}, nil
	}
}

func lastMessage(messages []Message) Message {
	if len(messages) == 0 {
		return Message{}
	}
	return messages[len(messages)-1]
}

func toolJSON(name string, args map[string]any) string {
	data, _ := json.Marshal(map[string]any{
		"tool_calls": []ToolCall{{Name: name, Arguments: args}},
	})
	return string(data)
}
