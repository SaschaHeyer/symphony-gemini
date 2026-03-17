package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Initialize performs the ACP initialization handshake.
func (c *ACPClient) Initialize(readTimeout time.Duration) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: 1,
		ClientInfo: ClientInfo{
			Name:    "symphony-go",
			Version: "1.0",
		},
		ClientCapabilities: ClientCapabilities{
			FS: &FSCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	}

	id, err := c.sendRequest("initialize", params)
	if err != nil {
		return nil, fmt.Errorf("failed to send initialize: %w", err)
	}

	resp, err := c.readResponseByID(id, readTimeout, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	return &result, nil
}

// SessionNew creates a new ACP session.
func (c *ACPClient) SessionNew(cwd string, readTimeout time.Duration) (string, error) {
	params := SessionNewParams{
		CWD:        cwd,
		MCPServers: []MCPServer{},
	}

	id, err := c.sendRequest("session/new", params)
	if err != nil {
		return "", fmt.Errorf("failed to send session/new: %w", err)
	}

	resp, err := c.readResponseByID(id, readTimeout, nil)
	if err != nil {
		return "", fmt.Errorf("session/new failed: %w", err)
	}

	var result SessionNewResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse session/new result: %w", err)
	}

	if result.SessionID == "" {
		return "", fmt.Errorf("session/new returned empty sessionId")
	}

	// Set YOLO mode (auto-approve all tools)
	c.setMode(result.SessionID, "yolo", readTimeout)

	return result.SessionID, nil
}

// setMode switches the agent to a different operating mode.
func (c *ACPClient) setMode(sessionID string, modeID string, timeout time.Duration) {
	id, err := c.sendRequest("session/set_mode", map[string]string{
		"sessionId": sessionID,
		"modeId":    modeID,
	})
	if err != nil {
		slog.Warn("failed to send session/set_mode", "error", err, "mode", modeID)
		return
	}

	_, err = c.readResponseByID(id, timeout, nil)
	if err != nil {
		slog.Warn("session/set_mode failed", "error", err, "mode", modeID)
		return
	}

	slog.Info("agent mode set", "mode", modeID, "session_id", sessionID)
}

// SessionPrompt sends a prompt and streams updates until the turn completes.
func (c *ACPClient) SessionPrompt(sessionID string, prompt []ContentBlock, turnTimeout time.Duration, handler UpdateHandler) (*SessionPromptResult, error) {
	params := SessionPromptParams{
		SessionID: sessionID,
		Prompt:    prompt,
	}

	id, err := c.sendRequest("session/prompt", params)
	if err != nil {
		return nil, fmt.Errorf("failed to send session/prompt: %w", err)
	}

	resp, err := c.readResponseByID(id, turnTimeout, handler)
	if err != nil {
		return nil, fmt.Errorf("session/prompt failed: %w", err)
	}

	var result SessionPromptResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse session/prompt result: %w", err)
	}

	return &result, nil
}

// SessionCancel sends a cancel notification for a session.
func (c *ACPClient) SessionCancel(sessionID string) error {
	return c.sendNotification("session/cancel", map[string]string{
		"sessionId": sessionID,
	})
}
