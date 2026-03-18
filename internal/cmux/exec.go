package cmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const cmdTimeout = 5 * time.Second

// run executes a cmux CLI command with a 5-second timeout.
// Automatically prepends --id-format refs so all output uses short refs.
// Returns trimmed stdout output.
func (m *Manager) run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	// --id-format refs is a global flag that must come before the command
	fullArgs := append([]string{"--id-format", "refs"}, args...)
	cmd := exec.CommandContext(ctx, m.cmuxBin, fullArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cmux %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseRef extracts a ref from cmux output by finding the token starting with prefix.
// Example: parseRef("OK workspace:5 pane:3", "workspace:") returns "workspace:5".
func parseRef(output, prefix string) string {
	for _, token := range strings.Fields(output) {
		if strings.HasPrefix(token, prefix) {
			return token
		}
	}
	return ""
}
