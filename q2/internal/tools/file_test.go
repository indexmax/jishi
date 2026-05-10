package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileToolsReadWriteList(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()
	if err := RegisterFileTools(registry, root); err != nil {
		t.Fatal(err)
	}

	result, err := registry.Call(context.Background(), "write_file", map[string]any{
		"path":    "dir/a.txt",
		"content": "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !result.Changed {
		t.Fatalf("write result=%+v", result)
	}

	result, err = registry.Call(context.Background(), "read_file", map[string]any{"path": "dir/a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "alpha" {
		t.Fatalf("read output=%q", result.Output)
	}

	result, err = registry.Call(context.Background(), "list_dir", map[string]any{"path": "dir"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "a.txt\n" {
		t.Fatalf("list output=%q", result.Output)
	}
}

func TestSafePathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := safePath(root, "../outside.txt"); err == nil {
		t.Fatal("expected escaping path to be rejected")
	}
	if _, err := safePath(root, filepath.Join("..", "outside.txt")); err == nil {
		t.Fatal("expected parent path to be rejected")
	}
	if _, err := safePath(root, filepath.Base(os.TempDir())); err != nil {
		t.Fatalf("safe relative path rejected: %v", err)
	}
}
