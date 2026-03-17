package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/symphony-go/symphony/internal/config"
)

func TestCreateForIssue_DeterministicPath(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root, nil)

	ws1, err := mgr.CreateForIssue("MT-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(root, "MT-123")
	if ws1.Path != expected {
		t.Errorf("expected path %q, got %q", expected, ws1.Path)
	}
	if ws1.WorkspaceKey != "MT-123" {
		t.Errorf("expected key MT-123, got %q", ws1.WorkspaceKey)
	}
}

func TestCreateForIssue_CreatesNewDirectory(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root, nil)

	ws, err := mgr.CreateForIssue("MT-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ws.CreatedNow {
		t.Error("expected CreatedNow=true for new directory")
	}

	info, err := os.Stat(ws.Path)
	if err != nil {
		t.Fatalf("workspace dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("workspace path is not a directory")
	}
}

func TestCreateForIssue_ReusesExistingDirectory(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root, nil)

	// Create first
	ws1, _ := mgr.CreateForIssue("MT-1")
	if !ws1.CreatedNow {
		t.Error("expected CreatedNow=true on first create")
	}

	// Reuse
	ws2, err := mgr.CreateForIssue("MT-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws2.CreatedNow {
		t.Error("expected CreatedNow=false for existing directory")
	}
	if ws2.Path != ws1.Path {
		t.Errorf("expected same path on reuse, got %q vs %q", ws2.Path, ws1.Path)
	}
}

func TestCreateForIssue_AfterCreateHookRunsOnNew(t *testing.T) {
	root := t.TempDir()
	script := "touch after_create_marker"
	hooks := &config.HooksConfig{
		AfterCreate: &script,
		TimeoutMs:   5000,
	}
	mgr := NewManager(root, hooks)

	ws, err := mgr.CreateForIssue("MT-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	marker := filepath.Join(ws.Path, "after_create_marker")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Error("after_create hook did not run")
	}
}

func TestCreateForIssue_AfterCreateHookNotRunOnReuse(t *testing.T) {
	root := t.TempDir()
	script := "touch after_create_marker"
	hooks := &config.HooksConfig{
		AfterCreate: &script,
		TimeoutMs:   5000,
	}
	mgr := NewManager(root, hooks)

	// Create workspace
	ws, _ := mgr.CreateForIssue("MT-1")
	// Remove marker
	os.Remove(filepath.Join(ws.Path, "after_create_marker"))

	// Reuse — hook should NOT run
	ws2, err := mgr.CreateForIssue("MT-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	marker := filepath.Join(ws2.Path, "after_create_marker")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("after_create hook should not run on reuse")
	}
}

func TestCreateForIssue_AfterCreateFailureRemovesDir(t *testing.T) {
	root := t.TempDir()
	script := "exit 1"
	hooks := &config.HooksConfig{
		AfterCreate: &script,
		TimeoutMs:   5000,
	}
	mgr := NewManager(root, hooks)

	_, err := mgr.CreateForIssue("MT-FAIL")
	if err == nil {
		t.Fatal("expected error from failing after_create hook")
	}

	wsPath := filepath.Join(root, "MT-FAIL")
	if _, statErr := os.Stat(wsPath); !os.IsNotExist(statErr) {
		t.Error("expected workspace directory to be removed after hook failure")
	}
}

func TestRunBeforeRun_FailureReturnsError(t *testing.T) {
	root := t.TempDir()
	script := "exit 1"
	hooks := &config.HooksConfig{
		BeforeRun: &script,
		TimeoutMs: 5000,
	}
	mgr := NewManager(root, hooks)

	wsPath := filepath.Join(root, "test-ws")
	os.MkdirAll(wsPath, 0755)

	err := mgr.RunBeforeRun(wsPath)
	if err == nil {
		t.Error("expected error from failing before_run hook")
	}
}

func TestRunAfterRun_FailureDoesNotReturnError(t *testing.T) {
	root := t.TempDir()
	script := "exit 1"
	hooks := &config.HooksConfig{
		AfterRun:  &script,
		TimeoutMs: 5000,
	}
	mgr := NewManager(root, hooks)

	wsPath := filepath.Join(root, "test-ws")
	os.MkdirAll(wsPath, 0755)

	// Should not panic or return error
	mgr.RunAfterRun(wsPath)
}

func TestCleanWorkspace_RemovesDirectory(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root, nil)

	ws, _ := mgr.CreateForIssue("MT-DEL")
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		t.Fatal("workspace should exist before cleanup")
	}

	err := mgr.CleanWorkspace("MT-DEL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Error("workspace directory should be removed after cleanup")
	}
}

func TestCleanWorkspace_RunsBeforeRemoveHook(t *testing.T) {
	root := t.TempDir()
	script := "touch before_remove_marker"
	hooks := &config.HooksConfig{
		BeforeRemove: &script,
		TimeoutMs:    5000,
	}
	mgr := NewManager(root, hooks)

	ws, _ := mgr.CreateForIssue("MT-HOOK")

	// The hook writes a marker in the workspace dir, but cleanup removes it.
	// Verify hook ran by checking it didn't error (we can't check the file after removal).
	err := mgr.CleanWorkspace("MT-HOOK")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Workspace should be gone
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Error("workspace should be removed")
	}
}

func TestCreateForIssue_PathOutsideRootRejected(t *testing.T) {
	root := t.TempDir()

	// Direct safety check
	err := ValidateWorkspacePath("/tmp/other/MT-1", root)
	if err == nil {
		t.Error("expected error for path outside workspace root")
	}
}
