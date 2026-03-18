package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/symphony-go/symphony/internal/config"
)

func testClaudeConfig() *config.ClaudeConfig {
	return &config.ClaudeConfig{
		Command:        "claude",
		Model:          "claude-sonnet-4-6",
		PermissionMode: "bypassPermissions",
		AllowedTools:   []string{"Read", "Write", "Edit", "Bash"},
		MaxTurns:       25,
		TurnTimeoutMs:  600000,
	}
}

// --- buildClaudeArgs tests ---

func TestBuildClaudeArgs_FirstTurn(t *testing.T) {
	cfg := testClaudeConfig()
	args := buildClaudeArgs(cfg, "Fix the bug", "", "/tmp/workspace")

	// Should NOT have --resume
	for i, a := range args {
		if a == "--resume" {
			t.Errorf("expected no --resume for empty sessionID, but found at index %d", i)
		}
	}

	// Should have -p, prompt
	found := false
	for i, a := range args {
		if a == "-p" && i+1 < len(args) && args[i+1] == "Fix the bug" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -p 'Fix the bug' in args: %v", args)
	}

	// Should have --output-format stream-json
	found = false
	for i, a := range args {
		if a == "--output-format" && i+1 < len(args) && args[i+1] == "stream-json" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --output-format stream-json in args: %v", args)
	}

	// Should have --model
	found = false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "claude-sonnet-4-6" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --model claude-sonnet-4-6 in args: %v", args)
	}
}

func TestBuildClaudeArgs_WithResume(t *testing.T) {
	cfg := testClaudeConfig()
	args := buildClaudeArgs(cfg, "Continue", "session-abc-123", "/tmp/workspace")

	found := false
	for i, a := range args {
		if a == "--resume" && i+1 < len(args) && args[i+1] == "session-abc-123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --resume session-abc-123 in args: %v", args)
	}
}

func TestBuildClaudeArgs_AllowedTools(t *testing.T) {
	cfg := testClaudeConfig()
	cfg.AllowedTools = []string{"Read", "Write", "Bash"}
	args := buildClaudeArgs(cfg, "prompt", "", "/tmp/workspace")

	toolCount := 0
	for i, a := range args {
		if a == "--allowedTools" && i+1 < len(args) {
			toolCount++
		}
	}
	if toolCount != 3 {
		t.Errorf("expected 3 --allowedTools entries, got %d in args: %v", toolCount, args)
	}

	// Verify each tool is present
	for _, tool := range []string{"Read", "Write", "Bash"} {
		found := false
		for i, a := range args {
			if a == "--allowedTools" && i+1 < len(args) && args[i+1] == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected --allowedTools %s in args: %v", tool, args)
		}
	}
}

func TestBuildClaudeArgs_WithMcpConfig(t *testing.T) {
	ws := t.TempDir()
	mcpFile := filepath.Join(ws, ".mcp.json")
	os.WriteFile(mcpFile, []byte(`{}`), 0644)

	cfg := testClaudeConfig()
	args := buildClaudeArgs(cfg, "prompt", "", ws)

	found := false
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) && args[i+1] == mcpFile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --mcp-config %s in args: %v", mcpFile, args)
	}
}

func TestBuildClaudeArgs_NoMcpConfig(t *testing.T) {
	ws := t.TempDir()
	// Do NOT create .mcp.json

	cfg := testClaudeConfig()
	args := buildClaudeArgs(cfg, "prompt", "", ws)

	for _, a := range args {
		if a == "--mcp-config" {
			t.Errorf("expected no --mcp-config when .mcp.json absent, but found in args: %v", args)
		}
	}
}

func TestBuildClaudeArgs_PermissionMode(t *testing.T) {
	cfg := testClaudeConfig()
	cfg.PermissionMode = "plan"
	args := buildClaudeArgs(cfg, "prompt", "", "/tmp/workspace")

	found := false
	for i, a := range args {
		if a == "--permission-mode" && i+1 < len(args) && args[i+1] == "plan" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --permission-mode plan in args: %v", args)
	}

	// Test empty permission mode
	cfg.PermissionMode = ""
	args = buildClaudeArgs(cfg, "prompt", "", "/tmp/workspace")
	for _, a := range args {
		if a == "--permission-mode" {
			t.Errorf("expected no --permission-mode when empty, but found in args: %v", args)
		}
	}
}

// --- session persistence tests ---

func TestReadSessionID_Exists(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, claudeSessionIDFile), []byte("  session-xyz-456  \n"), 0644)

	sid := readSessionID(ws)
	if sid != "session-xyz-456" {
		t.Errorf("expected trimmed session ID 'session-xyz-456', got %q", sid)
	}
}

func TestReadSessionID_Missing(t *testing.T) {
	ws := t.TempDir()
	sid := readSessionID(ws)
	if sid != "" {
		t.Errorf("expected empty string for missing file, got %q", sid)
	}
}

func TestWriteSessionID_RoundTrip(t *testing.T) {
	ws := t.TempDir()

	err := writeSessionID(ws, "session-round-trip")
	if err != nil {
		t.Fatalf("writeSessionID failed: %v", err)
	}

	sid := readSessionID(ws)
	if sid != "session-round-trip" {
		t.Errorf("expected 'session-round-trip', got %q", sid)
	}
}

// --- mapNdjsonToAgentEvent tests ---

func TestMapNdjsonToAgentEvent_SystemInit(t *testing.T) {
	evt := NdjsonEvent{
		Type:    "system",
		Subtype: "init",
		Raw: map[string]any{
			"type":       "system",
			"subtype":    "init",
			"session_id": "init-session-1",
		},
	}

	agentEvt := mapNdjsonToAgentEvent(evt, "")
	if agentEvt.Type != EventSessionStarted {
		t.Errorf("expected %s, got %s", EventSessionStarted, agentEvt.Type)
	}
	if agentEvt.SessionID != "init-session-1" {
		t.Errorf("expected session ID 'init-session-1', got %q", agentEvt.SessionID)
	}
}

func TestMapNdjsonToAgentEvent_ResultSuccess(t *testing.T) {
	evt := NdjsonEvent{
		Type:    "result",
		Subtype: "success",
		Raw: map[string]any{
			"type":    "result",
			"subtype": "success",
			"usage": map[string]any{
				"input_tokens":  float64(100),
				"output_tokens": float64(50),
			},
		},
	}

	agentEvt := mapNdjsonToAgentEvent(evt, "sid-1")
	if agentEvt.Type != EventTurnCompleted {
		t.Errorf("expected %s, got %s", EventTurnCompleted, agentEvt.Type)
	}
	if agentEvt.Usage == nil {
		t.Fatal("expected non-nil Usage")
	}
	if agentEvt.Usage.InputTokens != 100 {
		t.Errorf("expected input_tokens=100, got %d", agentEvt.Usage.InputTokens)
	}
	if agentEvt.Usage.OutputTokens != 50 {
		t.Errorf("expected output_tokens=50, got %d", agentEvt.Usage.OutputTokens)
	}
}

func TestMapNdjsonToAgentEvent_ResultError(t *testing.T) {
	evt := NdjsonEvent{
		Type:    "result",
		Subtype: "error",
		Raw: map[string]any{
			"type":    "result",
			"subtype": "error",
			"error":   "something went wrong",
		},
	}

	agentEvt := mapNdjsonToAgentEvent(evt, "sid-1")
	if agentEvt.Type != EventTurnFailed {
		t.Errorf("expected %s, got %s", EventTurnFailed, agentEvt.Type)
	}
	if agentEvt.Message != "something went wrong" {
		t.Errorf("expected error message, got %q", agentEvt.Message)
	}
}

func TestMapNdjsonToAgentEvent_ToolUse(t *testing.T) {
	evt := NdjsonEvent{
		Type: "assistant",
		Raw: map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "Let me edit the file"},
					map[string]any{"type": "tool_use", "name": "Edit", "id": "tool-1"},
				},
			},
		},
	}

	agentEvt := mapNdjsonToAgentEvent(evt, "sid-1")
	if agentEvt.Type != EventToolCall {
		t.Errorf("expected %s, got %s", EventToolCall, agentEvt.Type)
	}
	if !strings.Contains(agentEvt.Message, "Edit") {
		t.Errorf("expected tool name in message, got %q", agentEvt.Message)
	}
}

func TestMapNdjsonToAgentEvent_AssistantText(t *testing.T) {
	evt := NdjsonEvent{
		Type: "assistant",
		Raw: map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "Working on the task..."},
				},
			},
		},
	}

	agentEvt := mapNdjsonToAgentEvent(evt, "sid-1")
	if agentEvt.Type != EventNotification {
		t.Errorf("expected %s, got %s", EventNotification, agentEvt.Type)
	}
	if agentEvt.Message != "Working on the task..." {
		t.Errorf("expected assistant text, got %q", agentEvt.Message)
	}
}

// --- hasToolUse tests ---

func TestHasToolUse_True(t *testing.T) {
	raw := map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "tool_use", "name": "Bash"},
			},
		},
	}
	if !hasToolUse(raw) {
		t.Error("expected hasToolUse to return true")
	}
}

func TestHasToolUse_False(t *testing.T) {
	raw := map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "hello"},
			},
		},
	}
	if hasToolUse(raw) {
		t.Error("expected hasToolUse to return false")
	}
}

// --- shellEscape tests ---

func TestShellEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"it's a test", "'it'\"'\"'s a test'"},
		{"no special", "'no special'"},
		{"a'b'c", "'a'\"'\"'b'\"'\"'c'"},
	}

	for _, tc := range tests {
		got := shellEscape(tc.input)
		if got != tc.expected {
			t.Errorf("shellEscape(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}

	// Test shellJoin
	joined := shellJoin([]string{"echo", "hello world", "it's"})
	if !strings.Contains(joined, "'echo'") {
		t.Errorf("expected escaped echo in joined: %s", joined)
	}
}

// --- collectClaudeOutput mock process test ---

func TestCollectClaudeOutput_MockProcess(t *testing.T) {
	// Create a temp shell script that outputs canned NDJSON
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "mock_claude.sh")
	scriptContent := `#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"test-session-123"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Working on it..."}]}}'
echo '{"type":"result","subtype":"success","usage":{"input_tokens":100,"output_tokens":50},"session_id":"test-session-123"}'
`
	os.WriteFile(scriptPath, []byte(scriptContent), 0755)

	// Build cmd directly (no PTY needed for test)
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Dir = tmpDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start mock process: %v", err)
	}

	eventCh := make(chan OrchestratorEvent, 100)
	ctx := context.Background()

	resultType, sessionID, collectErr := collectClaudeOutput(
		ctx, cmd, stdout, "", 30000, eventCh, "issue-1", nil,
	)

	if collectErr != nil {
		t.Fatalf("collectClaudeOutput returned error: %v", collectErr)
	}

	if sessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %q", sessionID)
	}

	if resultType != "result/success" {
		t.Errorf("expected result type 'result/success', got %q", resultType)
	}

	// Drain events
	close(eventCh)
	var events []AgentEvent
	for oe := range eventCh {
		if ae, ok := oe.Payload.(AgentEvent); ok {
			events = append(events, ae)
		}
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// First event should be session_started
	if events[0].Type != EventSessionStarted {
		t.Errorf("expected first event to be %s, got %s", EventSessionStarted, events[0].Type)
	}

	// Find the notification event
	foundNotification := false
	for _, e := range events {
		if e.Type == EventNotification {
			foundNotification = true
			break
		}
	}
	if !foundNotification {
		t.Error("expected at least one notification event")
	}

	// Find the turn_completed event
	foundCompleted := false
	for _, e := range events {
		if e.Type == EventTurnCompleted {
			foundCompleted = true
			if e.Usage != nil {
				if e.Usage.InputTokens != 100 {
					t.Errorf("expected input_tokens=100, got %d", e.Usage.InputTokens)
				}
			}
			break
		}
	}
	if !foundCompleted {
		t.Error("expected a turn_completed event")
	}
}

func TestClaudeRunner_MockProcess(t *testing.T) {
	// Create a temp shell script that outputs canned NDJSON
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "mock_claude.sh")
	scriptContent := `#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"runner-session-1"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Done!"}]}}'
echo '{"type":"result","subtype":"success","usage":{"input_tokens":200,"output_tokens":100},"session_id":"runner-session-1"}'
`
	os.WriteFile(scriptPath, []byte(scriptContent), 0755)

	root := t.TempDir()

	claudeCfg := &config.ClaudeConfig{
		Command:        scriptPath,
		Model:          "claude-sonnet-4-6",
		PermissionMode: "bypassPermissions",
		AllowedTools:   []string{},
		MaxTurns:       1,
		TurnTimeoutMs:  30000,
	}

	// Verify buildClaudeArgs works with this config
	args := buildClaudeArgs(claudeCfg, "Fix MT-RUNNER-1", "", root)
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}

	// Verify the script can be executed and output collected
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Dir = tmpDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	eventCh := make(chan OrchestratorEvent, 100)
	resultType, sessionID, collectErr := collectClaudeOutput(
		context.Background(), cmd, stdout, "", 30000, eventCh, "issue-runner-1", nil,
	)

	if collectErr != nil {
		t.Fatalf("collect error: %v", collectErr)
	}
	if sessionID != "runner-session-1" {
		t.Errorf("expected session 'runner-session-1', got %q", sessionID)
	}
	if resultType != "result/success" {
		t.Errorf("expected 'result/success', got %q", resultType)
	}

	close(eventCh)
	count := 0
	for range eventCh {
		count++
	}
	if count < 3 {
		t.Errorf("expected at least 3 events, got %d", count)
	}
}

// Test that extractUsageFromResult works
func TestExtractUsageFromResult(t *testing.T) {
	raw := map[string]any{
		"usage": map[string]any{
			"input_tokens":  float64(500),
			"output_tokens": float64(250),
		},
	}
	usage := extractUsageFromResult(raw)
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.InputTokens != 500 {
		t.Errorf("expected 500, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 250 {
		t.Errorf("expected 250, got %d", usage.OutputTokens)
	}

	// No usage
	raw2 := map[string]any{"result": "ok"}
	usage2 := extractUsageFromResult(raw2)
	if usage2 != nil {
		t.Error("expected nil usage when no usage field")
	}
}

func TestExtractAssistantText(t *testing.T) {
	raw := map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "Hello"},
				map[string]any{"type": "text", "text": "World"},
			},
		},
	}
	text := extractAssistantText(raw)
	if text != "Hello\nWorld" {
		t.Errorf("expected 'Hello\\nWorld', got %q", text)
	}
}

func TestExtractToolCallSummary(t *testing.T) {
	raw := map[string]any{
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "tool_use", "name": "Read"},
				map[string]any{"type": "tool_use", "name": "Edit"},
			},
		},
	}
	summary := extractToolCallSummary(raw)
	if summary != "tool_use: Read, Edit" {
		t.Errorf("expected 'tool_use: Read, Edit', got %q", summary)
	}
}

// Verify timeout behavior
func TestCollectClaudeOutput_Timeout(t *testing.T) {
	// Create a script that hangs
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "hang_claude.sh")
	scriptContent := `#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"hang-session"}'
sleep 60
`
	os.WriteFile(scriptPath, []byte(scriptContent), 0755)

	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Dir = tmpDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	eventCh := make(chan OrchestratorEvent, 100)
	ctx := context.Background()

	start := time.Now()
	_, _, collectErr := collectClaudeOutput(
		ctx, cmd, stdout, "", 1000, eventCh, "timeout-issue", nil, // 1 second timeout
	)
	elapsed := time.Since(start)

	if collectErr == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(collectErr.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", collectErr)
	}
	if elapsed > 10*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}
