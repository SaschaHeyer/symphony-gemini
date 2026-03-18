# STEP-1: Config Updates for Jira

**Goal:** Add `Email` field, validate Jira-specific requirements, resolve Jira env vars.

---

## Files to Modify

### `internal/config/config.go`

Add `Email` field to `TrackerConfig`:
```go
Email string `yaml:"email" json:"email"`
```

No changes to `ParseConfig` or `applyDefaults` needed — yaml.Unmarshal handles the new field, and empty string is a fine default.

### `internal/config/validate.go`

Update `ValidateDispatchConfig` to handle `kind: "jira"`:

```go
switch cfg.Tracker.Kind {
case "linear":
    if cfg.Tracker.ProjectSlug == "" {
        errs = append(errs, `tracker.project_slug is required when tracker.kind is "linear"`)
    }
case "jira":
    if cfg.Tracker.Endpoint == "" {
        errs = append(errs, `tracker.endpoint is required when tracker.kind is "jira"`)
    }
    if cfg.Tracker.ProjectSlug == "" {
        errs = append(errs, `tracker.project_slug is required when tracker.kind is "jira"`)
    }
    if cfg.Tracker.Email == "" {
        errs = append(errs, `tracker.email is required when tracker.kind is "jira"`)
    }
default:
    errs = append(errs, fmt.Sprintf("tracker.kind %q is not supported", cfg.Tracker.Kind))
}
```

### `internal/config/resolve.go`

Add to `ResolveConfig()`:

```go
// Resolve $VAR in tracker.email
resolved.Tracker.Email = resolveEnvVar(resolved.Tracker.Email)

// Jira-specific env fallbacks
if resolved.Tracker.Kind == "jira" {
    if resolved.Tracker.Email == "" {
        resolved.Tracker.Email = os.Getenv("JIRA_EMAIL")
    }
    if resolved.Tracker.APIKey == "" {
        resolved.Tracker.APIKey = os.Getenv("JIRA_API_TOKEN")
    }
}
```

---

## Tests

### `internal/config/validate_test.go` (add)

| Test | What |
|------|------|
| `TestValidateDispatchConfig_JiraValid` | All Jira fields present → no error |
| `TestValidateDispatchConfig_JiraMissingEmail` | Missing email → error |
| `TestValidateDispatchConfig_JiraMissingEndpoint` | Missing endpoint → error |
| `TestValidateDispatchConfig_JiraMissingProjectSlug` | Missing project_slug → error |

### `internal/config/config_test.go` (add)

| Test | What |
|------|------|
| `TestParseConfig_TrackerEmail` | Email field parsed from raw config |

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./internal/config/...` — new tests pass
- [ ] Existing tests unaffected
