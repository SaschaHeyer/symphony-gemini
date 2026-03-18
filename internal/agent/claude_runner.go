package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/workspace"
)

const claudeSessionIDFile = ".symphony-session-id"

// ClaudeRunner implements AgentLauncher using Claude Code CLI.
type ClaudeRunner struct{}

// NewClaudeRunner creates a new ClaudeRunner.
func NewClaudeRunner() *ClaudeRunner {
	return &ClaudeRunner{}
}

// Launch runs a full Claude Code agent attempt: workspace -> prompt -> CLI spawn -> NDJSON parse -> turn loop.
func (r *ClaudeRunner) Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error {
	logger := slog.With("issue_id", params.Issue.ID, "issue_identifier", params.Issue.Identifier)

	// 1. Create/reuse workspace
	ws, err := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
	if err != nil {
		return fmt.Errorf("workspace creation failed: %w", err)
	}
	logger = logger.With("workspace", ws.Path)

	// 2. Validate workspace path
	if err := workspace.ValidateWorkspacePath(ws.Path, params.WorkspaceRoot); err != nil {
		return fmt.Errorf("workspace safety check failed: %w", err)
	}

	// 3. Run before_run hook
	if err := params.WorkspaceMgr.RunBeforeRun(ws.Path); err != nil {
		return fmt.Errorf("before_run hook failed: %w", err)
	}

	defer params.WorkspaceMgr.RunAfterRun(ws.Path)

	// 4. Read existing session ID (may be empty on first run)
	sessionID := readSessionID(ws.Path)

	// 5. Turn loop
	maxTurns := params.AgentCfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
	}

	for turnNumber := 1; turnNumber <= maxTurns; turnNumber++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Build prompt
		turnPrompt, err := buildTurnPrompt(params.Workflow, params.Issue, params.Attempt, turnNumber, maxTurns)
		if err != nil {
			return fmt.Errorf("prompt rendering failed: %w", err)
		}

		logger.Info("starting claude turn", "turn", turnNumber, "max_turns", maxTurns)
		if params.EventLogWriter != nil {
			logAnnotation(params.EventLogWriter, fmt.Sprintf("Starting turn %d of %d", turnNumber, maxTurns))
		}

		// Build CLI args
		args := buildClaudeArgs(params.ClaudeCfg, turnPrompt, sessionID, ws.Path)

		// Spawn process with PTY
		cmd, stdout, err := spawnWithPTY(params.ClaudeCfg.Command, args, ws.Path, params.ExtraEnv)
		if err != nil {
			return fmt.Errorf("failed to spawn claude process: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start claude process: %w", err)
		}

		// Parse NDJSON & emit events, with turn timeout
		turnResult, newSessionID, collectErr := collectClaudeOutput(
			ctx, cmd, stdout, sessionID,
			params.ClaudeCfg.TurnTimeoutMs, eventCh, params.Issue.ID,
			params.EventLogWriter,
		)

		// Persist session ID if we got one
		if newSessionID != "" && newSessionID != sessionID {
			writeSessionID(ws.Path, newSessionID)
			sessionID = newSessionID
		}

		// Check result
		if collectErr != nil {
			eventCh <- OrchestratorEvent{
				Type:    EventAgentUpdate,
				IssueID: params.Issue.ID,
				Payload: AgentEvent{
					Type:      EventTurnFailed,
					Timestamp: time.Now().UTC(),
					SessionID: sessionID,
					Message:   collectErr.Error(),
				},
			}
			return fmt.Errorf("turn %d failed: %w", turnNumber, collectErr)
		}

		logger.Info("claude turn completed", "turn", turnNumber, "result", turnResult)

		// Emit turn_completed
		eventCh <- OrchestratorEvent{
			Type:    EventAgentUpdate,
			IssueID: params.Issue.ID,
			Payload: AgentEvent{
				Type:      EventTurnCompleted,
				Timestamp: time.Now().UTC(),
				SessionID: sessionID,
				Message:   turnResult,
			},
		}

		// If result indicates an error, stop
		if turnResult == "result/error" {
			break
		}

		// Last turn -- don't check state
		if turnNumber >= maxTurns {
			logger.Info("reached max turns")
			break
		}

		// Re-check issue state before continuing
		if params.CheckIssueState != nil {
			state, err := params.CheckIssueState(ctx, params.Issue.ID)
			if err != nil {
				return fmt.Errorf("issue state check failed: %w", err)
			}
			if !isActiveState(state, params) {
				logger.Info("issue no longer active, ending turn loop", "state", state)
				break
			}
		}
	}

	return nil
}

// buildClaudeArgs constructs the CLI arguments for a Claude Code invocation.
func buildClaudeArgs(cfg *config.ClaudeConfig, prompt string, sessionID string, workspace string) []string {
	var args []string

	// 1. Prompt
	args = append(args, "-p", prompt)

	// 2. Output format (--verbose is required with stream-json when using -p)
	args = append(args, "--output-format", "stream-json", "--verbose")

	// 3. Max turns
	args = append(args, "--max-turns", strconv.Itoa(cfg.MaxTurns))

	// 4. Model
	args = append(args, "--model", cfg.Model)

	// 5. Resume with session ID
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}

	// 6. Permission mode
	if cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", cfg.PermissionMode)
	}

	// 7. Allowed tools
	for _, tool := range cfg.AllowedTools {
		args = append(args, "--allowedTools", tool)
	}

	// 8. MCP config
	mcpPath := filepath.Join(workspace, ".mcp.json")
	if _, err := os.Stat(mcpPath); err == nil {
		args = append(args, "--mcp-config", mcpPath)
	}

	return args
}

// spawnWithPTY wraps execution via `script -q /dev/null` for PTY allocation.
func spawnWithPTY(executable string, args []string, cwd string, extraEnv []string) (*exec.Cmd, io.ReadCloser, error) {
	var cmd *exec.Cmd

	scriptPath, lookErr := exec.LookPath("script")
	if lookErr == nil {
		// Build the full command string with proper escaping
		fullArgs := append([]string{executable}, args...)
		joined := shellJoin(fullArgs)
		cmd = exec.Command(scriptPath, "-q", "/dev/null", "/bin/sh", "-c", joined)
	} else {
		slog.Warn("script command not found on PATH, spawning without PTY")
		cmd = exec.Command(executable, args...)
	}

	cmd.Dir = cwd
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Capture stderr for diagnostic logging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	go drainStderr(stderr)

	return cmd, stdout, nil
}

// collectClaudeOutput reads stdout from the Claude process, parses NDJSON events,
// emits OrchestratorEvents, and returns the last result type and discovered session ID.
func collectClaudeOutput(
	ctx context.Context,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	currentSessionID string,
	turnTimeoutMs int,
	eventCh chan<- OrchestratorEvent,
	issueID string,
	eventLogWriter io.Writer,
) (lastResultType string, sessionID string, err error) {
	sessionID = currentSessionID

	// Create timeout context
	if turnTimeoutMs <= 0 {
		turnTimeoutMs = 600000 // default 10 minutes
	}
	timeoutDuration := time.Duration(turnTimeoutMs) * time.Millisecond
	timeoutCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	parser := NewNdjsonParser()
	reader := bufio.NewReaderSize(stdout, 4096)

	// Channel for read results
	type readResult struct {
		data []byte
		err  error
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])

				events := parser.Feed(chunk)
				for _, evt := range events {
					logEvent(eventLogWriter, formatClaudeEvent(evt))

					agentEvt := mapNdjsonToAgentEvent(evt, sessionID)
					eventCh <- OrchestratorEvent{
						Type:    EventAgentUpdate,
						IssueID: issueID,
						Payload: agentEvt,
					}

					// Extract session ID from system/init
					if evt.Type == "system" && evt.Subtype == "init" {
						if sid, ok := evt.Raw["session_id"].(string); ok && sid != "" {
							sessionID = sid
						}
					}

					// Track last result type
					if evt.Type == "result" {
						if evt.Subtype != "" {
							lastResultType = "result/" + evt.Subtype
						} else {
							lastResultType = "result"
						}
					}
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// Wait for either completion or timeout
	select {
	case <-done:
		// Reading finished, now wait for process
	case <-timeoutCtx.Done():
		// Timeout: kill process
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		return lastResultType, sessionID, fmt.Errorf("claude turn timed out after %v", timeoutDuration)
	}

	// Wait for process to exit
	waitErr := cmd.Wait()

	// Flush remaining parser data
	remaining := parser.Flush()
	for _, evt := range remaining {
		if eventLogWriter != nil {
			fmt.Fprintf(eventLogWriter, "[%s] %s\n", time.Now().Format("15:04:05"), formatClaudeEvent(evt))
		}

		agentEvt := mapNdjsonToAgentEvent(evt, sessionID)
		eventCh <- OrchestratorEvent{
			Type:    EventAgentUpdate,
			IssueID: issueID,
			Payload: agentEvt,
		}

		if evt.Type == "system" && evt.Subtype == "init" {
			if sid, ok := evt.Raw["session_id"].(string); ok && sid != "" {
				sessionID = sid
			}
		}
		if evt.Type == "result" {
			if evt.Subtype != "" {
				lastResultType = "result/" + evt.Subtype
			} else {
				lastResultType = "result"
			}
		}
	}

	// Check process exit code
	if waitErr != nil {
		return lastResultType, sessionID, fmt.Errorf("claude process exited with error: %w", waitErr)
	}

	return lastResultType, sessionID, nil
}

// mapNdjsonToAgentEvent converts an NdjsonEvent into an AgentEvent.
func mapNdjsonToAgentEvent(evt NdjsonEvent, sessionID string) AgentEvent {
	agentEvt := AgentEvent{
		Timestamp: time.Now().UTC(),
		SessionID: sessionID,
	}

	switch {
	case evt.Type == "system" && evt.Subtype == "init":
		agentEvt.Type = EventSessionStarted
		if sid, ok := evt.Raw["session_id"].(string); ok {
			agentEvt.SessionID = sid
		}

	case evt.Type == "result" && evt.Subtype == "success":
		agentEvt.Type = EventTurnCompleted
		agentEvt.Usage = extractUsageFromResult(evt.Raw)
		agentEvt.Message = extractAssistantText(evt.Raw)

	case evt.Type == "result" && evt.Subtype == "error":
		agentEvt.Type = EventTurnFailed
		agentEvt.Usage = extractUsageFromResult(evt.Raw)
		if errMsg, ok := evt.Raw["error"].(string); ok {
			agentEvt.Message = errMsg
		}

	case evt.Type == "result":
		// result/* (other subtypes)
		agentEvt.Type = EventTurnCompleted
		agentEvt.Usage = extractUsageFromResult(evt.Raw)

	case evt.Type == "assistant" && hasToolUse(evt.Raw):
		agentEvt.Type = EventToolCall
		agentEvt.Message = extractToolCallSummary(evt.Raw)

	case evt.Type == "assistant":
		agentEvt.Type = EventNotification
		agentEvt.Message = extractAssistantText(evt.Raw)

	default:
		agentEvt.Type = EventNotification
	}

	return agentEvt
}

// readSessionID reads the stored session ID from the workspace.
func readSessionID(workspace string) string {
	data, err := os.ReadFile(filepath.Join(workspace, claudeSessionIDFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeSessionID persists the session ID to the workspace.
func writeSessionID(workspace string, sessionID string) error {
	return os.WriteFile(
		filepath.Join(workspace, claudeSessionIDFile),
		[]byte(sessionID+"\n"),
		0644,
	)
}

// hasToolUse checks if an assistant message contains tool_use content blocks.
func hasToolUse(raw map[string]any) bool {
	msg, ok := raw["message"].(map[string]any)
	if !ok {
		return false
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return false
	}
	for _, c := range content {
		if block, ok := c.(map[string]any); ok {
			if block["type"] == "tool_use" {
				return true
			}
		}
	}
	return false
}

// shellEscape wraps a string in single quotes with proper escaping.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// shellJoin joins a list of arguments into a shell-safe command string.
func shellJoin(args []string) string {
	escaped := make([]string, len(args))
	for i, a := range args {
		escaped[i] = shellEscape(a)
	}
	return strings.Join(escaped, " ")
}

// extractUsageFromResult extracts TokenUsage from a result event's raw data.
func extractUsageFromResult(raw map[string]any) *TokenUsage {
	if usage, ok := raw["usage"].(map[string]any); ok {
		return extractTokenUsageFromMap(usage)
	}
	return nil
}

// extractAssistantText extracts text content from an assistant message.
func extractAssistantText(raw map[string]any) string {
	msg, ok := raw["message"].(map[string]any)
	if !ok {
		// For result events, try "result" field
		if result, ok := raw["result"].(string); ok {
			return result
		}
		return ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, c := range content {
		if block, ok := c.(map[string]any); ok {
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

// extractToolCallSummary builds a summary string from tool_use content blocks.
func extractToolCallSummary(raw map[string]any) string {
	msg, ok := raw["message"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return ""
	}
	var tools []string
	for _, c := range content {
		if block, ok := c.(map[string]any); ok {
			if block["type"] == "tool_use" {
				name, _ := block["name"].(string)
				if name != "" {
					tools = append(tools, name)
				}
			}
		}
	}
	if len(tools) == 0 {
		return "tool_use"
	}
	return "tool_use: " + strings.Join(tools, ", ")
}

// formatClaudeEvent returns a human-readable one-line summary of a Claude NDJSON event.
func formatClaudeEvent(evt NdjsonEvent) string {
	switch {
	case evt.Type == "system" && evt.Subtype == "init":
		sid, _ := evt.Raw["session_id"].(string)
		return fmt.Sprintf("%sSESSION%s  %s%s%s", cBlue, cReset, cDim, sid, cReset)

	case evt.Type == "assistant" && hasToolUse(evt.Raw):
		return fmt.Sprintf("%sTOOL%s   %s", cYellow, cReset, extractToolCallSummary(evt.Raw))

	case evt.Type == "assistant":
		text := extractAssistantText(evt.Raw)
		return fmt.Sprintf("%sAGENT%s  %s", cCyan, cReset, truncate(text, 150))

	case evt.Type == "result" && evt.Subtype == "success":
		text := extractAssistantText(evt.Raw)
		if text == "" {
			text = "completed"
		}
		return fmt.Sprintf("%sDONE%s   %s", cGreen, cReset, truncate(text, 150))

	case evt.Type == "result" && evt.Subtype == "error":
		errMsg, _ := evt.Raw["error"].(string)
		return fmt.Sprintf("%sERROR%s  %s", cRed, cReset, truncate(errMsg, 150))

	case evt.Type == "result":
		return fmt.Sprintf("%sRESULT%s %s", cGreen, cReset, evt.Subtype)

	default:
		return fmt.Sprintf("%sEVENT%s  %s/%s", cGray, cReset, evt.Type, evt.Subtype)
	}
}
