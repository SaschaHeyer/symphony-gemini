# Step 1: Project Scaffold + Workflow Loader + Config Layer

**Covers:** FR-1 (Workflow Loader), FR-2 (Configuration Layer)
**Packages:** `cmd/symphony`, `internal/workflow`, `internal/config`, `internal/logging`

---

## 1. Tasks

### 1.1 Project Scaffold

- [ ] Create `go/` directory at repo root (sibling to `elixir/`)
- [ ] `go mod init` with appropriate module path
- [ ] Create directory structure per TECH.md Section 1
- [ ] `cmd/symphony/main.go` — minimal `func main()` placeholder that prints version and exits
- [ ] `Makefile` with `build`, `test`, `run` targets
- [ ] `go mod tidy` to verify clean module

### 1.2 Logging Setup

- [ ] `internal/logging/logging.go`:
  - `Setup()` function that configures `slog` default logger with JSON handler to stderr
  - Helper to create loggers with issue/session context fields: `WithIssue(logger, id, identifier)`, `WithSession(logger, sessionID)`

### 1.3 Workflow Loader

- [ ] `internal/workflow/loader.go`:
  - `LoadWorkflow(path string) (*WorkflowDefinition, error)`
  - Split on `---` delimiters: first occurrence starts front matter, second closes it
  - Parse YAML between delimiters using `gopkg.in/yaml.v3` to `map[string]any`
  - Remaining content after second `---` is prompt body, trimmed
  - No `---` → entire file is prompt body, empty config map
  - Return typed errors: `ErrMissingWorkflowFile`, `ErrWorkflowParseError`, `ErrFrontMatterNotMap`

- [ ] `internal/workflow/loader_test.go`:
  - Test: valid YAML + prompt body
  - Test: empty front matter
  - Test: no delimiters → full body
  - Test: invalid YAML → `ErrWorkflowParseError`
  - Test: non-map YAML (e.g., list) → `ErrFrontMatterNotMap`
  - Test: missing file → `ErrMissingWorkflowFile`
  - Test: prompt body trimmed
  - Test: unknown keys preserved

### 1.4 Config Resolution

- [ ] `internal/config/defaults.go`:
  - `DefaultConfig()` returning `Config` struct with all defaults per SPEC Section 6.4
  - Use `os.TempDir()` for workspace root default

- [ ] `internal/config/config.go`:
  - `Config` struct and all sub-structs per TECH.md Section 2.3
  - `ParseConfig(raw map[string]any) (*Config, error)`:
    - Marshal raw map to YAML bytes, then unmarshal onto a `Config` struct pre-filled with defaults
    - Handle `codex` → `gemini` key aliasing: if raw map has `codex` key and no `gemini` key, rename it

- [ ] `internal/config/resolve.go`:
  - `ResolveConfig(cfg *Config) (*Config, error)`:
    - Resolve `$VAR_NAME` in `cfg.Tracker.APIKey` from `os.Getenv`
    - Resolve `$VAR_NAME` in `cfg.Workspace.Root` from `os.Getenv`
    - Expand `~` to `os.UserHomeDir()` in path fields (`workspace.root`)
    - Coerce `polling.interval_ms` etc. from string to int (YAML might produce strings)
    - Normalize `per_state_concurrency` keys to lowercase, drop non-positive values
    - If `$VAR` resolves to empty → treat as missing (leave field empty for validation to catch)

- [ ] `internal/config/resolve_test.go`:
  - Test: `$VAR` resolves from env
  - Test: `$EMPTY_VAR` treated as missing
  - Test: `~` expands
  - Test: string int coercion
  - Test: per-state key normalization
  - Test: codex → gemini alias

- [ ] `internal/config/validate.go`:
  - `ValidateDispatchConfig(cfg *Config) error`:
    - `tracker.kind` present and == `"linear"`
    - `tracker.api_key` non-empty
    - `tracker.project_slug` non-empty
    - `gemini.command` non-empty

- [ ] `internal/config/validate_test.go`:
  - Test: valid config passes
  - Test: each missing required field fails with descriptive error

### 1.5 Workflow Watcher

- [ ] `internal/workflow/watcher.go`:
  - `WatchWorkflow(path string, onChange func(*WorkflowDefinition, *Config)) (stop func(), err error)`
  - Use `fsnotify` to watch the workflow file
  - On change: reload → parse → resolve config → call `onChange` callback
  - On parse/resolve error: log error, do NOT call `onChange` (keeps last good config)
  - Debounce rapid changes (100ms)

- [ ] No test for watcher in this step (filesystem watch is integration-level; tested in Step 7)

---

## 2. Dependencies to Add

```
gopkg.in/yaml.v3
github.com/fsnotify/fsnotify
```

---

## 3. Definition of Done

- [ ] `go build ./cmd/symphony` succeeds
- [ ] `go test ./internal/workflow/... ./internal/config/...` — all pass
- [ ] Workflow loader correctly parses a sample WORKFLOW.md file
- [ ] Config resolution handles $VAR, ~, defaults, codex alias
- [ ] Config validation catches all required-field violations
