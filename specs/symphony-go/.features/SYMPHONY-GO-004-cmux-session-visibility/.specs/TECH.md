# TECH SPEC: cmux Session Visibility — Architecture & Data Models

**Status:** Draft v1
**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-004
**Depends on:** FUNCTIONAL.md v1, SYMPHONY-GO-001, SYMPHONY-GO-002

---

## 1. Architecture Overview

The cmux integration is a **display-only layer** that mirrors agent events to visible cmux terminal surfaces. It sits alongside the existing process management — no changes to how agents are launched, how events flow, or how the orchestrator manages state.

```
┌─────────────────────────────────────────────────────────┐
│                    Orchestrator                          │
│                                                         │
│   dispatchIssue() ──┬──> AgentLauncher.Launch()         │
│                     │        │                          │
│                     │        ├─ emits AgentEvent ──────>│──> handleEvent()
│                     │        │                          │
│                     └──> CmuxManager.CreateSurface()    │
│                              │                          │
│   handleEvent() ────────> CmuxManager.WriteEvent()      │
│                              │                          │
│   removeRunning() ──────> CmuxManager.CloseSurface()    │
│                              │                          │
└──────────────────────────────│──────────────────────────┘
                               ▼
                    ┌──────────────────┐
                    │   cmux CLI       │
                    │  (subprocess)    │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │  cmux workspace  │
                    │  "Symphony"      │
                    │                  │
                    │  ┌──────────┐    │
                    │  │ AIE-10   │    │  ← terminal surface running
                    │  │ tail -f  │    │    tail -f <workspace>/.symphony-agent.log
                    │  └──────────┘    │
                    │  ┌──────────┐    │
                    │  │ AIE-11   │    │
                    │  │ tail -f  │    │
                    │  └──────────┘    │
                    └──────────────────┘
```

**Key principle:** The `CmuxManager` is the only component that calls the cmux CLI. All other components interact with it via Go method calls. If cmux is disabled or unavailable, all methods are no-ops.

## 2. New Package: `internal/cmux`

### 2.1 CmuxConfig

```go
// internal/config/config.go — new field on Config struct

type CmuxConfig struct {
    Enabled       bool   `yaml:"enabled"        json:"enabled"`
    WorkspaceName string `yaml:"workspace_name" json:"workspace_name"`
    CloseDelayMs  int    `yaml:"close_delay_ms" json:"close_delay_ms"`
}
```

**Defaults** (in `internal/config/defaults.go`):
```go
Cmux: CmuxConfig{
    Enabled:       false,
    WorkspaceName: "Symphony",
    CloseDelayMs:  30000,
},
```

### 2.2 Manager Interface

```go
// internal/cmux/manager.go

// Manager handles cmux workspace and surface lifecycle.
// All methods are safe to call even when cmux is disabled — they become no-ops.
type Manager struct {
    enabled       bool
    workspaceName string
    closeDelayMs  int
    cmuxBin       string            // resolved path to cmux binary
    workspaceRef  string            // cmux workspace ref (e.g., "workspace:5")
    surfaces      map[string]string // issueID → surface ref
    logFiles      map[string]*os.File // issueID → open log file handle
    mu            sync.Mutex
}

// New creates a CmuxManager. If enabled, it locates the cmux binary and
// verifies connectivity via `cmux ping`. Returns a no-op manager on failure.
func New(cfg *config.CmuxConfig) *Manager

// Init creates or reuses the cmux workspace. Called once at startup.
// Returns error only for logging — caller should not fail on cmux errors.
func (m *Manager) Init() error

// CreateSurface creates a terminal surface for an issue.
// The surface runs `tail -f <logPath>` and is named after the issue identifier.
// If a surface already exists for this issue (retry), it is reused.
func (m *Manager) CreateSurface(issueID, identifier, workspacePath string) error

// WriteEvent writes a formatted event line to the issue's log file.
// Format: [HH:MM:SS] <content>
// This is the hot path — must not block on cmux CLI calls.
func (m *Manager) WriteEvent(issueID string, content string)

// WriteAnnotation writes a Symphony status annotation to the log file.
// Format: [HH:MM:SS] [SYMPHONY] <message>
func (m *Manager) WriteAnnotation(issueID string, message string)

// CloseSurface closes a surface after the configured delay.
// Spawns a goroutine that sleeps for closeDelayMs then closes.
func (m *Manager) CloseSurface(issueID string)

// Shutdown closes all surfaces and log files immediately.
func (m *Manager) Shutdown()
```

### 2.3 cmux CLI Interactions

All cmux interactions use `exec.Command` to call the cmux binary. Each call is wrapped in a helper with a 5-second timeout.

```go
// internal/cmux/exec.go

// run executes a cmux CLI command and returns stdout.
func (m *Manager) run(args ...string) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, m.cmuxBin, args...)
    out, err := cmd.Output()
    return strings.TrimSpace(string(out)), err
}
```

**Startup sequence** (`Init`):
```
1. cmux ping                           → verify socket alive
2. cmux list-workspaces --json         → check if workspace exists
3. cmux new-workspace                  → create if not found
   OR reuse existing workspace ref
4. cmux rename-tab --workspace <ref> "Symphony"
```

**Surface creation** (`CreateSurface`):
```
1. Create log file: <workspacePath>/.symphony-agent.log (append mode)
2. cmux new-surface --type terminal --workspace <ref>
   → returns surface ref (e.g., "surface:12")
3. cmux rename-tab --surface <ref> "<identifier>"
4. cmux surface.send_text "tail -f <workspacePath>/.symphony-agent.log\n" --surface <ref>
5. Store: surfaces[issueID] = surfaceRef, logFiles[issueID] = fileHandle
```

**Surface close** (`CloseSurface`):
```
1. Write final annotation: "[SYMPHONY] Session ended"
2. Sleep closeDelayMs
3. cmux close-surface --surface <ref>
4. Close log file handle
5. Remove from maps
```

### 2.4 Log File Format

The log file is plain text, one line per event, timestamp-prefixed:

```
[06:40:40] [SYMPHONY] Agent dispatched — turn 1 of 20
[06:40:41] {"type":"system","subtype":"init","session_id":"abc-123",...}
[06:40:42] {"type":"assistant","message":{"content":[{"type":"text","text":"Let me..."}]}}
[06:40:43] {"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read",...}]}}
[06:40:50] [SYMPHONY] Turn 1 completed — result/success
[06:41:51] [SYMPHONY] Retry scheduled — attempt 2, backoff 1000ms
[06:41:52] [SYMPHONY] Agent dispatched — turn 2 of 20
[06:41:53] {"jsonrpc":"2.0","method":"session/update","params":{"updates":[...]}}
```

- **Claude events**: raw NDJSON line (as received from stdout)
- **Gemini events**: raw JSON-RPC message (both requests sent and responses received)
- **Symphony annotations**: prefixed with `[SYMPHONY]`

The `WriteEvent` method writes directly to the open file handle (buffered `os.File`) — no cmux CLI calls on the hot path.

## 3. Integration Points

### 3.1 Config Changes

**`internal/config/config.go`** — add field to Config:
```go
type Config struct {
    // ... existing fields ...
    Cmux CmuxConfig `yaml:"cmux" json:"cmux"`
}
```

**`internal/config/defaults.go`** — add defaults:
```go
Cmux: CmuxConfig{
    Enabled:       false,
    WorkspaceName: "Symphony",
    CloseDelayMs:  30000,
},
```

**`internal/config/config.go`** — add to `applyDefaults`:
```go
cmuxRaw, _ := raw["cmux"].(map[string]any)
if cmuxRaw == nil {
    cfg.Cmux = defaults.Cmux
} else {
    if _, ok := cmuxRaw["workspace_name"]; !ok {
        cfg.Cmux.WorkspaceName = defaults.Cmux.WorkspaceName
    }
    if _, ok := cmuxRaw["close_delay_ms"]; !ok {
        cfg.Cmux.CloseDelayMs = defaults.Cmux.CloseDelayMs
    }
}
```

### 3.2 Orchestrator Changes

**`internal/orchestrator/orchestrator.go`**:

Add `cmuxMgr` field to `Orchestrator` struct:
```go
type Orchestrator struct {
    // ... existing fields ...
    cmuxMgr *cmux.Manager
}
```

**`New()`** — accept and store CmuxManager:
```go
func New(
    cfg *config.Config,
    wf *workflow.WorkflowDefinition,
    trackerClient tracker.TrackerClient,
    launcher agent.AgentLauncher,
    workspaceMgr *workspace.Manager,
    cmuxMgr *cmux.Manager,       // new parameter
) *Orchestrator
```

**`dispatchIssue()`** — create surface before launching worker:
```go
func (o *Orchestrator) dispatchIssue(ctx context.Context, issue *tracker.Issue, attempt *int, cfg *config.Config) {
    // ... existing entry setup ...

    // Create cmux surface for visibility
    wsPath := filepath.Join(cfg.Workspace.Root, issue.Identifier)
    o.cmuxMgr.CreateSurface(issue.ID, issue.Identifier, wsPath)
    o.cmuxMgr.WriteAnnotation(issue.ID,
        fmt.Sprintf("Agent dispatched — %s [%s]", issue.Identifier, issue.State))

    // ... existing goroutine launch ...
}
```

**`handleEvent()`** — write events to cmux log:
```go
case agent.EventAgentUpdate:
    if agentEvt, ok := ev.Payload.(agent.AgentEvent); ok {
        // ... existing state update ...

        // Mirror to cmux
        o.cmuxMgr.WriteEvent(ev.IssueID, agentEvt.Message)
    }
```

**`removeRunning()`** call sites — close surface:
```go
// In handleEvent for EventWorkerDone and EventWorkerFailed:
o.cmuxMgr.WriteAnnotation(ev.IssueID, "Session ended")
o.cmuxMgr.CloseSurface(ev.IssueID)
```

**`shutdown()`** — clean up all surfaces:
```go
func (o *Orchestrator) shutdown() {
    // ... existing worker cancellation ...
    o.cmuxMgr.Shutdown()
}
```

### 3.3 Agent Runner Changes

The runners need to write **raw protocol data** to the cmux log, not just the summarized AgentEvent. This gives users full visibility into the wire protocol.

**Option A: Pass a log writer to runners via RunParams**

```go
// internal/agent/runner.go — add to RunParams
type RunParams struct {
    // ... existing fields ...
    EventLogWriter io.Writer  // if non-nil, raw protocol events are written here
}
```

**GeminiRunner** — in `Launch()`, write ACP messages:
```go
// After ACP initialize (line ~114):
if params.EventLogWriter != nil {
    fmt.Fprintf(params.EventLogWriter, "[%s] [SYMPHONY] ACP initialized — agent: %s, protocol: %s\n",
        timeNow(), initResult.AgentInfo.Name, initResult.ProtocolVersion)
}

// In updateHandler (line ~157), write raw update:
if params.EventLogWriter != nil {
    raw, _ := json.Marshal(update)
    fmt.Fprintf(params.EventLogWriter, "[%s] %s\n", timeNow(), raw)
}
```

**ClaudeRunner** — in `collectClaudeOutput()`, write raw NDJSON:
```go
// After parsing each NDJSON event (line ~274):
// The raw NDJSON line is already available as the parsed event's source.
// Write it to the log before mapping to AgentEvent.
if eventLogWriter != nil {
    fmt.Fprintf(eventLogWriter, "[%s] %s\n", timeNow(), rawLine)
}
```

**Orchestrator wiring** — in `dispatchIssue()`, pass the log writer:
```go
params := agent.RunParams{
    // ... existing fields ...
    EventLogWriter: o.cmuxMgr.LogWriter(issue.ID),
}
```

The `LogWriter` method returns an `io.Writer` that writes to the issue's log file (or `io.Discard` if cmux is disabled).

### 3.4 main.go Changes

```go
// After workspace manager creation:
cmuxMgr := cmux.New(&resolved.Cmux)
if resolved.Cmux.Enabled {
    if err := cmuxMgr.Init(); err != nil {
        slog.Warn("cmux initialization failed, continuing without visibility", "error", err)
    }
}

// Pass to orchestrator:
orch := orchestrator.New(resolved, wf, trackerClient, launcher, workspaceMgr, cmuxMgr)

// In shutdown:
defer cmuxMgr.Shutdown()
```

## 4. Data Flow

```
Agent Process (hidden subprocess)
    │
    ├─ stdout pipe ──> Runner parses events
    │                      │
    │                      ├─ Writes raw protocol line ──> LogWriter ──> .symphony-agent.log
    │                      │                                                      │
    │                      └─ Emits AgentEvent ──> eventCh ──> Orchestrator       │
    │                                                  │                          │
    │                                                  ├─ WriteAnnotation() ──────┤
    │                                                  │                          │
    │                                                  └─ Updates state           │
    │                                                                             ▼
    │                                                                   tail -f in cmux
    │                                                                   terminal surface
    └─ process exit ──> Runner returns ──> EventWorkerDone/Failed
```

## 5. Error Handling

| Scenario | Behavior |
|----------|----------|
| cmux binary not found | `New()` sets `enabled=false`, logs warning. All methods become no-ops. |
| cmux socket not responding (`ping` fails) | `Init()` returns error, logged as warning. Manager marks itself disabled. |
| Surface creation fails | `CreateSurface()` logs warning, returns error. Agent proceeds normally without visibility. |
| Log file write fails | `WriteEvent()`/`WriteAnnotation()` log warning once, continue. No retry. |
| cmux workspace deleted externally | Next `CreateSurface()` call re-creates workspace. |
| Surface close fails | `CloseSurface()` logs warning, cleans up local state anyway. |

**Core invariant:** cmux failures never affect agent execution or orchestrator state.

## 6. Hot Reload

When config is reloaded (`applyReload`):

- If `cmux.enabled` changes from `false` to `true`: call `Init()` to set up workspace. New dispatches will get surfaces.
- If `cmux.enabled` changes from `true` to `false`: stop creating new surfaces. Running surfaces remain until their agents complete.
- `workspace_name` and `close_delay_ms` changes take effect on next dispatch.

## 7. Dependencies

No new external dependencies. Uses only:
- `os/exec` for cmux CLI subprocess calls
- `os` for log file I/O
- Standard library `sync`, `time`, `fmt`, `io`

## 8. File Layout

```
internal/cmux/
├── manager.go        — Manager struct, New(), Init(), Shutdown()
├── surface.go        — CreateSurface(), CloseSurface(), surface lifecycle
├── log.go            — WriteEvent(), WriteAnnotation(), LogWriter()
├── exec.go           — run() helper, cmux CLI wrapper
└── manager_test.go   — tests with mock cmux binary
```

## 9. Mapping: FUNCTIONAL.md → Implementation

| FR | Primary File(s) | Key Function(s) |
|----|-----------------|-----------------|
| FR-1 | `internal/cmux/manager.go` | `New()`, `Init()` |
| FR-2 | `internal/cmux/surface.go` | `CreateSurface()`, `CloseSurface()`, `Shutdown()` |
| FR-3 | `internal/cmux/log.go`, `internal/agent/runner.go`, `internal/agent/claude_runner.go` | `WriteEvent()`, `WriteAnnotation()`, `LogWriter()` |
| FR-4 | `internal/config/config.go`, `internal/config/defaults.go` | `CmuxConfig` struct, `applyDefaults()` |
| FR-5 | No changes — existing process lifecycle preserved | — |
