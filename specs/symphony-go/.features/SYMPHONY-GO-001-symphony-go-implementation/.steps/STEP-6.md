# Step 6: Agent Runner

**Covers:** FR-6 (Agent Runner — composition layer)
**Package:** `internal/agent`

---

## 1. Tasks

### 1.1 Runner Interface

- [ ] `internal/agent/runner.go`:
  - Define `AgentLauncher` interface for mock injection:
    ```go
    type AgentLauncher interface {
        Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error
    }
    ```
  - `OrchestratorEvent` struct: `Type EventType`, `IssueID string`, `Payload any`
  - `EventType` enum: `EventWorkerDone`, `EventWorkerFailed`, `EventAgentUpdate`

### 1.2 Agent Runner Implementation

- [ ] `internal/agent/runner.go`:
  - `GeminiRunner` struct implementing `AgentLauncher`:
    - Fields: `workspaceMgr *workspace.Manager`, `geminiCfg *config.GeminiConfig`, `agentCfg *config.AgentConfig`
  - `(r *GeminiRunner) Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error`:
    1. **Create/reuse workspace**: `workspaceMgr.CreateForIssue(params.Issue.Identifier)`
    2. **Run `before_run` hook**: `workspaceMgr.RunBeforeRun(ws.Path)` — return error on failure
    3. **Validate workspace path**: `workspace.ValidateWorkspacePath(ws.Path, root)` — safety check before launch
    4. **Launch ACP client**: `NewACPClient(geminiCfg.Command, ws.Path)`
    5. **ACP handshake**: `client.Initialize(ctx, readTimeout)` → `client.SessionNew(ctx, ws.Path, readTimeout)`
    6. **Emit `session_started`** event with sessionId
    7. **Turn loop** (up to `agentCfg.MaxTurns`):
       a. Build prompt: first turn → full rendered prompt, continuation → continuation guidance
       b. `client.SessionPrompt(ctx, sessionId, prompt, turnTimeout, updateHandler)`
       c. `updateHandler` forwards events to `eventCh` (agent updates, tool calls, etc.)
       d. Check turn result: `end_turn` → may continue; `max_tokens`, `refusal`, `cancelled` → stop
       e. Re-check issue state via tracker (caller provides a state-check function)
       f. If issue no longer active → break
       g. Increment turn count
    8. **Cleanup**: `client.Close()`, `workspaceMgr.RunAfterRun(ws.Path)`
    9. Return nil on success, error on failure

  - On any error during steps 4-7:
    - Close ACP client if open
    - Run `after_run` hook (best effort)
    - Return error

### 1.3 Prompt Building Helper

- [ ] `internal/agent/runner.go` (or separate helper):
  - `buildTurnPrompt(workflow *workflow.WorkflowDefinition, issue *tracker.Issue, attempt *int, turnNumber int, maxTurns int) (string, error)`:
    - Turn 1: `prompt.RenderPrompt(workflow.PromptTemplate, issue, attempt)`
    - Turn 2+: Return continuation guidance string:
      ```
      Continue working on this issue. You are on turn {{turnNumber}} of {{maxTurns}}.
      Check the current state of your work and continue from where you left off.
      The issue is still in an active state in the tracker.
      ```
    - Render errors fail the attempt

### 1.4 State Check Callback

The runner needs to re-check issue state between turns but shouldn't directly depend on the tracker client. Use a callback:

```go
type RunParams struct {
    Issue          *tracker.Issue
    Attempt        *int
    WorkspacePath  string // pre-resolved (may be empty if runner creates workspace)
    Workflow       *workflow.WorkflowDefinition
    CheckIssueState func(ctx context.Context, issueID string) (string, error) // returns current state name
}
```

### 1.5 Tests

- [ ] `internal/agent/runner_test.go`:
  - Use pipe-based mock ACP client (from Step 5)
  - Use mock workspace manager (in-memory)
  - Test: full lifecycle — workspace create → before_run → ACP handshake → single turn → after_run → success
  - Test: workspace creation failure → error returned, no ACP launched
  - Test: before_run hook failure → error returned, no ACP launched
  - Test: ACP handshake failure → after_run still called, error returned
  - Test: turn failure → after_run called, error returned
  - Test: multi-turn: 2 turns, issue still active after turn 1, exits after turn 2
  - Test: multi-turn: issue becomes non-active after turn 1, exits early
  - Test: max_turns reached → exits gracefully
  - Test: prompt render failure → error returned

---

## 2. Dependencies

No new dependencies. Composes `internal/workspace`, `internal/prompt`, `internal/agent` (ACP client from Step 5).

---

## 3. Definition of Done

- [ ] `go test ./internal/agent/...` — all pass (including Step 5 tests)
- [ ] Full agent lifecycle works with mock ACP + mock workspace
- [ ] Turn loop respects max_turns and issue state changes
- [ ] Error paths run cleanup hooks
- [ ] `AgentLauncher` interface ready for orchestrator mock injection
