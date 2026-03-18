# STEP-5: Orchestrator & main.go Integration

**Goal:** Wire everything together. Use factory in main.go, pass Claude config in dispatch, generalize stall timeout to read from active backend config.

**Depends on:** STEP-2, STEP-3, STEP-4

---

## Files to Modify

### `cmd/symphony/main.go`

**Replace hardcoded GeminiRunner with factory:**

Before:
```go
geminiRunner := agent.NewGeminiRunner()
orch := orchestrator.New(resolved, wf, linearClient, geminiRunner, workspaceMgr)
```

After:
```go
launcher, err := agent.NewLauncher(resolved.Backend)
if err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
}
orch := orchestrator.New(resolved, wf, linearClient, launcher, workspaceMgr)
```

**Update startup log to show backend kind:**
```go
slog.Info("symphony-go starting",
    "version", version,
    "backend", resolved.Backend,
    "tracker", resolved.Tracker.Kind,
    // ...
)
```

**Remove `gemini_command` and `gemini_model` from startup log.** Replace with:
```go
"agent_model", agentModel,
"agent_command", agentCommand,
```
Where `agentModel` and `agentCommand` are derived from `resolved.Backend`:
```go
agentModel := resolved.Gemini.Model
agentCommand := resolved.Gemini.Command
if resolved.Backend == "claude" {
    agentModel = resolved.Claude.Model
    agentCommand = resolved.Claude.Command
}
```

### `internal/orchestrator/orchestrator.go`

**In `New()`:**

Set state fields based on backend:
```go
if cfg.Backend == "claude" {
    state.AgentModel = cfg.Claude.Model
    state.AgentCommand = cfg.Claude.Command
    state.BackendKind = "claude"
} else {
    state.AgentModel = cfg.Gemini.Model
    state.AgentCommand = cfg.Gemini.Command
    state.BackendKind = "gemini"
}
```

**In `dispatchIssue()`:**

Add Claude config to RunParams:
```go
claudeCfg := cfg.Claude

params := agent.RunParams{
    // ... existing fields ...
    GeminiCfg:     &geminiCfg,
    ClaudeCfg:     &claudeCfg,   // NEW
    // ...
}
```

**In `applyReload()`:**

Update to set agent model/command based on backend:
```go
if reload.Config.Backend == "claude" {
    o.state.AgentModel = reload.Config.Claude.Model
    o.state.AgentCommand = reload.Config.Claude.Command
} else {
    o.state.AgentModel = reload.Config.Gemini.Model
    o.state.AgentCommand = reload.Config.Gemini.Command
}
```

Log warning if backend changed on reload:
```go
if reload.Config.Backend != o.state.BackendKind {
    slog.Warn("backend changed on reload — restart required for this to take effect",
        "current", o.state.BackendKind,
        "new", reload.Config.Backend,
    )
}
```

### `internal/orchestrator/reconcile.go`

**Generalize stall timeout in `reconcileStalls()`:**

Before:
```go
if cfg.Gemini.StallTimeoutMs <= 0 {
    return
}
stallTimeout := time.Duration(cfg.Gemini.StallTimeoutMs) * time.Millisecond
```

After:
```go
stallTimeoutMs := cfg.Gemini.StallTimeoutMs
if cfg.Backend == "claude" {
    stallTimeoutMs = cfg.Claude.StallTimeoutMs
}
if stallTimeoutMs <= 0 {
    return
}
stallTimeout := time.Duration(stallTimeoutMs) * time.Millisecond
```

---

## Smoke Test

After wiring, do a manual verification:

1. Create a WORKFLOW.md with `backend: gemini` → existing behavior unchanged.
2. Create a WORKFLOW.md with `backend: claude` → ClaudeRunner is used.
3. Create a WORKFLOW.md with no `backend` → defaults to gemini.
4. Create a WORKFLOW.md with `backend: invalid` → startup validation error.

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (all tests)
- [ ] `main.go` uses `NewLauncher()` factory
- [ ] `dispatchIssue()` passes `ClaudeCfg` in RunParams
- [ ] Stall timeout reads from active backend config
- [ ] Startup log shows backend kind
- [ ] Hot-reload warns on backend change
- [ ] No hardcoded `NewGeminiRunner()` in main.go
