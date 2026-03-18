# STEP-3: Tracker Factory

**Goal:** Add `NewTrackerClient()` factory to tracker package.

**Depends on:** STEP-1 (config Email field), STEP-2 (JiraClient)

---

## Files to Modify

### `internal/tracker/issue.go`

Add factory function after the interface definition:

```go
func NewTrackerClient(cfg *config.TrackerConfig) (TrackerClient, error) {
    switch cfg.Kind {
    case "linear":
        return NewLinearClient(cfg.Endpoint, cfg.APIKey), nil
    case "jira":
        return NewJiraClient(cfg.Endpoint, cfg.Email, cfg.APIKey), nil
    default:
        return nil, &TrackerError{
            Kind:    ErrUnsupportedTrackerKind,
            Message: fmt.Sprintf("unsupported tracker kind: %q", cfg.Kind),
        }
    }
}
```

Add import for `config` package.

---

## Tests

### `internal/tracker/factory_test.go` (new file)

| Test | What |
|------|------|
| `TestNewTrackerClient_Linear` | kind=linear returns *LinearClient |
| `TestNewTrackerClient_Jira` | kind=jira returns *JiraClient |
| `TestNewTrackerClient_Unknown` | unknown → TrackerError with ErrUnsupportedTrackerKind |
| `TestNewTrackerClient_Empty` | empty kind → error |

Use type assertions to verify the returned client type.

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./internal/tracker/ -run TestNewTrackerClient` — all pass
- [ ] All existing tests still pass
