package cmux

import (
	"fmt"
	"io"
	"time"
)

// timePrefix returns a dim gray "[HH:MM:SS]" string.
func timePrefix() string {
	return fmt.Sprintf("%s%s%s", ColorGray, time.Now().Format("15:04:05"), ColorReset)
}

// WriteEvent writes a timestamped event line to the issue's log file.
func (m *Manager) WriteEvent(issueID string, content string) {
	if !m.enabled {
		return
	}
	m.mu.Lock()
	f := m.logFiles[issueID]
	m.mu.Unlock()

	if f == nil {
		return
	}
	fmt.Fprintf(f, " %s  %s\n", timePrefix(), content)
}

// WriteAnnotation writes a highlighted Symphony annotation to the log file.
func (m *Manager) WriteAnnotation(issueID string, message string) {
	if !m.enabled {
		return
	}
	m.mu.Lock()
	f := m.logFiles[issueID]
	m.mu.Unlock()

	if f == nil {
		return
	}
	fmt.Fprintf(f, " %s  %s%s── %s ──%s\n", timePrefix(), ColorBold, ColorBlue, message, ColorReset)
}

// LogWriter returns an io.Writer for the issue's log file.
// Returns io.Discard if cmux is disabled or no log file exists.
// Callers are responsible for formatting and timestamps.
func (m *Manager) LogWriter(issueID string) io.Writer {
	if !m.enabled {
		return io.Discard
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.logFiles[issueID]; ok {
		return f
	}
	return io.Discard
}
