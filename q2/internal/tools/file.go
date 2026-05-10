package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RegisterFileTools(registry *Registry, workspace string) error {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	if err := registry.Register(Spec{
		Name:        "read_file",
		Description: "Read a UTF-8 text file inside the workspace.",
		Parameters:  map[string]any{"path": "relative file path"},
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		var in struct {
			Path string `json:"path"`
		}
		if err := BindArgs(args, &in); err != nil {
			return Result{}, err
		}
		path, err := safePath(root, in.Path)
		if err != nil {
			return Result{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, err
		}
		return Result{OK: true, Output: string(data)}, nil
	}); err != nil {
		return err
	}

	if err := registry.Register(Spec{
		Name:        "write_file",
		Description: "Create or replace a UTF-8 text file inside the workspace.",
		Parameters:  map[string]any{"path": "relative file path", "content": "new file content"},
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		var in struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := BindArgs(args, &in); err != nil {
			return Result{}, err
		}
		path, err := safePath(root, in.Path)
		if err != nil {
			return Result{}, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(path, []byte(in.Content), 0644); err != nil {
			return Result{}, err
		}
		return Result{OK: true, Output: "written " + in.Path, Changed: true}, nil
	}); err != nil {
		return err
	}

	return registry.Register(Spec{
		Name:        "list_dir",
		Description: "List files and directories inside the workspace.",
		Parameters:  map[string]any{"path": "relative directory path"},
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		var in struct {
			Path string `json:"path"`
		}
		if err := BindArgs(args, &in); err != nil {
			return Result{}, err
		}
		path, err := safePath(root, in.Path)
		if err != nil {
			return Result{}, err
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return Result{}, err
		}
		var out strings.Builder
		for _, entry := range entries {
			suffix := ""
			if entry.IsDir() {
				suffix = "/"
			}
			out.WriteString(entry.Name())
			out.WriteString(suffix)
			out.WriteByte('\n')
		}
		return Result{OK: true, Output: out.String()}, nil
	})
}

func safePath(root, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		requested = "."
	}
	cleaned := filepath.Clean(requested)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	full, err := filepath.Abs(filepath.Join(root, cleaned))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	return full, nil
}
