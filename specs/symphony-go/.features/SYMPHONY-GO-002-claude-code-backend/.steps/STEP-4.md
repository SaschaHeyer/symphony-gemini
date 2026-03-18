# STEP-4: Backend-Agnostic State Renames

**Goal:** Rename all Gemini-specific field names in orchestrator state, snapshots, and dashboard to be backend-neutral. Add `BackendKind` field.

**Independent of:** STEP-1, STEP-2, STEP-3 (pure refactor, no new logic)

---

## Files to Modify

### `internal/orchestrator/state.go`

**State struct renames:**
```
GeminiTotals  → AgentTotals
GeminiModel   → AgentModel
GeminiCommand → AgentCommand
```

**RunningEntry rename:**
```
GeminiPID → AgentPID
```

**New field in State:**
```go
BackendKind string  // "gemini" or "claude"
```

**ConfigSnapshot renames:**
```
GeminiModel   → AgentModel    (json:"agent_model")
GeminiCommand → AgentCommand  (json:"agent_command")
```

**New field in ConfigSnapshot:**
```go
BackendKind string `json:"backend_kind"`
```

**StateSnapshot rename:**
```
GeminiTotals → AgentTotals  (json:"agent_totals")
```

**Update `NewState()`:**
- No GeminiTotals initialization needed (zero value is fine).

**Update `Snapshot()`:**
- Replace all `s.GeminiTotals` → `s.AgentTotals`.
- Replace `s.GeminiModel` → `s.AgentModel`.
- Replace `s.GeminiCommand` → `s.AgentCommand`.
- Add `BackendKind: s.BackendKind` to ConfigSnapshot.

### `internal/orchestrator/orchestrator.go`

**In `New()`:**
```go
state.AgentModel = cfg.Gemini.Model     // will be updated in STEP-5 for claude
state.AgentCommand = cfg.Gemini.Command  // will be updated in STEP-5 for claude
```

**In `applyReload()`:**
```go
o.state.AgentModel = reload.Config.Gemini.Model
o.state.AgentCommand = reload.Config.Gemini.Command
```

### `internal/orchestrator/reconcile.go`

**In `removeRunning()`:**
```go
state.AgentTotals.SecondsRunning += elapsed  // was GeminiTotals
```

**In `reconcileStalls()`:**
```go
cfg.Gemini.StallTimeoutMs  // leave as-is for now, STEP-5 will generalize
```

### `internal/orchestrator/metrics.go`

**In `UpdateTokens()`:**
```go
state.AgentTotals.InputTokens += deltaInput    // was GeminiTotals
state.AgentTotals.OutputTokens += deltaOutput   // was GeminiTotals
state.AgentTotals.TotalTokens += deltaTotal     // was GeminiTotals
```

### `internal/server/dashboard.go`

Update template references:
```
snapshot.Config.GeminiModel → snapshot.Config.AgentModel
snapshot.GeminiTotals → snapshot.AgentTotals
```

---

## Verification Strategy

This is a mechanical rename. Use the following approach:
1. Do all renames.
2. Run `go build ./...` — fix any compilation errors (catch any missed references).
3. Run `go test ./...` — all existing tests must pass.

**No new tests needed** — existing tests cover the renamed fields. If any test references `GeminiTotals` etc., update the test too.

---

## DoD

- [ ] All references to `GeminiTotals`, `GeminiModel`, `GeminiCommand`, `GeminiPID` are renamed
- [ ] `BackendKind` field added to State and ConfigSnapshot
- [ ] JSON keys updated: `agent_model`, `agent_command`, `agent_totals`, `backend_kind`
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (all existing tests)
- [ ] `grep -r "GeminiTotals\|GeminiModel\|GeminiCommand\|GeminiPID" internal/` returns nothing
