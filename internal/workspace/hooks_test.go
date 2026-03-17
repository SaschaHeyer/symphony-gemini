package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunHook_Success(t *testing.T) {
	dir := t.TempDir()
	err := RunHook("test", "echo hello", dir, 5000)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunHook_Failure(t *testing.T) {
	dir := t.TempDir()
	err := RunHook("test", "exit 1", dir, 5000)
	if err == nil {
		t.Error("expected error for failing hook")
	}
}

func TestRunHook_Timeout(t *testing.T) {
	dir := t.TempDir()
	err := RunHook("test", "sleep 10", dir, 100) // 100ms timeout
	if err == nil {
		t.Error("expected error for timed out hook")
	}
}

func TestRunHook_CorrectCwd(t *testing.T) {
	dir := t.TempDir()
	markerFile := filepath.Join(dir, "hook_ran_here")
	err := RunHook("test", "touch hook_ran_here", dir, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Error("hook did not run in the correct working directory")
	}
}

func TestRunHook_EmptyScript(t *testing.T) {
	dir := t.TempDir()
	err := RunHook("test", "", dir, 5000)
	if err != nil {
		t.Errorf("expected nil for empty script, got %v", err)
	}
}
