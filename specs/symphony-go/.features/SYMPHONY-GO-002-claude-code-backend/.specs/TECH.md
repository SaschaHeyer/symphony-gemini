# TECH SPEC: Claude Code Backend for Symphony Go

**Status:** Draft v1
**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-002

---

## 1. Architecture Overview

```
main.go
  │
  ├─ config.ParseConfig()       ── reads backend + claude section
  │
  ├─ agent.NewLauncher(backend)  ── factory returns ClaudeRunner or GeminiRunner
  │
  └─ orchestrator.New(cfg, wf, tracker, launcher, wsMgr)
       │
       └─ dispatchIssue() ── calls launcher.Launch()
            │
            ├─ [backend=gemini] GeminiRunner.Launch()  ── ACP JSON-RPC (unchanged)
            │
            └─ [backend=claude] ClaudeRunner.Launch()
                 │
                 ├─ Workspace lifecycle (shared with Gemini)
                 ├─ Turn loop: spawn claude -p per turn
                 │    ├─ PTY wrapper: script -q /dev/null
                 │    ├─ NDJSON parser: real-time stdout parsing
                 │    ├─ Session persistence: .symphony-session-id
                 │    └─ Event emission: OrchestratorEvent channel
                 └─ Cleanup
```

---

## 2. New Files

| File | Purpose |
|------|---------|
| `internal/agent/claude_runner.go` | `ClaudeRunner` struct implementing `AgentLauncher` |
| `internal/agent/ndjson.go` | `NdjsonParser` stateful line-accumulator + event classifier |
| `internal/agent/claude_runner_test.go` | Unit tests for ClaudeRunner |
| `internal/agent/ndjson_test.go` | Unit tests for NdjsonParser |

## 3. Modified Files

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add `Backend` field, `ClaudeConfig` struct, update `ParseConfig` + `applyDefaults` |
| `internal/config/defaults.go` | Add `ClaudeConfig` defaults, `Backend: "gemini"` default |
| `internal/config/validate.go` | Validate `Backend` field |
| `internal/agent/runner.go` | Add `ClaudeCfg` to `RunParams`, add `NewLauncher()` factory |
| `internal/orchestrator/state.go` | Rename Gemini-specific fields to backend-agnostic names |
| `internal/orchestrator/orchestrator.go` | Use renamed state fields, pass `ClaudeCfg` in dispatch |
| `internal/server/server.go` | Update JSON keys for renamed snapshot fields |
| `cmd/symphony/main.go` | Use `NewLauncher()` factory, log backend kind |

---

## 4. Data Models

### 4.1 ClaudeConfig

```go
// internal/config/config.go

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

### 4.2 Updated Config struct

```go
type Config struct {
    Backend   string          `yaml:"backend"   json:"backend"`     // NEW
    Tracker   TrackerConfig   `yaml:"tracker"   json:"tracker"`
    Polling   PollingConfig   `yaml:"polling"   json:"polling"`
    Workspace WorkspaceConfig `yaml:"workspace" json:"workspace"`
    Hooks     HooksConfig     `yaml:"hooks"     json:"hooks"`
    Agent     AgentConfig     `yaml:"agent"     json:"agent"`
    Gemini    GeminiConfig    `yaml:"gemini"    json:"gemini"`
    Claude    ClaudeConfig    `yaml:"claude"    json:"claude"`      // NEW
    Server    ServerConfig    `yaml:"server"    json:"server"`
}
```

### 4.3 Updated RunParams

```go
// internal/agent/runner.go

type RunParams struct {
    Issue           *tracker.Issue
    Attempt         *int
    Workflow        *workflow.WorkflowDefinition
    GeminiCfg       *config.GeminiConfig   // used by GeminiRunner
    ClaudeCfg       *config.ClaudeConfig   // used by ClaudeRunner
    AgentCfg        *config.AgentConfig
    ActiveStates    []string
    WorkspaceMgr    *workspace.Manager
    WorkspaceRoot   string
    ExtraEnv        []string
    CheckIssueState func(ctx context.Context, issueID string) (string, error)
}
```

### 4.4 NdjsonParser

```go
// internal/agent/ndjson.go

// NdjsonEvent represents a parsed event from Claude Code's stream-json output.
type NdjsonEvent struct {
    Type    string         // "system", "assistant", "result", "user", etc.
    Subtype string         // "init", "success", "error", etc.
    Raw     map[string]any // Full decoded JSON object
}

// NdjsonParser is a stateful line-accumulator for NDJSON streams.
type NdjsonParser struct {
    buffer []byte
}

func NewNdjsonParser() *NdjsonParser
func (p *NdjsonParser) Feed(data []byte) []NdjsonEvent
func (p *NdjsonParser) Flush() []NdjsonEvent
```

### 4.5 Renamed State Fields

```go
// internal/orchestrator/state.go — field renames

State.GeminiTotals  → State.AgentTotals
State.GeminiModel   → State.AgentModel
State.GeminiCommand → State.AgentCommand

// New field:
State.BackendKind string  // "gemini" or "claude"

// RunningEntry:
RunningEntry.GeminiPID → RunningEntry.AgentPID

// StateSnapshot JSON keys:
"gemini_model"   → "agent_model"
"gemini_command"  → "agent_command"
"codex_totals"    → "agent_totals"
```

---

## 5. ClaudeRunner Implementation Detail

### 5.1 Launch Flow

```
ClaudeRunner.Launch(ctx, params, eventCh):
  1. Create/reuse workspace (same as GeminiRunner)
  2. Validate workspace path
  3. Run before_run hook
  4. Read session ID from <workspace>/.symphony-session-id (may be empty)
  5. Turn loop (1..params.AgentCfg.MaxTurns):
     a. Check ctx.Done()
     b. Build prompt (full on turn 1, continuation on turn N)
     c. Build CLI args
     d. Spawn subprocess with PTY wrapper
     e. Parse NDJSON stdout in real-time, emit events
     f. Wait for process exit
     g. Extract & persist session ID if found
     h. Check result event type (success/error)
     i. Re-check issue state if not last turn
  6. Run after_run hook
```

### 5.2 CLI Argument Construction

```go
func buildClaudeArgs(cfg *config.ClaudeConfig, prompt string, sessionID string, workspace string) []string {
    args := []string{
        "-p", prompt,
        "--output-format", "stream-json",
        "--max-turns", strconv.Itoa(cfg.MaxTurns),
        "--model", cfg.Model,
    }

    if sessionID != "" {
        args = append(args, "--resume", sessionID)
    }

    if cfg.PermissionMode != "" {
        args = append(args, "--permission-mode", cfg.PermissionMode)
    }

    for _, tool := range cfg.AllowedTools {
        args = append(args, "--allowedTools", tool)
    }

    mcpPath := filepath.Join(workspace, ".mcp.json")
    if _, err := os.Stat(mcpPath); err == nil {
        args = append(args, "--mcp-config", mcpPath)
    }

    return args
}
```

### 5.3 PTY Wrapper (script -q /dev/null)

```go
func spawnWithPTY(executable string, args []string, cwd string, extraEnv []string) (*exec.Cmd, io.ReadCloser, error) {
    scriptPath, err := exec.LookPath("script")
    if err != nil {
        // Fallback: direct execution (may not produce stream-json output)
        slog.Warn("'script' not found; Claude Code may not produce output without TTY")
        cmd := exec.Command(executable, args...)
        cmd.Dir = cwd
        if len(extraEnv) > 0 {
            cmd.Env = append(os.Environ(), extraEnv...)
        }
        stdout, _ := cmd.StdoutPipe()
        return cmd, stdout, nil
    }

    // Build shell command string for script wrapper
    fullCmd := shellJoin(append([]string{executable}, args...))

    cmd := exec.Command(scriptPath, "-q", "/dev/null", "/bin/sh", "-c", fullCmd)
    cmd.Dir = cwd
    if len(extraEnv) > 0 {
        cmd.Env = append(os.Environ(), extraEnv...)
    }

    stdout, _ := cmd.StdoutPipe()
    return cmd, stdout, nil
}
```

### 5.4 NDJSON Event-to-OrchestratorEvent Mapping

```go
func (r *ClaudeRunner) mapNdjsonToAgentEvent(evt NdjsonEvent, sessionID string) AgentEvent {
    agentEvt := AgentEvent{
        Timestamp: time.Now().UTC(),
        SessionID: sessionID,
    }

    switch evt.Type {
    case "system":
        if evt.Subtype == "init" {
            agentEvt.Type = EventSessionStarted
            // Extract session_id from evt.Raw
        } else {
            agentEvt.Type = EventNotification
        }

    case "assistant":
        // Check if any content block has type "tool_use"
        if hasToolUse(evt.Raw) {
            agentEvt.Type = EventToolCall
            agentEvt.Message = extractToolCallSummary(evt.Raw)
        } else {
            agentEvt.Type = EventNotification
            agentEvt.Message = extractAssistantText(evt.Raw)
        }

    case "result":
        if evt.Subtype == "success" {
            agentEvt.Type = EventTurnCompleted
        } else if evt.Subtype == "error" {
            agentEvt.Type = EventTurnFailed
        } else {
            // error_max_turns, error_tool, etc. — turn IS done
            agentEvt.Type = EventTurnCompleted
        }
        agentEvt.Usage = extractUsageFromResult(evt.Raw)
        agentEvt.Message = evt.Subtype

    default:
        agentEvt.Type = EventNotification
    }

    return agentEvt
}
```

### 5.5 Session ID Persistence

```go
const sessionIDFile = ".symphony-session-id"

func readSessionID(workspace string) string {
    data, err := os.ReadFile(filepath.Join(workspace, sessionIDFile))
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(data))
}

func writeSessionID(workspace string, sessionID string) error {
    return os.WriteFile(
        filepath.Join(workspace, sessionIDFile),
        []byte(sessionID+"\n"),
        0644,
    )
}
```

### 5.6 Token Usage Extraction

Claude Code `result` events include usage data:

```json
{
  "type": "result",
  "subtype": "success",
  "usage": {
    "input_tokens": 15234,
    "output_tokens": 3421
  },
  "cost_usd": 0.042,
  "session_id": "abc-123"
}
```

```go
func extractUsageFromResult(raw map[string]any) *TokenUsage {
    usage, ok := raw["usage"].(map[string]any)
    if !ok {
        return nil
    }
    return extractTokenUsageFromMap(usage) // reuse existing function from acp.go
}
```

---

## 6. Config Parsing Updates

### 6.1 ParseConfig Changes

```go
// In ParseConfig, after existing codex→gemini alias:

// Also alias: if raw has "claude_code" but not "claude", rename it
if _, hasClaude := raw["claude"]; !hasClaude {
    if ccVal, hasCC := raw["claude_code"]; hasCC {
        raw["claude"] = ccVal
        delete(raw, "claude_code")
    }
}
```

### 6.2 Default Values

```go
// In DefaultConfig():
Claude: ClaudeConfig{
    Command:        "claude",
    Model:          "claude-sonnet-4-6",
    PermissionMode: "bypassPermissions",
    AllowedTools:   []string{"Read", "Write", "Edit", "Bash"},
    MaxTurns:       25,
    TurnTimeoutMs:  600000,   // 10 minutes
    StallTimeoutMs: 300000,   // 5 minutes
},
```

### 6.3 Validation

```go
// In ValidateDispatchConfig or a new validateBackend:
func validateBackend(cfg *Config) error {
    switch cfg.Backend {
    case "", "gemini":
        return nil
    case "claude":
        return nil
    default:
        return fmt.Errorf("unsupported backend: %q", cfg.Backend)
    }
}
```

---

## 7. Factory Pattern

```go
// internal/agent/runner.go

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

---

## 8. Orchestrator Dispatch Updates

```go
// internal/orchestrator/orchestrator.go — dispatchIssue()

// Replace:
//   geminiCfg := cfg.Gemini
// With:
geminiCfg := cfg.Gemini
claudeCfg := cfg.Claude

// In RunParams construction:
params := agent.RunParams{
    // ... existing fields ...
    GeminiCfg: &geminiCfg,
    ClaudeCfg: &claudeCfg,   // NEW
}
```

---

## 9. Error Handling

| Scenario | Behavior |
|----------|----------|
| `claude` binary not on PATH | `Launch()` returns error: `"executable not found: claude"` |
| `script` binary not found | Warning log, fallback to direct execution |
| Claude process exits non-zero | Turn failure, retry with backoff |
| NDJSON parse error (malformed line) | Emit `malformed` event, continue parsing |
| No `result` event before process exit | Treat as completed (warn) |
| `.symphony-session-id` missing on turn > 1 | Warning, start fresh session |
| Turn timeout | Kill process, return timeout error |
| Stall (no output within stall_timeout_ms) | Handled by orchestrator reconciliation (same as Gemini) |

---

## 10. Backward Compatibility

- Default `backend` = `"gemini"` → existing users are unaffected.
- All Gemini-specific code paths remain intact.
- State field renames (GeminiTotals → AgentTotals) change JSON API keys — this is a **breaking change** for API consumers parsing `codex_totals` or `gemini_model`. Since this is a pre-1.0 project, this is acceptable.
- The `codex` → `gemini` alias in config parsing remains.
- New `claude_code` → `claude` alias added for config key flexibility.
