# Step 4: Orchestrator + Agent Runner + main.go Integration

**Covers:** FR-3 (raw protocol logging), FR-5 (process control), full end-to-end wiring
**Packages:** `internal/orchestrator`, `internal/agent`, `cmd/symphony`

---

## 1. Tasks

### 1.1 Add EventLogWriter to RunParams

- [ ] In `internal/agent/runner.go`, add field to `RunParams`:
  ```go
  EventLogWriter io.Writer // if non-nil, raw protocol events are written here
  ```

### 1.2 Wire EventLogWriter into GeminiRunner

- [ ] In `internal/agent/runner.go` `Launch()`, after ACP initialize (line ~114):
  ```go
  if params.EventLogWriter != nil {
      fmt.Fprintf(params.EventLogWriter, "[%s] [SYMPHONY] ACP initialized — agent: %s, protocol: %s\n",
          time.Now().Format("15:04:05"), initResult.AgentInfo.Name, initResult.ProtocolVersion)
  }
  ```
- [ ] In `Launch()`, after session created (line ~121):
  ```go
  if params.EventLogWriter != nil {
      fmt.Fprintf(params.EventLogWriter, "[%s] [SYMPHONY] Session created: %s\n",
          time.Now().Format("15:04:05"), sessionID)
  }
  ```
- [ ] In the `updateHandler` closure (line ~157), write raw update JSON:
  ```go
  if params.EventLogWriter != nil {
      raw, _ := json.Marshal(update)
      fmt.Fprintf(params.EventLogWriter, "[%s] %s\n", time.Now().Format("15:04:05"), raw)
  }
  ```
- [ ] After turn completed (line ~191):
  ```go
  if params.EventLogWriter != nil {
      fmt.Fprintf(params.EventLogWriter, "[%s] [SYMPHONY] Turn %d completed — %s\n",
          time.Now().Format("15:04:05"), turnNumber, result.StopReason)
  }
  ```
- [ ] On turn failed (line ~177):
  ```go
  if params.EventLogWriter != nil {
      fmt.Fprintf(params.EventLogWriter, "[%s] [SYMPHONY] Turn %d failed — %s\n",
          time.Now().Format("15:04:05"), turnNumber, err.Error())
  }
  ```

### 1.3 Wire EventLogWriter into ClaudeRunner

- [ ] In `internal/agent/claude_runner.go`, pass `EventLogWriter` through to `collectClaudeOutput`. Add parameter:
  ```go
  func collectClaudeOutput(
      ctx context.Context,
      cmd *exec.Cmd,
      stdout io.ReadCloser,
      currentSessionID string,
      turnTimeoutMs int,
      eventCh chan<- OrchestratorEvent,
      issueID string,
      eventLogWriter io.Writer,  // new parameter
  ) (lastResultType string, sessionID string, err error)
  ```
- [ ] In the NDJSON read loop (line ~273), after `parser.Feed(chunk)` returns events, write each raw NDJSON line to the log writer before mapping:
  ```go
  for _, evt := range events {
      if eventLogWriter != nil {
          // Write raw JSON from evt.Raw
          raw, _ := json.Marshal(evt.Raw)
          fmt.Fprintf(eventLogWriter, "[%s] %s\n", time.Now().Format("15:04:05"), raw)
      }
      // ... existing mapNdjsonToAgentEvent and emit ...
  }
  ```
- [ ] Same for the flush loop (line ~323)
- [ ] In `Launch()` (line ~91), update the `collectClaudeOutput` call to pass `params.EventLogWriter`:
  ```go
  turnResult, newSessionID, collectErr := collectClaudeOutput(
      ctx, cmd, stdout, sessionID,
      params.ClaudeCfg.TurnTimeoutMs, eventCh, params.Issue.ID,
      params.EventLogWriter,
  )
  ```
- [ ] In `Launch()`, write turn start annotation:
  ```go
  if params.EventLogWriter != nil {
      fmt.Fprintf(params.EventLogWriter, "[%s] [SYMPHONY] Starting turn %d of %d\n",
          time.Now().Format("15:04:05"), turnNumber, maxTurns)
  }
  ```

### 1.4 Wire CmuxManager into Orchestrator

- [ ] In `internal/orchestrator/orchestrator.go`, add field:
  ```go
  cmuxMgr *cmux.Manager
  ```
- [ ] Update `New()` signature to accept `*cmux.Manager`:
  ```go
  func New(
      cfg *config.Config,
      wf *workflow.WorkflowDefinition,
      trackerClient tracker.TrackerClient,
      launcher agent.AgentLauncher,
      workspaceMgr *workspace.Manager,
      cmuxMgr *cmux.Manager,
  ) *Orchestrator
  ```
  Store in struct: `cmuxMgr: cmuxMgr`

- [ ] In `dispatchIssue()`, after creating `RunningEntry` (line ~213), before the goroutine:
  ```go
  // Create cmux surface for visibility
  wsPath := filepath.Join(cfg.Workspace.Root, issue.Identifier)
  if err := o.cmuxMgr.CreateSurface(issue.ID, issue.Identifier, wsPath); err != nil {
      slog.Warn("cmux surface creation failed", "issue_identifier", issue.Identifier, "error", err)
  }
  ```

- [ ] In `dispatchIssue()`, add `EventLogWriter` to RunParams:
  ```go
  params := agent.RunParams{
      // ... existing fields ...
      EventLogWriter: o.cmuxMgr.LogWriter(issue.ID),
  }
  ```

- [ ] In `handleEvent()`, in the `EventAgentUpdate` case (line ~319), after updating state:
  ```go
  // Mirror event message to cmux log
  o.cmuxMgr.WriteEvent(ev.IssueID, agentEvt.Message)
  ```

- [ ] In `handleEvent()`, in the `EventWorkerDone` case (line ~272), before `removeRunning`:
  ```go
  o.cmuxMgr.WriteAnnotation(ev.IssueID, "Worker completed normally")
  o.cmuxMgr.CloseSurface(ev.IssueID)
  ```

- [ ] In `handleEvent()`, in the `EventWorkerFailed` case (line ~289), before `removeRunning`:
  ```go
  o.cmuxMgr.WriteAnnotation(ev.IssueID, fmt.Sprintf("Worker failed: %s", errMsg))
  o.cmuxMgr.CloseSurface(ev.IssueID)
  ```

- [ ] In `shutdown()`, after cancelling workers and retries:
  ```go
  o.cmuxMgr.Shutdown()
  ```

### 1.5 Wire CmuxManager into main.go

- [ ] In `cmd/symphony/main.go`, after workspace manager creation (line ~80):
  ```go
  cmuxMgr := cmux.New(&resolved.Cmux)
  if resolved.Cmux.Enabled {
      if err := cmuxMgr.Init(); err != nil {
          slog.Warn("cmux initialization failed, continuing without visibility", "error", err)
      }
  }
  ```
- [ ] Add import: `"github.com/symphony-go/symphony/internal/cmux"`
- [ ] Update orchestrator creation (line ~88) to pass cmuxMgr:
  ```go
  orch := orchestrator.New(resolved, wf, trackerClient, launcher, workspaceMgr, cmuxMgr)
  ```
- [ ] Add to shutdown sequence (before `orch.Run`):
  ```go
  defer cmuxMgr.Shutdown()
  ```
- [ ] Add cmux status to startup log:
  ```go
  slog.Info("symphony-go starting",
      // ... existing fields ...
      "cmux_enabled", resolved.Cmux.Enabled,
  )
  ```

### 1.6 Tests

- [ ] In `internal/agent/runner_test.go`, add:
  - `TestGeminiRunnerWritesRawEvents` — create a `bytes.Buffer` as EventLogWriter, run with mock ACP, verify log contains ACP messages and `[SYMPHONY]` annotations
  - `TestGeminiRunnerNilLogWriter` — verify no panic when EventLogWriter is nil

- [ ] In `internal/agent/claude_runner_test.go`, add:
  - `TestClaudeRunnerWritesRawEvents` — create a `bytes.Buffer` as EventLogWriter, run with mock process outputting NDJSON, verify log contains raw JSON lines
  - `TestClaudeRunnerNilLogWriter` — verify no panic when EventLogWriter is nil

- [ ] In `internal/orchestrator/` (new file `cmux_test.go` or extend existing):
  - `TestDispatchCallsCreateSurface` — use mock cmux manager, dispatch issue, verify CreateSurface called with correct args
  - `TestHandleEventWritesEvent` — emit EventAgentUpdate, verify WriteEvent called
  - `TestWorkerDoneClosesSurface` — emit EventWorkerDone, verify WriteAnnotation + CloseSurface called
  - `TestDispatchWithoutCmux` — with disabled cmux manager, verify dispatch works identically (no panics)
  - `TestShutdownCallsCmuxShutdown` — shutdown orchestrator, verify cmux Shutdown called

## 2. Definition of Done

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes (all existing + new tests)
- [ ] With `cmux.enabled: true` in WORKFLOW.md: agent sessions appear as cmux terminal tabs
- [ ] With `cmux.enabled: false` (default): behavior is identical to before this feature
- [ ] Raw protocol data (NDJSON for Claude, JSON-RPC for Gemini) streams to cmux log in real-time
- [ ] Surfaces are created on dispatch and closed on completion
