package tools

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func RegisterShellTool(registry *Registry, workspace string) error {
	return registry.Register(Spec{
		Name:        "shell",
		Description: "Run a command in the workspace and return combined output.",
		Parameters:  map[string]any{"command": "command line", "timeoutSeconds": "optional timeout"},
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		var in struct {
			Command        string `json:"command"`
			TimeoutSeconds int    `json:"timeoutSeconds"`
		}
		if err := BindArgs(args, &in); err != nil {
			return Result{}, err
		}
		if strings.TrimSpace(in.Command) == "" {
			return Result{}, nil
		}
		timeout := time.Duration(in.TimeoutSeconds) * time.Second
		if timeout <= 0 || timeout > 60*time.Second {
			timeout = 20 * time.Second
		}
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(runCtx, "powershell", "-NoProfile", "-Command", in.Command)
		} else {
			cmd = exec.CommandContext(runCtx, "sh", "-c", in.Command)
		}
		cmd.Dir = workspace
		output, err := cmd.CombinedOutput()
		result := Result{OK: err == nil, Output: string(output)}
		if runCtx.Err() == context.DeadlineExceeded {
			result.Error = "command timed out"
			return result, nil
		}
		if err != nil {
			result.Error = err.Error()
		}
		return result, nil
	})
}
