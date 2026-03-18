# STEP-2: Config & Factory

**Goal:** Add `Backend` field, `ClaudeConfig` struct, defaults, validation, config parsing, and `NewLauncher()` factory.

---

## Files to Modify

### `internal/config/config.go`

1. **Add `ClaudeConfig` struct** (after `GeminiConfig`):
   ```go
   type ClaudeConfig struct {
       Command        string   `yaml:"command"          json:"command"`
       Model          string   `yaml:"model"            json:"model"`
       PermissionMode string   `yaml:"permission_mode"  json:"permission_mode"`
       AllowedTools   []string `yaml:"allowed_tools"    json:"allowed_tools"`
       MaxTurns       int      `yaml:"max_turns"        json:"max_turns"`
       TurnTimeoutMs  int      `yaml:"turn_timeout_ms"  json:"turn_timeout_ms"`
       StallTimeoutMs int      `yaml:"stall_timeout_ms" json:"stall_timeout_ms"`
   }
   ```

2. **Add fields to `Config` struct:**
   ```go
   Backend string       `yaml:"backend"  json:"backend"`
   Claude  ClaudeConfig `yaml:"claude"   json:"claude"`
   ```

3. **Update `ParseConfig`:**
   - Add alias: if raw has `"claude_code"` but not `"claude"`, rename it.
   - No other changes needed (yaml.Unmarshal handles the new fields).

4. **Update `applyDefaults`:**
   - Add `claudeRaw, _ := raw["claude"].(map[string]any)` extraction.
   - Add defaults block for Claude fields (same pattern as Gemini block).
   - Add backend default: if raw has no `"backend"` key, set `cfg.Backend = defaults.Backend`.

### `internal/config/defaults.go`

Add to `DefaultConfig()`:
```go
Backend: "gemini",
Claude: ClaudeConfig{
    Command:        "claude",
    Model:          "claude-sonnet-4-6",
    PermissionMode: "bypassPermissions",
    AllowedTools:   []string{"Read", "Write", "Edit", "Bash"},
    MaxTurns:       25,
    TurnTimeoutMs:  600000,
    StallTimeoutMs: 300000,
},
```

### `internal/config/validate.go`

Update `ValidateDispatchConfig`:
- Add backend validation:
  ```go
  switch cfg.Backend {
  case "", "gemini":
      // existing gemini.command check
  case "claude":
      if cfg.Claude.Command == "" {
          errs = append(errs, "claude.command is required when backend is \"claude\"")
      }
  default:
      errs = append(errs, fmt.Sprintf("unsupported backend: %q", cfg.Backend))
  }
  ```
- Make the `gemini.command` check conditional on backend being gemini (or empty).

---

## Files to Modify (Agent Package)

### `internal/agent/runner.go`

1. **Add `ClaudeCfg` to `RunParams`:**
   ```go
   ClaudeCfg *config.ClaudeConfig
   ```

2. **Add `NewLauncher()` factory function:**
   ```go
   func NewLauncher(backend string) (AgentLauncher, error) {
       switch backend {
       case "", "gemini":
           return NewGeminiRunner(), nil
       case "claude":
           return NewClaudeRunner(), nil
       default:
           return nil, fmt.Errorf("unsupported backend: %q", backend)
       }
   }
   ```

3. **Add placeholder `ClaudeRunner`** (will be fully implemented in STEP-3):
   ```go
   type ClaudeRunner struct{}

   func NewClaudeRunner() *ClaudeRunner {
       return &ClaudeRunner{}
   }

   func (r *ClaudeRunner) Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error {
       return fmt.Errorf("claude runner not yet implemented")
   }
   ```

---

## Tests

### `internal/config/config_test.go` (add tests, don't overwrite existing)

| Test | What |
|---|---|
| `TestParseConfig_BackendDefault` | Omitted backend → "gemini" |
| `TestParseConfig_BackendClaude` | `backend: claude` parsed correctly |
| `TestParseConfig_ClaudeDefaults` | Missing `claude` section → correct defaults |
| `TestParseConfig_ClaudeOverrides` | Partial `claude` section merges with defaults |
| `TestParseConfig_ClaudeCodeAlias` | `claude_code` key aliased to `claude` |
| `TestValidateDispatchConfig_InvalidBackend` | Unknown backend → error |
| `TestValidateDispatchConfig_ClaudeEmptyCommand` | backend=claude with empty command → error |

### `internal/agent/runner_test.go` (add tests)

| Test | What |
|---|---|
| `TestNewLauncher_Gemini` | "gemini" → GeminiRunner |
| `TestNewLauncher_Claude` | "claude" → ClaudeRunner |
| `TestNewLauncher_Empty` | "" → GeminiRunner |
| `TestNewLauncher_Invalid` | "unknown" → error |

---

## DoD

- [ ] `go build ./...` passes
- [ ] `go test ./internal/config/...` passes (existing + new tests)
- [ ] `go test ./internal/agent/ -run TestNewLauncher` passes
- [ ] Existing tests still pass (no regressions)
- [ ] `ClaudeRunner.Launch()` returns "not yet implemented" error (placeholder)
