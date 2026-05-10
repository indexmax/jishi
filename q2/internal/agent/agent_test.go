package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"q2/internal/providers"
	"q2/internal/tools"
)

func TestRunnerExecutesToolCall(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry()
	if err := tools.RegisterFileTools(registry, root); err != nil {
		t.Fatal(err)
	}
	runner := New(Config{
		Provider: &fakeProvider{
			responses: []providers.Response{
				{ToolCalls: []providers.ToolCall{{
					Name:      "write_file",
					Arguments: map[string]any{"path": "note.txt", "content": "hello agent"},
				}}},
				{Content: "done"},
			},
		},
		Tools:    registry,
		MaxSteps: 4,
	})

	answer, err := runner.Run(context.Background(), "create note")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("answer=%q", answer)
	}
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello agent" {
		t.Fatalf("file content=%q", data)
	}
}

func TestMockProviderCanUseShellTool(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry()
	if err := tools.RegisterShellTool(registry, root); err != nil {
		t.Fatal(err)
	}
	runner := New(Config{
		Provider: providers.Mock{},
		Tools:    registry,
		MaxSteps: 4,
	})

	answer, err := runner.Run(context.Background(), "shell go version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "工具执行结果") {
		t.Fatalf("answer=%q", answer)
	}
}

type fakeProvider struct {
	responses []providers.Response
	index     int
}

func (f *fakeProvider) Chat(ctx context.Context, req providers.Request) (providers.Response, error) {
	if f.index >= len(f.responses) {
		return providers.Response{Content: "fallback"}, nil
	}
	resp := f.responses[f.index]
	f.index++
	return resp, nil
}
