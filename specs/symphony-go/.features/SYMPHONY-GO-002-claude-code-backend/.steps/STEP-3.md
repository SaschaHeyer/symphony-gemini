# STEP-3: Claude Runner Implementation

**Goal:** Fully implement `ClaudeRunner.Launch()` — PTY wrapper, CLI arg builder, NDJSON stream parsing, session persistence, turn loop, and event mapping.

**Depends on:** STEP-1 (NdjsonParser), STEP-2 (ClaudeConfig, ClaudeRunner placeholder)

---

## File to Create / Replace

### `internal/agent/claude_runner.go`

Replace the placeholder from STEP-2 with full implementation.

#### Struct

```go
type ClaudeRunner struct{}

func NewClaudeRunner() *ClaudeRunner { return &ClaudeRunner{} }
```

#### Main Entry: `Launch()`

```go
func (r *ClaudeRunner) Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error {
    logger := slog.With("issue_id", params.Issue.ID, "issue_identifier", params.Issue.Identifier)

    // 1. Create/reuse workspace
    ws, err := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
    // ... (same pattern as GeminiRunner)

    // 2. Validate workspace path
    workspace.ValidateWorkspacePath(ws.Path, params.WorkspaceRoot)

    // 3. Run before_run hook
    params.WorkspaceMgr.RunBeforeRun(ws.Path)

    defer params.WorkspaceMgr.RunAfterRun(ws.Path)

    // 4. Read existing session ID (may be empty on first run)
    sessionID := readSessionID(ws.Path)

    // 5. Turn loop
    maxTurns := params.AgentCfg.MaxTurns
    if maxTurns <= 0 { maxTurns = 20 }

    for turnNumber := 1; turnNumber <= maxTurns; turnNumber++ {
        select {
        case <-ctx.Done(): return ctx.Err()
        default:
        }

        // Build prompt
        turnPrompt, err := buildTurnPrompt(params.Workflow, params.Issue, params.Attempt, turnNumber, maxTurns)

        // Build CLI args
        args := buildClaudeArgs(params.ClaudeCfg, turnPrompt, sessionID, ws.Path)

        // Spawn process with PTY
        cmd, stdout, err := spawnWithPTY(params.ClaudeCfg.Command, args, ws.Path, params.ExtraEnv)
        cmd.Start()

        // Parse NDJSON & emit events, with turn timeout
        turnResult, newSessionID, err := collectClaudeOutput(ctx, cmd, stdout, sessionID, params.ClaudeCfg.TurnTimeoutMs, eventCh, params.Issue.ID)

        // Persist session ID if we got one
        if newSessionID != "" && newSessionID != sessionID {
            writeSessionID(ws.Path, newSessionID)
            sessionID = newSessionID
        }

        // Check result
        if turnResult is error → return error
        if turnResult is success → emit turn_completed

        // Re-check issue state (if not last turn)
        if params.CheckIssueState != nil {
            state, _ := params.CheckIssueState(ctx, params.Issue.ID)
            if !isActiveState(state, params) → break
        }
    }

    return nil
}
```

#### CLI Argument Builder

```go
func buildClaudeArgs(cfg *config.ClaudeConfig, prompt string, sessionID string, workspace string) []string
```

Arguments to build (in order):
1. `-p`, prompt
2. `--output-format`, `stream-json`
3. `--max-turns`, strconv.Itoa(cfg.MaxTurns)
4. `--model`, cfg.Model
5. If sessionID != "": `--resume`, sessionID
6. If cfg.PermissionMode != "": `--permission-mode`, cfg.PermissionMode
7. For each tool in cfg.AllowedTools: `--allowedTools`, tool
8. If `.mcp.json` exists in workspace: `--mcp-config`, path

#### PTY Wrapper

```go
func spawnWithPTY(executable string, args []string, cwd string, extraEnv []string) (*exec.Cmd, io.ReadCloser, error)
```

- Look up `script` on PATH via `exec.LookPath("script")`.
- If found: build shell command string by joining executable+args with proper escaping, then spawn: `script -q /dev/null /bin/sh -c '<joined_command>'`.
- If not found: warn and spawn executable directly.
- Set `cmd.Dir = cwd`, `cmd.Env = append(os.Environ(), extraEnv...)`.
- Return `cmd`, `stdout` pipe, error.

Shell escaping helper:
```go
func shellEscape(s string) string {
    return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func shellJoin(args []string) string {
    escaped := make([]string, len(args))
    for i, a := range args { escaped[i] = shellEscape(a) }
    return strings.Join(escaped, " ")
}
```

#### NDJSON Collection

```go
func collectClaudeOutput(
    ctx context.Context,
    cmd *exec.Cmd,
    stdout io.ReadCloser,
    currentSessionID string,
    turnTimeoutMs int,
    eventCh chan<- OrchestratorEvent,
    issueID string,
) (lastResultType string, sessionID string, err error)
```

Logic:
1. Create `NdjsonParser`.
2. Create timeout context from `turnTimeoutMs`.
3. Read loop: `bufio.NewReader(stdout)`, read in chunks (4KB), feed to parser.
4. For each `NdjsonEvent`:
   - Map to `AgentEvent` via `mapNdjsonToAgentEvent()`.
   - Send to `eventCh`.
   - If event is `system/init`, extract `session_id` from `Raw`.
   - If event is `result/*`, record as last result.
5. On timeout: kill process, return error.
6. Wait for process exit.
7. On non-zero exit: return error.
8. Flush parser for any remaining buffered data.
9. Return (lastResultType, sessionID, nil).

#### Event Mapping

```go
func mapNdjsonToAgentEvent(evt NdjsonEvent, sessionID string) AgentEvent
```

Mapping table (from FUNCTIONAL.md AC-3.25):
- `system/init` → `EventSessionStarted`
- `assistant` with tool_use → `EventToolCall`
- `assistant` text only → `EventNotification`
- `result/success` → `EventTurnCompleted`
- `result/error` → `EventTurnFailed`
- `result/*` (other) → `EventTurnCompleted`
- all others → `EventNotification`

Extract token usage from `result` events: `evt.Raw["usage"]` → `extractTokenUsageFromMap()` (reuse from `acp.go`).

#### Session Persistence

```go
const claudeSessionIDFile = ".symphony-session-id"

func readSessionID(workspace string) string
func writeSessionID(workspace string, sessionID string) error
```

#### Helper: hasToolUse

```go
func hasToolUse(raw map[string]any) bool {
    msg, ok := raw["message"].(map[string]any)
    if !ok { return false }
    content, ok := msg["content"].([]any)
    if !ok { return false }
    for _, c := range content {
        if block, ok := c.(map[string]any); ok {
            if block["type"] == "tool_use" { return true }
        }
    }
    return false
}
```

---

## File to Create

### `internal/agent/claude_runner_test.go`

| Test | What |
|---|---|
| `TestBuildClaudeArgs_FirstTurn` | No --resume when sessionID is empty |
| `TestBuildClaudeArgs_WithResume` | --resume appended with session ID |
| `TestBuildClaudeArgs_AllowedTools` | --allowedTools for each tool |
| `TestBuildClaudeArgs_WithMcpConfig` | --mcp-config when .mcp.json exists |
| `TestBuildClaudeArgs_NoMcpConfig` | No --mcp-config when .mcp.json absent |
| `TestBuildClaudeArgs_PermissionMode` | --permission-mode set correctly |
| `TestReadSessionID_Exists` | Read existing file, trimmed |
| `TestReadSessionID_Missing` | Missing file → empty string |
| `TestWriteSessionID_RoundTrip` | Write then read back |
| `TestMapNdjsonToAgentEvent_SystemInit` | system/init → EventSessionStarted |
| `TestMapNdjsonToAgentEvent_ResultSuccess` | result/success → EventTurnCompleted with usage |
| `TestMapNdjsonToAgentEvent_ResultError` | result/error → EventTurnFailed |
| `TestMapNdjsonToAgentEvent_ToolUse` | assistant with tool_use → EventToolCall |
| `TestMapNdjsonToAgentEvent_AssistantText` | assistant text → EventNotification |
| `TestHasToolUse_True` | Content array with tool_use block → true |
| `TestHasToolUse_False` | Content array with text only → false |
| `TestShellEscape` | Various strings including single quotes |
| `TestCollectClaudeOutput_MockProcess` | Mock script outputs NDJSON, verify events and session ID extraction |

**Mock strategy for `TestCollectClaudeOutput_MockProcess`:**
Create a temp shell script that echoes canned NDJSON lines to stdout:
```bash
#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"test-123"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Working..."}]}}'
echo '{"type":"result","subtype":"success","usage":{"input_tokens":100,"output_tokens":50},"session_id":"test-123"}'
```
Set `params.ClaudeCfg.Command` to this script path. Verify events received and session ID extracted.

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./internal/agent/ -run TestBuildClaudeArgs` passes
- [ ] `go test ./internal/agent/ -run TestReadSessionID` passes
- [ ] `go test ./internal/agent/ -run TestWriteSessionID` passes
- [ ] `go test ./internal/agent/ -run TestMapNdjsonToAgentEvent` passes
- [ ] `go test ./internal/agent/ -run TestHasToolUse` passes
- [ ] `go test ./internal/agent/ -run TestShellEscape` passes
- [ ] `go test ./internal/agent/ -run TestCollectClaudeOutput` passes
- [ ] All existing tests still pass
