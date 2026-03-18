package cmux

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/symphony-go/symphony/internal/config"
)

func TestNewDisabled(t *testing.T) {
	m := New(&config.CmuxConfig{Enabled: false})
	if m.enabled {
		t.Error("expected disabled manager")
	}

	// All methods should be safe no-ops
	if err := m.Init(); err != nil {
		t.Errorf("Init should return nil: %v", err)
	}
	if err := m.CreateSurface("id", "AIE-10", "/tmp/ws"); err != nil {
		t.Errorf("CreateSurface should return nil: %v", err)
	}
	m.WriteEvent("id", "test event")
	m.WriteAnnotation("id", "test annotation")
	m.CloseSurface("id")
	m.Shutdown()
}

func TestNewNilConfig(t *testing.T) {
	m := New(nil)
	if m.enabled {
		t.Error("expected disabled manager for nil config")
	}
	if m.surfaces == nil {
		t.Error("surfaces map should be initialized")
	}
	if m.logFiles == nil {
		t.Error("logFiles map should be initialized")
	}
}

func TestNewBinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	cfg := &config.CmuxConfig{
		Enabled:       true,
		WorkspaceName: "Test",
		CloseDelayMs:  1000,
	}

	result := newWithFallback(cfg, "/nonexistent/cmux")
	if result.enabled {
		t.Error("expected disabled manager when binary not found")
	}
}

func TestLogWriterDisabled(t *testing.T) {
	m := New(&config.CmuxConfig{Enabled: false})
	w := m.LogWriter("any-id")
	if w != io.Discard {
		t.Error("expected io.Discard for disabled manager")
	}
}

func TestInitSuccess(t *testing.T) {
	m, logFile := newTestManager(t, "mock_cmux.sh")

	err := m.Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if !m.enabled {
		t.Error("expected manager to remain enabled after Init")
	}

	calls := readCallLog(t, logFile)
	if !strings.Contains(calls, "ping") {
		t.Error("expected ping call")
	}
}

func TestExtractUUID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"OK 12345678-ABCD-1234-ABCD-123456789ABC", "12345678-ABCD-1234-ABCD-123456789ABC"},
		{"OK EBF83486-7ED5-453C-B4F1-D2CBA0B1D272", "EBF83486-7ED5-453C-B4F1-D2CBA0B1D272"},
		{"OK", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractUUID(tt.input)
		if got != tt.expected {
			t.Errorf("extractUUID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestInitPingFails(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux_fail.sh")

	err := m.Init()
	if err == nil {
		t.Fatal("expected error when ping fails")
	}
	if m.enabled {
		t.Error("expected manager to be disabled after ping failure")
	}
}

// newWithFallback is a test helper that allows overriding the fallback binary path.
func newWithFallback(cfg *config.CmuxConfig, fallback string) *Manager {
	m := &Manager{
		surfaces: make(map[string]string),
		logFiles: make(map[string]*os.File),
	}

	if cfg == nil || !cfg.Enabled {
		return m
	}

	m.closeDelayMs = cfg.CloseDelayMs

	bin, err := exec.LookPath("cmux")
	if err != nil {
		if _, statErr := os.Stat(fallback); statErr == nil {
			bin = fallback
		} else {
			return m
		}
	}

	m.cmuxBin = bin
	m.enabled = true
	return m
}
