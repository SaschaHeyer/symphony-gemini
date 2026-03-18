package cmux

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteEvent(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")

	// Create a temp log file and register it
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, agentLogFile)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	m.logFiles["issue-1"] = f

	m.WriteEvent("issue-1", `{"type":"assistant","text":"hello"}`)
	f.Close()

	data, _ := os.ReadFile(logPath)
	content := string(data)

	if !strings.Contains(content, `{"type":"assistant","text":"hello"}`) {
		t.Errorf("expected event in log, got: %s", content)
	}
}

func TestWriteAnnotation(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, agentLogFile)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	m.logFiles["issue-1"] = f

	m.WriteAnnotation("issue-1", "Turn 1 completed")
	f.Close()

	data, _ := os.ReadFile(logPath)
	content := string(data)

	if !strings.Contains(content, "Turn 1 completed") {
		t.Errorf("expected annotation message, got: %s", content)
	}
}

func TestWriteEventNoFile(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")
	// No log file registered — should not panic
	m.WriteEvent("nonexistent", "test")
	m.WriteAnnotation("nonexistent", "test")
}

func TestWriteEventConcurrent(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, agentLogFile)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	m.logFiles["issue-1"] = f

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.WriteEvent("issue-1", "event from goroutine")
			}
		}(i)
	}
	wg.Wait()
	f.Close()

	data, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1000 {
		t.Errorf("expected 1000 lines, got %d", len(lines))
	}
}

func TestLogWriter(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, agentLogFile)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	m.logFiles["issue-1"] = f

	w := m.LogWriter("issue-1")
	if w == io.Discard {
		t.Fatal("expected non-discard writer")
	}

	w.Write([]byte("test line\n"))
	f.Close()

	data, _ := os.ReadFile(logPath)
	content := string(data)

	if !strings.Contains(content, "test line") {
		t.Errorf("expected content from LogWriter, got: %s", content)
	}
}

func TestLogWriterNoFile(t *testing.T) {
	m, _ := newTestManager(t, "mock_cmux.sh")
	w := m.LogWriter("nonexistent")
	if w != io.Discard {
		t.Error("expected io.Discard for unknown issueID")
	}
}
