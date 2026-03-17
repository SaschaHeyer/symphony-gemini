package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// RunHook executes a shell script in the workspace directory with a timeout.
func RunHook(name string, script string, cwd string, timeoutMs int) error {
	if script == "" {
		return nil
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-lc", script)
	cmd.Dir = cwd

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		slog.Error("hook timed out",
			"hook", name,
			"timeout_ms", timeoutMs,
			"cwd", cwd,
		)
		return fmt.Errorf("hook %q timed out after %dms", name, timeoutMs)
	}

	if err != nil {
		// Truncate output for logging
		outStr := string(output)
		if len(outStr) > 500 {
			outStr = outStr[:500] + "... (truncated)"
		}
		slog.Error("hook failed",
			"hook", name,
			"error", err,
			"output", outStr,
			"cwd", cwd,
		)
		return fmt.Errorf("hook %q failed: %w", name, err)
	}

	slog.Debug("hook completed", "hook", name, "cwd", cwd)
	return nil
}
