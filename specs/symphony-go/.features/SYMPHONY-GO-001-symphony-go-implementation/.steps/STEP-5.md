# Step 5: ACP Client (Gemini CLI JSON-RPC Protocol)

**Covers:** FR-6 (Agent Runner — protocol layer only)
**Package:** `internal/agent`

---

## 1. Tasks

### 1.1 JSON-RPC Message Types

- [ ] `internal/agent/messages.go`:
  - All ACP message structs per TECH.md Section 3.1:
    - `JSONRPCRequest`, `JSONRPCResponse`, `JSONRPCNotification`, `JSONRPCError`
    - `InitializeParams`, `InitializeResult`, `ClientInfo`, `ClientCapabilities`, `FSCapabilities`
    - `AgentInfo`, `AgentCapabilities`, `PromptCapabilities`
    - `SessionNewParams`, `SessionNewResult`
    - `SessionPromptParams`, `SessionPromptResult`, `ContentBlock`
    - `SessionUpdateParams`, `SessionUpdate`
    - `RequestPermissionParams`, `RequestPermissionResult`, `PermissionOutcome`, `PermissionOption`, `ToolCallRef`

### 1.2 ACP Client Core

- [ ] `internal/agent/acp.go`:
  - `ACPClient` struct: `cmd *exec.Cmd`, `stdin io.WriteCloser`, `stdout *bufio.Reader`, `nextID int`, `mu sync.Mutex`
  - `NewACPClient(command string, cwd string) (*ACPClient, error)`:
    - Launch subprocess via `exec.Command("bash", "-lc", command)`
    - Set `cmd.Dir = cwd`
    - Pipe stdin, stdout, stderr
    - Start process
    - Launch stderr drain goroutine (read + log lines, never parse as protocol)
  - `sendRequest(method string, params any) (int, error)`:
    - Increment `nextID`, marshal `JSONRPCRequest`, write line to stdin
  - `readLine(timeout time.Duration) ([]byte, error)`:
    - Read one line from stdout with timeout via context
    - Buffer partial lines until newline
  - `readResponse(id int, timeout time.Duration) (*JSONRPCResponse, error)`:
    - Read lines until we get a response matching our request ID
    - Non-matching lines: check if notification or agent-initiated request, handle inline
  - `Close() error`:
    - Close stdin, kill process if still running, wait

### 1.3 ACP Session Methods

- [ ] `internal/agent/session.go`:
  - `(c *ACPClient) Initialize(ctx context.Context, readTimeout time.Duration) (*InitializeResult, error)`:
    - Send `initialize` with `protocolVersion: 1`, `clientInfo: {name: "symphony-go", version: "1.0"}`, `clientCapabilities: {fs: {readTextFile: true, writeTextFile: true}, terminal: true}`
    - Wait for response within `readTimeout`
    - Verify protocol version compatibility
  - `(c *ACPClient) SessionNew(ctx context.Context, cwd string, readTimeout time.Duration) (string, error)`:
    - Send `session/new` with `cwd` and `mcpServers: []`
    - Wait for response, extract `sessionId`
  - `(c *ACPClient) SessionPrompt(ctx context.Context, sessionID string, prompt []ContentBlock, turnTimeout time.Duration, handler UpdateHandler) (*SessionPromptResult, error)`:
    - Send `session/prompt` with `sessionId` and `prompt`
    - Enter read loop:
      - Read lines until turn completes (response to our prompt request ID)
      - For each `session/update` notification → call `handler(update)`
      - For each `session/request_permission` request → auto-approve: find first option with kind containing "allow", respond with `{outcome: "selected", optionId: id}`
      - Enforce `turnTimeout` on the entire loop
    - Parse `StopReason` from response
  - `(c *ACPClient) SessionCancel(sessionID string) error`:
    - Send `session/cancel` notification (no response expected)

  - `UpdateHandler` type: `func(update *SessionUpdateParams)`

### 1.4 Event Types

- [ ] `internal/agent/events.go`:
  - `AgentEvent` struct: `Type string`, `Timestamp time.Time`, `SessionID string`, `Payload any`
  - Event type constants: `EventSessionStarted`, `EventTurnCompleted`, `EventTurnFailed`, `EventTurnCancelled`, `EventNotification`, `EventToolCall`, `EventMalformed`
  - `ExtractTokenUsage(update *SessionUpdateParams) *TokenUsage`:
    - Best-effort extraction of token counts from session update payloads
    - Return nil if no usage data found

### 1.5 Tests

- [ ] `internal/agent/acp_test.go`:
  - Create a mock ACP server: small helper that spawns a goroutine pair (write to client's stdin, read from client's stdout) simulating Gemini CLI responses
  - **Approach:** Use `os.Pipe()` pairs to simulate stdin/stdout without a real subprocess
  - Test: `Initialize` sends correct JSON-RPC, receives capabilities
  - Test: `SessionNew` sends cwd, receives sessionId
  - Test: `SessionPrompt` sends prompt, receives `session/update` notifications, returns `StopReason`
  - Test: `session/request_permission` auto-approved with first allow option
  - Test: read timeout enforced on handshake (no response → error)
  - Test: turn timeout enforced (slow response → error)
  - Test: partial JSON lines buffered until newline
  - Test: stderr content does not interfere with protocol parsing
  - Test: subprocess exit during turn → error

---

## 2. Testing Strategy for Subprocess Simulation

Rather than launching a real process, tests use pipe-based simulation:

```go
func newTestACPClient(t *testing.T) (*ACPClient, *mockAgent) {
    // Create pipe pairs for stdin/stdout
    clientStdinR, clientStdinW := io.Pipe()  // client writes, mock reads
    mockStdoutR, mockStdoutW := io.Pipe()    // mock writes, client reads

    client := &ACPClient{
        stdin:  clientStdinW,
        stdout: bufio.NewReader(mockStdoutR),
        nextID: 0,
    }

    mock := &mockAgent{
        incoming: clientStdinR,  // reads what client sends
        outgoing: mockStdoutW,   // writes responses/notifications
    }

    return client, mock
}
```

The mock agent reads requests and writes back canned JSON-RPC responses/notifications.

---

## 3. Dependencies

No new dependencies. Uses `os/exec`, `bufio`, `encoding/json`, `io`, `sync` from stdlib.

---

## 4. Definition of Done

- [ ] `go test ./internal/agent/...` — all pass
- [ ] ACP handshake (initialize → session/new) works with pipe-based mock
- [ ] Turn streaming (session/prompt → session/update* → response) works
- [ ] Permission auto-approval works
- [ ] Timeouts enforced on all blocking reads
