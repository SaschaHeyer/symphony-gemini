package cmux

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const agentLogFile = ".symphony-agent.log"

// CreateSurface creates a cmux workspace for an issue running `tail -f` on its log file.
// Each issue gets its own workspace named after the identifier (e.g., "AIE-10").
// If a surface already exists for this issue (retry), it is reused.
func (m *Manager) CreateSurface(issueID, identifier, workspacePath string) error {
	if !m.enabled {
		return nil
	}

	// Check for existing surface (reuse on retry)
	m.mu.Lock()
	if _, exists := m.surfaces[issueID]; exists {
		m.mu.Unlock()
		return nil
	}

	// Ensure workspace directory exists (it may not yet if the agent hasn't launched)
	os.MkdirAll(workspacePath, 0755)

	// Create log file
	logPath := filepath.Join(workspacePath, agentLogFile)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to create log file %s: %w", logPath, err)
	}
	m.logFiles[issueID] = f
	m.mu.Unlock()

	// Create a cmux workspace running tail -f on the log file.
	// new-workspace --command creates a real terminal with the command running in it.
	tailCmd := fmt.Sprintf("tail -f %s", logPath)
	out, err := m.run("new-workspace", "--command", tailCmd)
	if err != nil {
		m.mu.Lock()
		f.Close()
		delete(m.logFiles, issueID)
		m.mu.Unlock()
		return fmt.Errorf("cmux new-workspace failed: %w", err)
	}

	// Extract UUID for the new workspace
	wsRef := extractUUID(out)
	if wsRef == "" {
		wsRef = parseRef(out, "workspace:")
	}
	if wsRef == "" {
		m.mu.Lock()
		f.Close()
		delete(m.logFiles, issueID)
		m.mu.Unlock()
		return fmt.Errorf("could not parse workspace ref from: %s", out)
	}

	// Name the workspace after the issue identifier
	m.run("workspace-action", "--action", "rename", "--workspace", wsRef, "--title", identifier)

	// Store workspace ref (we track workspaces, not surfaces)
	m.mu.Lock()
	m.surfaces[issueID] = wsRef
	m.mu.Unlock()

	slog.Info("cmux surface created", "issue_identifier", identifier, "workspace", wsRef)
	return nil
}

// CloseSurface closes an issue's workspace after the configured delay.
// Spawns a goroutine to wait before closing, allowing users to read final output.
func (m *Manager) CloseSurface(issueID string) {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	wsRef, hasWs := m.surfaces[issueID]
	m.mu.Unlock()

	if !hasWs {
		return
	}

	m.WriteAnnotation(issueID, "Session ended")

	delay := time.Duration(m.closeDelayMs) * time.Millisecond

	go func() {
		time.Sleep(delay)

		m.run("close-workspace", "--workspace", wsRef)

		m.mu.Lock()
		if f, ok := m.logFiles[issueID]; ok {
			f.Close()
			delete(m.logFiles, issueID)
		}
		delete(m.surfaces, issueID)
		m.mu.Unlock()
	}()
}

// Shutdown closes all issue workspaces and log files immediately.
func (m *Manager) Shutdown() {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for issueID, ref := range m.surfaces {
		m.run("close-workspace", "--workspace", ref)
		delete(m.surfaces, issueID)
	}

	for issueID, f := range m.logFiles {
		f.Close()
		delete(m.logFiles, issueID)
	}
}
