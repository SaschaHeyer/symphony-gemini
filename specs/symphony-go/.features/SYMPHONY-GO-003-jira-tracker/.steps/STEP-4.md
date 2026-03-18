# STEP-4: main.go Integration & README

**Goal:** Wire the tracker factory into main.go and document Jira support in README.

**Depends on:** STEP-3

---

## Files to Modify

### `cmd/symphony/main.go`

Replace:
```go
linearClient := tracker.NewLinearClient(resolved.Tracker.Endpoint, resolved.Tracker.APIKey)
```

With:
```go
trackerClient, err := tracker.NewTrackerClient(&resolved.Tracker)
if err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
}
```

Update orchestrator constructor:
```go
orch := orchestrator.New(resolved, wf, trackerClient, launcher, workspaceMgr)
```

Update startup log to show tracker kind:
```go
slog.Info("symphony-go starting",
    // ... existing fields ...
    "tracker_kind", resolved.Tracker.Kind,
)
```

### `README.md`

Add Jira to the Prerequisites section alongside Linear.

Add a Jira example to the Configuration section showing a minimal WORKFLOW.md with `tracker.kind: jira`.

Update the tracker config reference table to document the `email` field and Jira-specific defaults/requirements.

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `main.go` uses `NewTrackerClient()` factory
- [ ] No hardcoded `NewLinearClient()` in main.go
- [ ] README documents Jira setup
