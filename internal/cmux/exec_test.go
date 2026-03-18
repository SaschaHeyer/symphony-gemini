package cmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSuccess(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")

	out, err := m.run("ping")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if out != "PONG" {
		t.Errorf("expected %q, got %q", "PONG", out)
	}
}

func TestRunTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	// Create a mock that sleeps longer than cmdTimeout (5s)
	sleepScript := filepath.Join(t.TempDir(), "slow_cmux.sh")
	os.WriteFile(sleepScript, []byte("#!/bin/bash\nsleep 30\n"), 0755)

	m := &Manager{
		enabled: true,
		cmuxBin: sleepScript,
	}

	_, err := m.run("ping")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "cmux ping") {
		t.Errorf("expected error to contain command info, got: %v", err)
	}
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		output   string
		prefix   string
		expected string
	}{
		{"OK workspace:5", "workspace:", "workspace:5"},
		{"OK surface:12 pane:3 workspace:5", "surface:", "surface:12"},
		{"OK surface:12 pane:3 workspace:5", "workspace:", "workspace:5"},
		{"OK surface:12 pane:3 workspace:5", "pane:", "pane:3"},
		{"OK", "workspace:", ""},
		{"", "workspace:", ""},
	}

	for _, tt := range tests {
		got := parseRef(tt.output, tt.prefix)
		if got != tt.expected {
			t.Errorf("parseRef(%q, %q) = %q, want %q", tt.output, tt.prefix, got, tt.expected)
		}
	}
}

// newTestManager creates a Manager pointing at a mock cmux script.
// Returns the manager and the path to the call log file.
func newTestManager(t *testing.T, scriptName string) (*Manager, string) {
	t.Helper()

	scriptPath := filepath.Join("testdata", scriptName)
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("mock script not found: %s", scriptPath)
	}

	absScript, _ := filepath.Abs(scriptPath)
	logFile := filepath.Join(t.TempDir(), "cmux-calls.log")
	t.Setenv("CMUX_TEST_LOG", logFile)

	m := &Manager{
		enabled:      true,
		cmuxBin:      absScript,
		closeDelayMs: 100,
		surfaces:     make(map[string]string),
		logFiles:     make(map[string]*os.File),
	}

	return m, logFile
}

// readCallLog reads the mock cmux call log file.
func readCallLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
