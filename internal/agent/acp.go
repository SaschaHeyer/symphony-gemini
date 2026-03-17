package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// terminalEntry tracks a running terminal command.
type terminalEntry struct {
	cmd    *exec.Cmd
	output *bytes.Buffer
	done   bool
	err    error
}

// ACPClient manages communication with a Gemini CLI subprocess via ACP (JSON-RPC 2.0 over stdio).
type ACPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID int
	mu     sync.Mutex

	// Terminal management
	termMu    sync.Mutex
	terminals map[string]*terminalEntry

	// For testing: allow injection of reader/writer without a real process
	noProcess bool
}

// NewACPClient launches a Gemini CLI subprocess in ACP mode and returns a client.
// extraEnv is appended to the current process environment.
func NewACPClient(command string, cwd string, extraEnv []string) (*ACPClient, error) {
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = cwd
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to start ACP subprocess: %w", err)
	}

	// Drain stderr in background (log, never parse as protocol)
	go drainStderr(stderr)

	return &ACPClient{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReaderSize(stdout, 10*1024*1024), // 10MB max line
	}, nil
}

// newTestACPClient creates an ACPClient backed by pipes (no subprocess).
func newTestACPClient(stdinWriter io.WriteCloser, stdoutReader io.Reader) *ACPClient {
	return &ACPClient{
		stdin:     stdinWriter,
		reader:    bufio.NewReaderSize(stdoutReader, 10*1024*1024),
		noProcess: true,
	}
}

// sendRequest writes a JSON-RPC request to the subprocess stdin.
func (c *ACPClient) sendRequest(method string, params any) (int, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return 0, fmt.Errorf("failed to write request: %w", err)
	}

	return id, nil
}

// sendNotification writes a JSON-RPC notification (no ID, no response expected).
func (c *ACPClient) sendNotification(method string, params any) error {
	notif := jsonrpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}

	return nil
}

// sendResponse writes a JSON-RPC response (for agent-initiated requests like permission).
func (c *ACPClient) sendResponse(id int, result any) error {
	resultData, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultData,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}

// readMessage reads and parses the next JSON-RPC message from stdout.
// Returns nil message on timeout.
func (c *ACPClient) readMessage(timeout time.Duration) (*incomingMessage, error) {
	type result struct {
		msg *incomingMessage
		err error
	}

	ch := make(chan result, 1)
	go func() {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			ch <- result{nil, err}
			return
		}

		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 {
			ch <- result{nil, nil}
			return
		}

		var msg incomingMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Warn("malformed JSON-RPC message", "error", err, "line", string(line))
			ch <- result{nil, nil} // skip malformed, don't error
			return
		}

		slog.Debug("acp_recv", "method", msg.Method, "id", msg.ID, "raw", string(line))
		ch <- result{&msg, nil}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case r := <-ch:
		return r.msg, r.err
	case <-timer.C:
		return nil, fmt.Errorf("read timeout after %v", timeout)
	}
}

// readResponseByID reads messages until finding a response with the given ID.
// Handles notifications and agent-initiated requests inline.
func (c *ACPClient) readResponseByID(id int, timeout time.Duration, updateHandler UpdateHandler) (*jsonrpcResponse, error) {
	deadline := time.Now().Add(timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("response timeout waiting for id=%d", id)
		}

		msg, err := c.readMessage(remaining)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue // empty line or malformed, keep reading
		}

		if msg.isResponse() && *msg.ID == id {
			if msg.Error != nil {
				return nil, fmt.Errorf("RPC error (code=%d): %s", msg.Error.Code, msg.Error.Message)
			}
			return &jsonrpcResponse{
				JSONRPC: msg.JSONRPC,
				ID:      *msg.ID,
				Result:  msg.Result,
			}, nil
		}

		// Handle inline: notifications
		if msg.isNotification() {
			c.handleNotification(msg, updateHandler)
			continue
		}

		// Handle inline: agent-initiated requests (e.g., permission)
		if msg.isRequest() {
			c.handleAgentRequest(msg)
			continue
		}
	}
}

func (c *ACPClient) handleNotification(msg *incomingMessage, handler UpdateHandler) {
	if handler == nil {
		return
	}

	if msg.Method == "session/update" {
		var params SessionUpdateParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			slog.Warn("failed to parse session/update", "error", err)
			return
		}
		handler(&params)
		return
	}

	// Handle extNotification for token usage (Gemini sends thread/tokenUsage/updated)
	if msg.Method == "extNotification" || strings.Contains(msg.Method, "tokenUsage") {
		var raw map[string]any
		if err := json.Unmarshal(msg.Params, &raw); err != nil {
			return
		}

		usage := extractTokenUsageFromMap(raw)
		if usage != nil {
			handler(&SessionUpdateParams{
				Update: SessionUpdate{
					SessionUpdate: "token_usage",
				},
				Usage: usage,
			})
		}
		return
	}
}

func (c *ACPClient) handleAgentRequest(msg *incomingMessage) {
	if msg.Method == "session/request_permission" {
		var params RequestPermissionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			slog.Warn("failed to parse permission request", "error", err)
			return
		}

		// Auto-approve: find first "allow" option
		optionID := ""
		for _, opt := range params.Options {
			if strings.Contains(opt.Kind, "allow") {
				optionID = opt.OptionID
				break
			}
		}
		if optionID == "" && len(params.Options) > 0 {
			optionID = params.Options[0].OptionID
		}

		slog.Info("auto-approving permission request",
			"tool_call_id", params.ToolCall.ToolCallID,
			"option_id", optionID,
		)

		c.sendResponse(*msg.ID, RequestPermissionResult{
			Outcome: PermissionOutcome{
				Outcome:  "selected",
				OptionID: optionID,
			},
		})
	} else if msg.Method == "fs/read_text_file" {
		c.handleFSReadTextFile(msg)

	} else if msg.Method == "fs/write_text_file" {
		c.handleFSWriteTextFile(msg)

	} else if msg.Method == "terminal/create" || msg.Method == "terminal/output" ||
		msg.Method == "terminal/wait_for_exit" || msg.Method == "terminal/release" ||
		msg.Method == "terminal/kill" {
		c.handleTerminal(msg)

	} else {
		slog.Warn("unhandled agent request", "method", msg.Method)
		// Respond with error so Gemini doesn't hang
		c.sendResponse(*msg.ID, map[string]any{
			"error": map[string]any{
				"code":    -32601,
				"message": "method not supported: " + msg.Method,
			},
		})
	}
}

func (c *ACPClient) handleFSReadTextFile(msg *incomingMessage) {
	var params struct {
		Path     string `json:"path"`
		FilePath string `json:"filePath"` // fallback alias
		Line     *int   `json:"line"`
		Limit    *int   `json:"limit"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		slog.Warn("failed to parse fs/read_text_file params", "error", err)
		c.sendResponse(*msg.ID, map[string]any{
			"error": "failed to parse params",
		})
		return
	}

	// Use "path" (ACP spec), fall back to "filePath"
	filePath := params.Path
	if filePath == "" {
		filePath = params.FilePath
	}

	slog.Debug("fs/read_text_file", "path", filePath)

	if filePath == "" {
		c.sendResponse(*msg.ID, map[string]any{
			"error": "path is required",
		})
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		slog.Debug("fs/read_text_file failed", "path", filePath, "error", err)
		c.sendResponse(*msg.ID, map[string]any{
			"error": err.Error(),
		})
		return
	}

	// Handle line/limit for partial reads
	result := string(content)
	if params.Line != nil || params.Limit != nil {
		lines := strings.Split(result, "\n")
		start := 0
		if params.Line != nil && *params.Line > 0 {
			start = *params.Line - 1 // 1-based to 0-based
		}
		if start >= len(lines) {
			result = ""
		} else {
			end := len(lines)
			if params.Limit != nil && *params.Limit > 0 {
				end = start + *params.Limit
				if end > len(lines) {
					end = len(lines)
				}
			}
			result = strings.Join(lines[start:end], "\n")
		}
	}

	c.sendResponse(*msg.ID, map[string]any{
		"content": result,
	})
}

func (c *ACPClient) handleFSWriteTextFile(msg *incomingMessage) {
	var params struct {
		Path     string `json:"path"`
		FilePath string `json:"filePath"` // fallback alias
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		slog.Warn("failed to parse fs/write_text_file params", "error", err)
		c.sendResponse(*msg.ID, map[string]any{
			"error": "failed to parse params",
		})
		return
	}

	filePath := params.Path
	if filePath == "" {
		filePath = params.FilePath
	}

	slog.Debug("fs/write_text_file", "path", filePath)

	if filePath == "" {
		c.sendResponse(*msg.ID, map[string]any{
			"error": "path is required",
		})
		return
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.sendResponse(*msg.ID, map[string]any{
			"error": err.Error(),
		})
		return
	}

	if err := os.WriteFile(filePath, []byte(params.Content), 0644); err != nil {
		c.sendResponse(*msg.ID, map[string]any{
			"error": err.Error(),
		})
		return
	}

	// ACP spec: successful write returns null result
	c.sendResponse(*msg.ID, nil)
}

// handleTerminal handles terminal/* requests by running commands.
func (c *ACPClient) handleTerminal(msg *incomingMessage) {
	switch msg.Method {
	case "terminal/create":
		var params struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Cwd     string   `json:"cwd"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			c.sendResponse(*msg.ID, map[string]any{"error": "invalid params"})
			return
		}

		// Generate a terminal ID
		c.mu.Lock()
		c.nextID++
		termID := fmt.Sprintf("term_%d", c.nextID)
		c.mu.Unlock()

		// Store terminal info for later
		c.termMu.Lock()
		if c.terminals == nil {
			c.terminals = make(map[string]*terminalEntry)
		}

		args := params.Args
		cmdStr := params.Command
		if len(args) > 0 {
			cmdStr = params.Command
		}

		cmd := exec.CommandContext(context.Background(), "bash", "-lc", cmdStr+" "+strings.Join(args, " "))
		if params.Cwd != "" {
			cmd.Dir = params.Cwd
		}

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf

		entry := &terminalEntry{cmd: cmd, output: &outBuf}
		c.terminals[termID] = entry
		c.termMu.Unlock()

		slog.Debug("terminal/create", "id", termID, "command", cmdStr)

		// Start the command
		if err := cmd.Start(); err != nil {
			c.sendResponse(*msg.ID, map[string]any{"error": err.Error()})
			return
		}

		// Wait in background
		go func() {
			entry.err = cmd.Wait()
			entry.done = true
		}()

		c.sendResponse(*msg.ID, map[string]any{
			"terminalId": termID,
		})

	case "terminal/output":
		var params struct {
			TerminalID string `json:"terminalId"`
		}
		json.Unmarshal(msg.Params, &params)

		c.termMu.Lock()
		entry, ok := c.terminals[params.TerminalID]
		c.termMu.Unlock()

		if !ok {
			c.sendResponse(*msg.ID, map[string]any{"error": "unknown terminal"})
			return
		}

		c.sendResponse(*msg.ID, map[string]any{
			"output":   entry.output.String(),
			"finished": entry.done,
		})

	case "terminal/wait_for_exit":
		var params struct {
			TerminalID string `json:"terminalId"`
		}
		json.Unmarshal(msg.Params, &params)

		c.termMu.Lock()
		entry, ok := c.terminals[params.TerminalID]
		c.termMu.Unlock()

		if !ok {
			c.sendResponse(*msg.ID, map[string]any{"error": "unknown terminal"})
			return
		}

		// Wait for completion (with timeout)
		for i := 0; i < 600; i++ { // max 60s
			if entry.done {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		exitCode := 0
		if entry.err != nil {
			exitCode = 1
		}

		c.sendResponse(*msg.ID, map[string]any{
			"output":   entry.output.String(),
			"exitCode": exitCode,
		})

	case "terminal/release":
		var params struct {
			TerminalID string `json:"terminalId"`
		}
		json.Unmarshal(msg.Params, &params)

		c.termMu.Lock()
		delete(c.terminals, params.TerminalID)
		c.termMu.Unlock()

		c.sendResponse(*msg.ID, map[string]any{"success": true})

	case "terminal/kill":
		var params struct {
			TerminalID string `json:"terminalId"`
		}
		json.Unmarshal(msg.Params, &params)

		c.termMu.Lock()
		entry, ok := c.terminals[params.TerminalID]
		c.termMu.Unlock()

		if ok && entry.cmd.Process != nil {
			entry.cmd.Process.Kill()
		}
		c.sendResponse(*msg.ID, map[string]any{"success": true})
	}
}

// Close terminates the ACP subprocess.
func (c *ACPClient) Close() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	return nil
}

// extractTokenUsageFromMap tries to find token counts in a map payload.
// Gemini sends promptTokenCount/candidatesTokenCount; we also check common aliases.
func extractTokenUsageFromMap(m map[string]any) *TokenUsage {
	usage := &TokenUsage{}
	found := false

	// Gemini-style: promptTokenCount, candidatesTokenCount
	if v, ok := toInt64(m["promptTokenCount"]); ok {
		usage.InputTokens = v
		found = true
	}
	if v, ok := toInt64(m["candidatesTokenCount"]); ok {
		usage.OutputTokens = v
		found = true
	}

	// Common aliases
	if v, ok := toInt64(m["input_tokens"]); ok {
		usage.InputTokens = v
		found = true
	}
	if v, ok := toInt64(m["output_tokens"]); ok {
		usage.OutputTokens = v
		found = true
	}
	if v, ok := toInt64(m["total_tokens"]); ok {
		usage.TotalTokens = v
		found = true
	}
	if v, ok := toInt64(m["totalTokenCount"]); ok {
		usage.TotalTokens = v
		found = true
	}

	// Check nested "usage" or "data" objects
	for _, key := range []string{"usage", "data", "params"} {
		if nested, ok := m[key].(map[string]any); ok {
			if inner := extractTokenUsageFromMap(nested); inner != nil {
				return inner
			}
		}
	}

	if !found {
		return nil
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	return usage
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	default:
		return 0, false
	}
}

func drainStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		slog.Debug("agent stderr", "line", scanner.Text())
	}
}

// UpdateHandler is called for each session/update notification during a turn.
type UpdateHandler func(update *SessionUpdateParams)
