package workspace

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var safeChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// SanitizeIdentifier replaces any character not in [A-Za-z0-9._-] with _.
func SanitizeIdentifier(identifier string) string {
	return safeChars.ReplaceAllString(identifier, "_")
}

// ValidateWorkspacePath checks that workspacePath is inside workspaceRoot.
func ValidateWorkspacePath(workspacePath, workspaceRoot string) error {
	absPath, err := filepath.Abs(workspacePath)
	if err != nil {
		return fmt.Errorf("invalid workspace path: %w", err)
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("invalid workspace root: %w", err)
	}

	// Ensure the path is a child of root
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return fmt.Errorf("workspace path %q is not relative to root %q: %w", absPath, absRoot, err)
	}

	// Reject path traversal
	if strings.HasPrefix(rel, "..") || rel == "." {
		return fmt.Errorf("workspace path %q is outside workspace root %q", absPath, absRoot)
	}

	return nil
}
