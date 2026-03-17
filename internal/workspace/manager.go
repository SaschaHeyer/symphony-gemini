package workspace

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/symphony-go/symphony/internal/config"
)

// Workspace represents a per-issue workspace directory.
type Workspace struct {
	Path         string
	WorkspaceKey string
	CreatedNow   bool
}

// Manager handles workspace creation, reuse, and cleanup.
type Manager struct {
	root  string
	hooks *config.HooksConfig
}

// NewManager creates a new workspace manager.
func NewManager(root string, hooks *config.HooksConfig) *Manager {
	return &Manager{root: root, hooks: hooks}
}

// UpdateConfig updates the manager's hooks configuration (for hot reload).
func (m *Manager) UpdateConfig(hooks *config.HooksConfig) {
	m.hooks = hooks
}

// CreateForIssue creates or reuses a workspace for an issue identifier.
func (m *Manager) CreateForIssue(identifier string) (*Workspace, error) {
	key := SanitizeIdentifier(identifier)
	wsPath := filepath.Join(m.root, key)

	if err := ValidateWorkspacePath(wsPath, m.root); err != nil {
		return nil, fmt.Errorf("workspace safety check failed: %w", err)
	}

	// Check if directory already exists
	createdNow := false
	info, err := os.Stat(wsPath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(wsPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspace directory: %w", err)
		}
		createdNow = true
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat workspace path: %w", err)
	} else if !info.IsDir() {
		// Path exists but is not a directory — remove and recreate
		if err := os.Remove(wsPath); err != nil {
			return nil, fmt.Errorf("workspace path exists as non-directory and cannot be removed: %w", err)
		}
		if err := os.MkdirAll(wsPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspace directory: %w", err)
		}
		createdNow = true
	}

	// Run after_create hook only on new creation
	if createdNow && m.hooks != nil && m.hooks.AfterCreate != nil {
		if err := RunHook("after_create", *m.hooks.AfterCreate, wsPath, m.hooks.TimeoutMs); err != nil {
			// Failure is fatal — remove the partially created workspace
			os.RemoveAll(wsPath)
			return nil, fmt.Errorf("after_create hook failed: %w", err)
		}
	}

	return &Workspace{
		Path:         wsPath,
		WorkspaceKey: key,
		CreatedNow:   createdNow,
	}, nil
}

// CleanWorkspace removes a workspace directory for an issue identifier.
func (m *Manager) CleanWorkspace(identifier string) error {
	key := SanitizeIdentifier(identifier)
	wsPath := filepath.Join(m.root, key)

	if err := ValidateWorkspacePath(wsPath, m.root); err != nil {
		return fmt.Errorf("workspace safety check failed: %w", err)
	}

	// Run before_remove hook if directory exists
	if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
		if m.hooks != nil && m.hooks.BeforeRemove != nil {
			if err := RunHook("before_remove", *m.hooks.BeforeRemove, wsPath, m.hooks.TimeoutMs); err != nil {
				slog.Warn("before_remove hook failed, continuing with cleanup",
					"error", err,
					"workspace", wsPath,
				)
			}
		}
	}

	return os.RemoveAll(wsPath)
}

// RunBeforeRun executes the before_run hook if configured.
// Returns error on failure (fatal to attempt).
func (m *Manager) RunBeforeRun(workspacePath string) error {
	if m.hooks != nil && m.hooks.BeforeRun != nil {
		return RunHook("before_run", *m.hooks.BeforeRun, workspacePath, m.hooks.TimeoutMs)
	}
	return nil
}

// RunAfterRun executes the after_run hook if configured.
// Failures are logged and ignored.
func (m *Manager) RunAfterRun(workspacePath string) {
	if m.hooks != nil && m.hooks.AfterRun != nil {
		if err := RunHook("after_run", *m.hooks.AfterRun, workspacePath, m.hooks.TimeoutMs); err != nil {
			slog.Warn("after_run hook failed", "error", err, "workspace", workspacePath)
		}
	}
}
