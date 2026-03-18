package cmux

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/symphony-go/symphony/internal/config"
)

const fallbackCmuxBin = "/Applications/cmux.app/Contents/Resources/bin/cmux"

// Manager handles cmux workspace and surface lifecycle.
// All methods are safe to call even when cmux is disabled — they become no-ops.
type Manager struct {
	enabled      bool
	closeDelayMs int
	cmuxBin      string
	surfaces     map[string]string   // issueID → workspace ref
	logFiles     map[string]*os.File // issueID → log file handle
	mu           sync.Mutex
}

// New creates a CmuxManager. If enabled, it locates the cmux binary.
// Returns a no-op manager if disabled or binary not found.
func New(cfg *config.CmuxConfig) *Manager {
	m := &Manager{
		surfaces: make(map[string]string),
		logFiles: make(map[string]*os.File),
	}

	if cfg == nil || !cfg.Enabled {
		return m
	}

	m.closeDelayMs = cfg.CloseDelayMs

	// Locate cmux binary
	bin, err := exec.LookPath("cmux")
	if err != nil {
		// Try fallback path
		if _, statErr := os.Stat(fallbackCmuxBin); statErr == nil {
			bin = fallbackCmuxBin
		} else {
			slog.Warn("cmux binary not found, disabling cmux visibility")
			return m
		}
	}

	m.cmuxBin = bin
	m.enabled = true
	return m
}

// Enabled returns whether cmux visibility is active.
func (m *Manager) Enabled() bool {
	return m.enabled
}

// Init verifies cmux connectivity. Called once at startup.
// Each dispatched issue gets its own workspace via CreateSurface.
func (m *Manager) Init() error {
	if !m.enabled {
		return nil
	}

	// Verify cmux is responsive
	if _, err := m.run("ping"); err != nil {
		slog.Warn("cmux ping failed, disabling cmux visibility", "error", err)
		m.enabled = false
		return fmt.Errorf("cmux ping failed: %w", err)
	}

	slog.Info("cmux visibility enabled")
	return nil
}

// extractUUID extracts a UUID from cmux output like "OK <UUID>".
func extractUUID(output string) string {
	for _, token := range strings.Fields(output) {
		// UUIDs are 36 chars: 8-4-4-4-12 hex digits with dashes
		if len(token) == 36 && strings.Count(token, "-") == 4 {
			return token
		}
	}
	return ""
}

