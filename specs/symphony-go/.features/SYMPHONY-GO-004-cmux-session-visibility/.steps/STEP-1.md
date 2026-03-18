# Step 1: Config + cmux Manager Foundation

**Covers:** FR-4 (Configuration), FR-1 (partial — Manager struct)
**Packages:** `internal/config`, `internal/cmux`

---

## 1. Tasks

### 1.1 CmuxConfig struct and defaults

- [ ] Add `CmuxConfig` struct to `internal/config/config.go`:
  ```go
  type CmuxConfig struct {
      Enabled       bool   `yaml:"enabled"        json:"enabled"`
      WorkspaceName string `yaml:"workspace_name" json:"workspace_name"`
      CloseDelayMs  int    `yaml:"close_delay_ms" json:"close_delay_ms"`
  }
  ```
- [ ] Add `Cmux CmuxConfig` field to `Config` struct (after `Server` field)
- [ ] Add defaults in `internal/config/defaults.go`:
  ```go
  Cmux: CmuxConfig{
      Enabled:       false,
      WorkspaceName: "Symphony",
      CloseDelayMs:  30000,
  },
  ```
- [ ] Add `applyDefaults` block in `internal/config/config.go` for cmux section (same pattern as existing sections):
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

### 1.2 Config tests

- [ ] Add `TestParseCmuxConfig` to `internal/config/config_test.go` — parse a raw map with cmux section, verify all fields
- [ ] Add `TestCmuxDefaults` — verify defaults when cmux section omitted entirely
- [ ] Add `TestCmuxDefaultsPartial` — verify unset fields keep defaults when cmux section partially specified

### 1.3 Manager struct skeleton

- [ ] Create `internal/cmux/manager.go` with:
  ```go
  type Manager struct {
      enabled       bool
      workspaceName string
      closeDelayMs  int
      cmuxBin       string
      workspaceRef  string
      surfaces      map[string]string   // issueID → surface ref
      logFiles      map[string]*os.File // issueID → log file handle
      mu            sync.Mutex
  }
  ```
- [ ] Implement `New(cfg *config.CmuxConfig) *Manager`:
  - If `cfg == nil` or `!cfg.Enabled`, return manager with `enabled=false`
  - If enabled, attempt to find cmux binary: check `exec.LookPath("cmux")`, then check `/Applications/cmux.app/Contents/Resources/bin/cmux`
  - If binary not found, log warning, set `enabled=false`
  - Initialize `surfaces` and `logFiles` maps
- [ ] Implement stub methods that check `m.enabled` and return early if false:
  - `Init() error` — returns nil
  - `CreateSurface(issueID, identifier, workspacePath string) error` — returns nil
  - `WriteEvent(issueID string, content string)` — no-op
  - `WriteAnnotation(issueID string, message string)` — no-op
  - `LogWriter(issueID string) io.Writer` — returns `io.Discard`
  - `CloseSurface(issueID string)` — no-op
  - `Shutdown()` — no-op

### 1.4 Manager tests

- [ ] Create `internal/cmux/manager_test.go` with:
  - `TestNewDisabled` — `New(&CmuxConfig{Enabled: false})` returns manager where all methods are no-ops, no panics
  - `TestNewNilConfig` — `New(nil)` returns disabled manager
  - `TestNewBinaryNotFound` — `New(&CmuxConfig{Enabled: true})` with no cmux in PATH returns disabled manager
  - `TestLogWriterDisabled` — disabled manager returns `io.Discard`

## 2. Definition of Done

- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/config/... ./internal/cmux/...` passes
- [ ] CmuxConfig round-trips through YAML parse correctly
- [ ] Disabled manager is completely safe (no panics, no file I/O, no subprocess calls)
