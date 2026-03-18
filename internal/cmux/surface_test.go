package cmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateSurface(t *testing.T) {
	m, logFile := newTestManager(t, "mock_cmux.sh")

	wsPath := t.TempDir()
	err := m.CreateSurface("issue-1", "AIE-10", wsPath)
	if err != nil {
		t.Fatalf("CreateSurface failed: %v", err)
	}

	// Verify cmux calls
	calls := readCallLog(t, logFile)
	if !strings.Contains(calls, "new-workspace") {
		t.Error("expected new-workspace call")
	}
	if !strings.Contains(calls, "workspace-action") {
		t.Error("expected workspace-action rename call")
	}
	if !strings.Contains(calls, "AIE-10") {
		t.Error("expected issue identifier in workspace-action call")
	}
}

func TestCreateSurfaceCreatesLogFile(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")

	wsPath := t.TempDir()
	err := m.CreateSurface("issue-1", "AIE-10", wsPath)
	if err != nil {
		t.Fatalf("CreateSurface failed: %v", err)
	}

	logPath := filepath.Join(wsPath, agentLogFile)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("expected log file to be created")
	}

	m.mu.Lock()
	_, hasFile := m.logFiles["issue-1"]
	m.mu.Unlock()
	if !hasFile {
		t.Error("expected log file in manager map")
	}
}

func TestCreateSurfaceReuse(t *testing.T) {
	m, logFile := newTestManager(t, "mock_cmux.sh")

	wsPath := t.TempDir()
	m.CreateSurface("issue-1", "AIE-10", wsPath)

	// Clear log to track second call
	os.Truncate(logFile, 0)

	// Second call should be a no-op
	err := m.CreateSurface("issue-1", "AIE-10", wsPath)
	if err != nil {
		t.Fatalf("second CreateSurface failed: %v", err)
	}

	calls := readCallLog(t, logFile)
	if strings.Contains(calls, "new-workspace") {
		t.Error("should not call new-workspace on reuse")
	}
}

func TestCloseSurface(t *testing.T) {
	m, logFile := newTestManager(t, "mock_cmux.sh")
	m.closeDelayMs = 100

	wsPath := t.TempDir()
	m.CreateSurface("issue-1", "AIE-10", wsPath)

	os.Truncate(logFile, 0)

	m.CloseSurface("issue-1")

	// Wait for the delayed close goroutine
	time.Sleep(300 * time.Millisecond)

	calls := readCallLog(t, logFile)
	if !strings.Contains(calls, "close-workspace") {
		t.Error("expected close-workspace call after delay")
	}

	m.mu.Lock()
	_, hasSurface := m.surfaces["issue-1"]
	_, hasFile := m.logFiles["issue-1"]
	m.mu.Unlock()

	if hasSurface {
		t.Error("expected workspace ref to be removed from map")
	}
	if hasFile {
		t.Error("expected log file to be removed from map")
	}
}

func TestCloseSurfaceUnknownID(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")
	m.CloseSurface("nonexistent")
}

func TestShutdown(t *testing.T) {
	m, logFile := newTestManager(t, "mock_cmux.sh")

	for i, id := range []string{"issue-1", "issue-2", "issue-3"} {
		wsPath := filepath.Join(t.TempDir(), id)
		os.MkdirAll(wsPath, 0755)
		m.CreateSurface(id, "AIE-"+string(rune('1'+i)), wsPath)
	}

	os.Truncate(logFile, 0)

	m.Shutdown()

	calls := readCallLog(t, logFile)
	closeCount := strings.Count(calls, "close-workspace")
	if closeCount != 3 {
		t.Errorf("expected 3 close-workspace calls, got %d", closeCount)
	}

	m.mu.Lock()
	surfaceCount := len(m.surfaces)
	fileCount := len(m.logFiles)
	m.mu.Unlock()

	if surfaceCount != 0 {
		t.Errorf("expected 0 surfaces after shutdown, got %d", surfaceCount)
	}
	if fileCount != 0 {
		t.Errorf("expected 0 log files after shutdown, got %d", fileCount)
	}
}
