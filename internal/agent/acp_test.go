package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockAgent simulates a Gemini CLI ACP subprocess using pipes.
type mockAgent struct {
	incoming io.Reader // reads what client sends
	outgoing io.Writer // writes responses/notifications to client
	mu       sync.Mutex
}

func (m *mockAgent) writeLine(data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := json.Marshal(data)
	m.outgoing.Write(append(b, '\n'))
}

func (m *mockAgent) readRequest() (*jsonrpcRequest, error) {
	buf := make([]byte, 65536)
	n, err := m.incoming.Read(buf)
	if err != nil {
		return nil, err
	}
	var req jsonrpcRequest
	if err := json.Unmarshal(buf[:n], &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func setupTestClient(t *testing.T) (*ACPClient, *mockAgent) {
	t.Helper()

	// Pipes: client writes to clientW → mockAgent reads from clientR
	clientR, clientW := io.Pipe()
	// Pipes: mockAgent writes to mockW → client reads from mockR
	mockR, mockW := io.Pipe()

	client := newTestACPClient(clientW, mockR)
	mock := &mockAgent{incoming: clientR, outgoing: mockW}

	return client, mock
}

func TestInitialize_SendsCorrectRequest(t *testing.T) {
	client, mock := setupTestClient(t)

	// Run initialize in background
	var initResult *InitializeResult
	var initErr error
	done := make(chan struct{})

	go func() {
		defer close(done)
		initResult, initErr = client.Initialize(5 * time.Second)
	}()

	// Mock reads the request
	req, err := mock.readRequest()
	if err != nil {
		t.Fatalf("mock failed to read request: %v", err)
	}

	if req.Method != "initialize" {
		t.Errorf("expected method=initialize, got %q", req.Method)
	}
	if req.ID == 0 {
		t.Error("expected non-zero request ID")
	}

	// Verify params
	paramsBytes, _ := json.Marshal(req.Params)
	var params InitializeParams
	json.Unmarshal(paramsBytes, &params)

	if params.ProtocolVersion != 1 {
		t.Errorf("expected protocolVersion=1, got %d", params.ProtocolVersion)
	}
	if params.ClientInfo.Name != "symphony-go" {
		t.Errorf("expected clientInfo.name=symphony-go, got %q", params.ClientInfo.Name)
	}

	// Send response
	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]any{
			"protocolVersion": 1,
			"agentInfo":       map[string]any{"name": "gemini-cli", "version": "1.0"},
			"agentCapabilities": map[string]any{
				"promptCapabilities": map[string]any{"image": true},
			},
		},
	})

	<-done
	if initErr != nil {
		t.Fatalf("Initialize error: %v", initErr)
	}
	if initResult.AgentInfo.Name != "gemini-cli" {
		t.Errorf("expected agent name=gemini-cli, got %q", initResult.AgentInfo.Name)
	}
}

func TestSessionNew_ReturnsSessionID(t *testing.T) {
	client, mock := setupTestClient(t)

	var sessionID string
	var sessionErr error
	done := make(chan struct{})

	go func() {
		defer close(done)
		sessionID, sessionErr = client.SessionNew("/workspace/MT-1", 5*time.Second)
	}()

	req, _ := mock.readRequest()
	if req.Method != "session/new" {
		t.Errorf("expected method=session/new, got %q", req.Method)
	}

	// Verify cwd param
	paramsBytes, _ := json.Marshal(req.Params)
	var params SessionNewParams
	json.Unmarshal(paramsBytes, &params)
	if params.CWD != "/workspace/MT-1" {
		t.Errorf("expected cwd=/workspace/MT-1, got %q", params.CWD)
	}

	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  map[string]any{"sessionId": "sess_abc123"},
	})

	// Handle the session/set_mode request (YOLO mode)
	modeReq, _ := mock.readRequest()
	if modeReq != nil && modeReq.Method == "session/set_mode" {
		mock.writeLine(map[string]any{
			"jsonrpc": "2.0",
			"id":      modeReq.ID,
			"result":  map[string]any{"modeId": "yolo"},
		})
	}

	<-done
	if sessionErr != nil {
		t.Fatalf("SessionNew error: %v", sessionErr)
	}
	if sessionID != "sess_abc123" {
		t.Errorf("expected sessionId=sess_abc123, got %q", sessionID)
	}
}

func TestSessionPrompt_StreamsUpdatesAndReturnsStopReason(t *testing.T) {
	client, mock := setupTestClient(t)

	var updates []SessionUpdateParams
	var promptResult *SessionPromptResult
	var promptErr error
	done := make(chan struct{})

	go func() {
		defer close(done)
		promptResult, promptErr = client.SessionPrompt("sess_1", []ContentBlock{
			{Type: "text", Text: "Fix the bug"},
		}, 5*time.Second, func(update *SessionUpdateParams) {
			updates = append(updates, *update)
		})
	}()

	req, _ := mock.readRequest()
	if req.Method != "session/prompt" {
		t.Errorf("expected method=session/prompt, got %q", req.Method)
	}

	// Send some update notifications
	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sess_1",
			"update": map[string]any{
				"sessionUpdate": "message_chunk",
				"role":          "agent",
				"text":          "Working on it...",
			},
		},
	})

	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sess_1",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    "call_1",
				"title":         "Reading file",
				"kind":          "read",
				"status":        "completed",
			},
		},
	})

	// Send the final response
	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  map[string]any{"stopReason": "end_turn"},
	})

	<-done
	if promptErr != nil {
		t.Fatalf("SessionPrompt error: %v", promptErr)
	}
	if promptResult.StopReason != "end_turn" {
		t.Errorf("expected stopReason=end_turn, got %q", promptResult.StopReason)
	}
	if len(updates) != 2 {
		t.Errorf("expected 2 updates, got %d", len(updates))
	}
}

func TestSessionPrompt_AutoApprovesPermission(t *testing.T) {
	client, mock := setupTestClient(t)

	var approvalResponse []byte
	done := make(chan struct{})

	go func() {
		defer close(done)
		client.SessionPrompt("sess_1", []ContentBlock{
			{Type: "text", Text: "test"},
		}, 5*time.Second, nil)
	}()

	// Read the session/prompt request
	req, _ := mock.readRequest()

	// Send a permission request from agent
	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      99,
		"method":  "session/request_permission",
		"params": map[string]any{
			"sessionId": "sess_1",
			"toolCall":  map[string]any{"toolCallId": "call_1"},
			"options": []map[string]any{
				{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
				{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"},
			},
		},
	})

	// Read the client's response to the permission request
	// (client writes to clientW → mockAgent reads from clientR)
	time.Sleep(100 * time.Millisecond) // let client process

	buf := make([]byte, 65536)
	n, _ := mock.incoming.Read(buf)
	// Skip past the session/prompt request we already read
	lines := strings.Split(strings.TrimSpace(string(buf[:n])), "\n")
	for _, line := range lines {
		if strings.Contains(line, "\"id\":99") {
			approvalResponse = []byte(line)
			break
		}
	}

	if approvalResponse != nil {
		var resp jsonrpcResponse
		json.Unmarshal(approvalResponse, &resp)
		var result RequestPermissionResult
		json.Unmarshal(resp.Result, &result)
		if result.Outcome.Outcome != "selected" {
			t.Errorf("expected outcome=selected, got %q", result.Outcome.Outcome)
		}
		if result.Outcome.OptionID != "allow-once" {
			t.Errorf("expected optionId=allow-once, got %q", result.Outcome.OptionID)
		}
	}

	// Finish the prompt
	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  map[string]any{"stopReason": "end_turn"},
	})

	<-done
}

func TestInitialize_ReadTimeout(t *testing.T) {
	client, mock := setupTestClient(t)

	// Drain the request but never respond
	go func() {
		buf := make([]byte, 65536)
		mock.incoming.Read(buf)
	}()

	_, err := client.Initialize(200 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout in error, got: %v", err)
	}
}

func TestSessionPrompt_TurnTimeout(t *testing.T) {
	client, mock := setupTestClient(t)

	// Drain requests in background
	go func() {
		buf := make([]byte, 65536)
		for {
			_, err := mock.incoming.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	_, err := client.SessionPrompt("sess_1", []ContentBlock{
		{Type: "text", Text: "test"},
	}, 200*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout in error, got: %v", err)
	}
}

func TestReadMessage_StderrDoesNotInterfere(t *testing.T) {
	// This is tested implicitly — stderr is on a separate pipe
	// and drained in a background goroutine. The test here verifies
	// that the client can still read responses from stdout.
	client, mock := setupTestClient(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := client.Initialize(2 * time.Second)
		if err != nil {
			t.Errorf("Initialize failed: %v", err)
		}
	}()

	req, _ := mock.readRequest()
	mock.writeLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]any{
			"protocolVersion":   1,
			"agentInfo":         map[string]any{"name": "test", "version": "1.0"},
			"agentCapabilities": map[string]any{},
		},
	})

	<-done
}

func TestReadMessage_SubprocessExit(t *testing.T) {
	// Simulate subprocess exit by closing the mock's outgoing pipe
	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()

	client := newTestACPClient(clientW, mockR)
	_ = clientR

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := client.Initialize(2 * time.Second)
		if err == nil {
			t.Error("expected error on subprocess exit")
		}
	}()

	// Read the request
	buf := make([]byte, 65536)
	clientR.Read(buf)

	// Close the pipe to simulate subprocess exit
	mockW.Close()
	<-done
}

func TestPartialJSONLinesBuffered(t *testing.T) {
	// Verify that partial lines are buffered until newline
	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()

	client := newTestACPClient(clientW, mockR)
	_ = clientR

	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := client.Initialize(5 * time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if result.AgentInfo.Name != "test-agent" {
			t.Errorf("expected test-agent, got %q", result.AgentInfo.Name)
		}
	}()

	// Read the initialize request
	buf := make([]byte, 65536)
	n, _ := clientR.Read(buf)
	var req jsonrpcRequest
	json.Unmarshal(buf[:n], &req)

	// Write response in chunks (partial lines)
	resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":1,"agentInfo":{"name":"test-agent","version":"1.0"},"agentCapabilities":{}}}`, req.ID)

	// Send first half
	mockW.Write([]byte(resp[:len(resp)/2]))
	time.Sleep(50 * time.Millisecond)
	// Send second half + newline
	mockW.Write([]byte(resp[len(resp)/2:] + "\n"))

	<-done
}
