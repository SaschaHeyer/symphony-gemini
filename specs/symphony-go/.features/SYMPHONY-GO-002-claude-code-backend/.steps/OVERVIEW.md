# Implementation Plan: Claude Code Backend

**Feature:** SYMPHONY-GO-002
**Total Steps:** 5

Each step is a vertical slice — builds, tests pass, and delivers visible progress.

---

## Execution Checklist

- [x] **STEP-1**: NDJSON Parser — `internal/agent/ndjson.go` + tests
- [x] **STEP-2**: Config & Factory — `ClaudeConfig`, `Backend` field, `NewLauncher()`, validation, defaults + tests
- [x] **STEP-3**: Claude Runner — `internal/agent/claude_runner.go` (PTY wrapper, CLI args, session persistence, turn loop, NDJSON→event mapping) + tests
- [x] **STEP-4**: Backend-Agnostic State Renames — rename `GeminiTotals`/`GeminiModel`/`GeminiCommand`/`GeminiPID` → `AgentTotals`/`AgentModel`/`AgentCommand`/`AgentPID`, add `BackendKind`, update dashboard + API JSON keys
- [x] **STEP-5**: Orchestrator & main.go Integration — wire factory into main.go, pass `ClaudeCfg` in dispatch, update reconcile stall timeout to read from active backend config

---

## Dependency Order

```
STEP-1 (NdjsonParser)  ─┐
                         ├─► STEP-3 (ClaudeRunner)
STEP-2 (Config+Factory) ─┘         │
                                    │
STEP-4 (State Renames) ────────────►├─► STEP-5 (Integration)
```

STEP-1 and STEP-2 are independent (can be done in either order).
STEP-3 depends on both STEP-1 and STEP-2.
STEP-4 is independent of STEP-1/2/3 (pure rename refactor).
STEP-5 ties everything together.
