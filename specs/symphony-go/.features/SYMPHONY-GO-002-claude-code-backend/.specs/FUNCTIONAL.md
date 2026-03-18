# FUNCTIONAL SPEC: Claude Code Backend for Symphony Go

**Status:** Draft v1
**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-002
**Depends on:** SYMPHONY-GO-001

---

## 1. Overview

Add Claude Code as a second agent backend to symphony-go alongside the existing Gemini CLI (ACP) backend. The user selects which backend to use via a `backend` field in WORKFLOW.md config. Claude Code uses a fundamentally different protocol than Gemini: one-shot CLI invocations with NDJSON streaming output instead of a long-running ACP JSON-RPC process.

### 1.1 Key Differences: Gemini ACP vs Claude Code

| Aspect | Gemini (Current) | Claude Code (New) |
|---|---|---|
| Protocol | ACP — JSON-RPC 2.0 over stdio, long-running process | CLI invocation per turn, NDJSON stream output |
| Session continuity | Single process, `sessionId` within ACP | `--resume <session_id>` flag across invocations |
| Session persistence | In-process (ACP session) | `.symphony-session-id` file in workspace |
| Tool access | Client-side injection (fs/terminal via ACP requests) | MCP servers via `.mcp.json` in workspace |
| Permission handling | ACP `session/request_permission` auto-approve | `--permission-mode bypassPermissions` flag |
| Output format | JSON-RPC notifications (`session/update`) | NDJSON events (`--output-format stream-json`) |
| TTY requirement | None | Requires pseudo-TTY; use `script -q /dev/null` wrapper |
| Default model | `gemini-3.1-pro-preview` | `claude-sonnet-4-6` |

### 1.2 What This Is NOT

- Not replacing the Gemini backend — both coexist
- Not implementing budget/cost tracking (out of scope)
- Not implementing cmux visibility mode (future enhancement)

---

## 2. Goals

1. **Pluggable backend selection** — Introduce a `backend` config field so WORKFLOW.md authors choose `"gemini"` (default) or `"claude"`.
2. **Claude Code agent runner** — Implement a `ClaudeRunner` that satisfies the existing `AgentLauncher` interface using `claude -p` with NDJSON streaming.
3. **Session continuity** — Persist Claude Code session ID to disk so multi-turn conversations resume seamlessly via `--resume`.
4. **NDJSON stream parsing** — Parse Claude Code's stream-json output in real-time and emit the same `OrchestratorEvent` types the existing event pipeline expects.
5. **Minimal orchestrator changes** — The orchestrator, dispatch, retry, and reconciliation logic remain unchanged; only the runner is swapped.
6. **Backend-agnostic state** — Rename Gemini-specific fields in state/config to be backend-neutral.

---

## 3. Functional Requirements

### FR-1: Backend Selection Config

**What:** Add a top-level `backend` field to WORKFLOW.md config that selects which `AgentLauncher` implementation to use.

**Acceptance Criteria:**
- AC-1.1: New config field `backend` (string). Valid values: `"gemini"` (default), `"claude"`.
- AC-1.2: When `backend` is omitted or empty, default to `"gemini"` (backward compatible).
- AC-1.3: When `backend` is `"claude"`, the orchestrator uses `ClaudeRunner` instead of `GeminiRunner`.
- AC-1.4: Unknown backend value → startup validation error: `"unsupported backend: <value>"`.
- AC-1.5: Backend selection happens at startup in `main.go` via a factory function.
- AC-1.6: Hot-reload of WORKFLOW.md does NOT change the active backend (restart required). Log a warning if `backend` value changes on reload.

### FR-2: Claude Code Configuration

**What:** Add a `claude` config section in WORKFLOW.md alongside the existing `gemini` section.

**Acceptance Criteria:**
- AC-2.1: New `ClaudeConfig` struct with fields:
  - `command` (string): CLI executable name. Default: `"claude"`.
  - `model` (string): Model to use. Default: `"claude-sonnet-4-6"`.
  - `permission_mode` (string): Permission mode flag. Default: `"bypassPermissions"`.
  - `allowed_tools` ([]string): Tools to allow. Default: `["Read", "Write", "Edit", "Bash"]`.
  - `max_turns` (int): Max turns per Claude invocation. Default: `25`.
  - `turn_timeout_ms` (int): Max time per CLI invocation. Default: `600000` (10 min).
  - `stall_timeout_ms` (int): Max time without NDJSON output. Default: `300000` (5 min).
- AC-2.2: Config section key is `claude` in WORKFLOW.md YAML.
- AC-2.3: Defaults applied for missing fields (same pattern as `GeminiConfig`).
- AC-2.4: Claude config is parsed regardless of `backend` selection (for validation), but only consumed when `backend: claude`.

**Example WORKFLOW.md snippet:**
```yaml
backend: claude

claude:
  command: claude
  model: claude-sonnet-4-6
  permission_mode: bypassPermissions
  allowed_tools:
    - Read
    - Write
    - Edit
    - "Bash(git *)"
  max_turns: 25
  turn_timeout_ms: 600000
  stall_timeout_ms: 300000
```

### FR-3: Claude Runner (AgentLauncher Implementation)

**What:** Implement `ClaudeRunner` satisfying the `AgentLauncher` interface. Each turn spawns a `claude -p` subprocess, parses NDJSON output, and manages session continuity via `--resume`.

**Acceptance Criteria:**

#### Lifecycle
- AC-3.1: `ClaudeRunner` implements `AgentLauncher.Launch(ctx, params, eventCh)`.
- AC-3.2: Same workspace lifecycle as `GeminiRunner`: create workspace → validate path → run before_run hook → run agent → run after_run hook.
- AC-3.3: On first turn, no session ID exists; do not pass `--resume`.
- AC-3.4: After the first turn's NDJSON output includes a `session_id` (from `system/init` event), persist it to `<workspace>/.symphony-session-id`.
- AC-3.5: On subsequent turns, read session ID from `<workspace>/.symphony-session-id` and pass `--resume <session_id>`.
- AC-3.6: If `.symphony-session-id` file is missing or empty on turn > 1, log a warning and proceed without `--resume` (starts fresh session).

#### CLI Invocation
- AC-3.7: Build CLI command: `<command> -p "<prompt>" --output-format stream-json --max-turns <max_turns> --model <model> --permission-mode <permission_mode>`.
- AC-3.8: Append `--allowedTools <tool>` for each entry in `allowed_tools`.
- AC-3.9: Append `--resume <session_id>` when a session ID is available.
- AC-3.10: If `.mcp.json` exists in workspace, append `--mcp-config <workspace>/.mcp.json`.
- AC-3.11: Spawn via PTY wrapper: `script -q /dev/null /bin/sh -c '<full_command>'` to satisfy Claude Code's TTY requirement.
- AC-3.12: If `script` binary is not found on PATH, fall back to direct execution with a warning that output may not be produced.
- AC-3.13: Set working directory to workspace path.
- AC-3.14: Pass `params.ExtraEnv` as additional environment variables.

#### Turn Loop
- AC-3.15: Turn loop runs up to `params.AgentCfg.MaxTurns` iterations (the orchestrator-level max turns, not the per-invocation Claude `--max-turns`).
- AC-3.16: Each turn spawns a fresh `claude -p` process (Claude Code exits after each invocation).
- AC-3.17: First turn uses the full rendered prompt from the workflow template.
- AC-3.18: Continuation turns (turn > 1) use a continuation prompt: `"Continue working on this issue. You are on turn <N> of <max>. Check the current state of your work and continue from where you left off."`.
- AC-3.19: Check context cancellation before each turn.
- AC-3.20: After each successful turn, re-check issue state via `params.CheckIssueState`. If no longer active, exit loop.
- AC-3.21: On turn result with `subtype: "error"`, exit the turn loop with an error.

#### NDJSON Output Parsing
- AC-3.22: Read stdout line-by-line (NDJSON = newline-delimited JSON).
- AC-3.23: Handle partial lines across read boundaries (line accumulator/buffer pattern).
- AC-3.24: Parse each complete JSON line and classify event type.
- AC-3.25: Map Claude Code event types to Symphony event types:

| Claude Code NDJSON `type` | `subtype` | Symphony Event |
|---|---|---|
| `system` | `init` | `session_started` (extract `session_id`) |
| `assistant` | (with `tool_use` content) | `tool_call` |
| `assistant` | (text only) | `notification` |
| `result` | `success` | `turn_completed` |
| `result` | `error` | `turn_failed` |
| `result` | (other, e.g. `error_max_turns`) | `turn_completed` |
| `user` | — | `notification` |
| (unknown) | — | `notification` |
| (malformed JSON) | — | `malformed` |

- AC-3.26: Extract token usage from `result` events when `usage` field is present (fields: `input_tokens`, `output_tokens`).
- AC-3.27: Forward each parsed event to `eventCh` as an `OrchestratorEvent` with `EventAgentUpdate` type.

#### Process Management
- AC-3.28: Kill the subprocess on context cancellation (SIGKILL to process group).
- AC-3.29: Apply `turn_timeout_ms` per CLI invocation. On timeout, kill subprocess and return error.
- AC-3.30: Non-zero exit code from `claude` is treated as a turn failure.
- AC-3.31: Exit code 0 with a `result/success` NDJSON event = successful turn.
- AC-3.32: Exit code 0 without any `result` event = treat as completed (Claude may have had nothing to do).

### FR-4: NDJSON Parser Module

**What:** A reusable, stateful line-accumulator that buffers partial reads and emits parsed NDJSON events.

**Acceptance Criteria:**
- AC-4.1: `NdjsonParser` struct with internal byte buffer.
- AC-4.2: `Feed(data []byte) []NdjsonEvent` — accepts raw bytes, returns zero or more complete parsed events.
- AC-4.3: `Flush() []NdjsonEvent` — returns any remaining buffered data as events (may be malformed).
- AC-4.4: Handles split lines across multiple `Feed()` calls.
- AC-4.5: Empty lines are skipped silently.
- AC-4.6: Invalid JSON lines emit a `malformed` event type (with raw line for debugging).
- AC-4.7: `NdjsonEvent` struct: `{Type string, Subtype string, Raw map[string]any}`.

### FR-5: RunParams Generalization

**What:** Update `RunParams` to carry backend config generically instead of only `GeminiCfg`.

**Acceptance Criteria:**
- AC-5.1: Add `ClaudeCfg *config.ClaudeConfig` field to `RunParams`.
- AC-5.2: Keep `GeminiCfg *config.GeminiConfig` for backward compatibility.
- AC-5.3: The orchestrator populates the appropriate config field based on active backend.
- AC-5.4: Each runner reads only its own config field.

### FR-6: Backend-Agnostic State & Dashboard

**What:** Rename Gemini-specific fields in orchestrator state and dashboard to be backend-neutral.

**Acceptance Criteria:**
- AC-6.1: `State.GeminiTotals` → `State.AgentTotals`.
- AC-6.2: `State.GeminiModel` → `State.AgentModel`.
- AC-6.3: `State.GeminiCommand` → `State.AgentCommand`.
- AC-6.4: `State.GeminiPID` (in `RunningEntry`) → `State.AgentPID`.
- AC-6.5: `StateSnapshot` and JSON keys updated accordingly: `gemini_model` → `agent_model`, `gemini_command` → `agent_command`.
- AC-6.6: Add `State.BackendKind` (string: `"gemini"` or `"claude"`) for dashboard display.
- AC-6.7: Dashboard and API show `backend_kind` in config snapshot.
- AC-6.8: JSON key `codex_totals` in snapshot → `agent_totals`.

### FR-7: Factory Function

**What:** A factory in `main.go` (or `agent` package) that returns the correct `AgentLauncher` based on config.

**Acceptance Criteria:**
- AC-7.1: `func NewLauncher(backend string) (AgentLauncher, error)` — returns `GeminiRunner` or `ClaudeRunner`.
- AC-7.2: Unknown backend string → error.
- AC-7.3: `main.go` calls `NewLauncher(cfg.Backend)` instead of hardcoding `NewGeminiRunner()`.

---

## 4. Out of Scope

- Budget/cost tracking (`max_budget_usd` from reference implementation)
- cmux visibility mode (visible terminal tabs)
- MCP server configuration management (users create `.mcp.json` via hooks)
- Claude Code `--append-system-prompt` flag
- Multiple backends per WORKFLOW.md (one backend at a time)
- Dispatch routing per issue state (all issues use the same backend)

---

## 5. Technical Constraints

- **Claude Code CLI:** Must be installed and on PATH (`claude` binary)
- **PTY requirement:** Claude Code requires a TTY for `--output-format stream-json`. Use `script -q /dev/null` as PTY wrapper on macOS/Linux.
- **Session file:** `.symphony-session-id` is a plain text file containing only the session UUID.
- **NDJSON format:** One JSON object per line, no framing. Events arrive in real-time as Claude processes.
- **Process model:** Each turn is a separate OS process (unlike Gemini's long-running ACP process). This means process startup overhead per turn but simpler lifecycle management.

---

## 6. Example WORKFLOW.md

```yaml
---
backend: claude

tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: my-project
  active_states:
    - Todo
    - In Progress
  terminal_states:
    - Done
    - Cancelled

polling:
  interval_ms: 30000

workspace:
  root: ~/symphony_workspaces

hooks:
  after_create: |
    git clone <repo_url> .
    cat > .mcp.json << 'EOF'
    {
      "mcpServers": {
        "linear": {
          "command": "npx",
          "args": ["linear-mcp-server"],
          "env": { "LINEAR_API_KEY": "${LINEAR_API_KEY}" }
        }
      }
    }
    EOF
  before_run: |
    git pull origin main
  timeout_ms: 120000

agent:
  max_concurrent_agents: 3
  max_turns: 10

claude:
  command: claude
  model: claude-sonnet-4-6
  permission_mode: bypassPermissions
  allowed_tools:
    - Read
    - Write
    - Edit
    - Bash
  max_turns: 25
  turn_timeout_ms: 600000
---

You are an autonomous coding agent working on issue {{ issue.identifier }}: {{ issue.title }}.

{% if issue.description %}
## Description
{{ issue.description }}
{% endif %}

Work in this repository to resolve the issue. Create a branch, make changes, run tests, and commit.
```
