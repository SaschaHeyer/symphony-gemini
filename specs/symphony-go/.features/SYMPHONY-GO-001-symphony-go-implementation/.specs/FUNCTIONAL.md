# FUNCTIONAL SPEC: Symphony Go — Gemini-Powered Issue Orchestrator

**Status:** Draft v1
**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-001

---

## 1. Overview

Symphony Go is a fresh, from-scratch Go implementation of the [Symphony specification](../../SPEC.md). It is a long-running automation service that continuously reads work from Linear, creates isolated workspaces, and runs a **Gemini CLI** coding-agent session (via the Agent Client Protocol over stdio) for each issue.

### 1.1 Key Divergence from Reference Implementation

| Aspect | Reference (Elixir/Codex) | This Implementation (Go/Gemini) |
|---|---|---|
| Language | Elixir/OTP | Go |
| Agent engine | Codex app-server (JSON-RPC) | Gemini CLI in ACP mode (`gemini --experimental-acp`) |
| Agent protocol | Codex-specific JSON-RPC | Agent Client Protocol (ACP) — JSON-RPC 2.0 over stdio |
| Default model | OpenAI models | `gemini-3.1-pro-preview` |
| Concurrency model | OTP GenServer + processes | Goroutines + channels |
| Approval handling | Codex approval_policy | ACP `session/request_permission` flow |

### 1.2 What This Is NOT

- Not a fork or port of the Elixir implementation
- Not using any existing Symphony code
- Not compatible with Codex app-server protocol

---

## 2. Goals

1. **Full spec conformance** — Implement all Core Conformance requirements from SPEC.md Sections 17 and 18.1.
2. **Gemini CLI as agent** — Replace Codex app-server with Gemini CLI running in ACP mode as the coding-agent subprocess.
3. **Go-native architecture** — Leverage Go's concurrency primitives (goroutines, channels, select) instead of OTP.
4. **Clean ownership** — Zero dependencies on the Elixir codebase; standalone Go module.
5. **Operational simplicity** — Single static binary, minimal runtime dependencies.

---

## 3. Functional Requirements

### FR-1: Workflow Loader

**What:** Parse `WORKFLOW.md` files containing YAML front matter + Markdown prompt body.

**Acceptance Criteria:**
- AC-1.1: Load workflow from explicit path argument or default `./WORKFLOW.md`.
- AC-1.2: Split YAML front matter (between `---` delimiters) from prompt body.
- AC-1.3: Return error `missing_workflow_file` when file is not found.
- AC-1.4: Return error `workflow_parse_error` when YAML is invalid.
- AC-1.5: Return error `workflow_front_matter_not_a_map` when YAML doesn't decode to a map.
- AC-1.6: Trim prompt body whitespace.
- AC-1.7: Empty front matter → empty config map, full file as prompt body.

### FR-2: Configuration Layer

**What:** Typed config getters with defaults, `$VAR` environment resolution, `~` expansion, and dynamic reload.

**Acceptance Criteria:**
- AC-2.1: All config fields from SPEC.md Section 6.4 are supported with correct defaults.
- AC-2.2: `$VAR_NAME` in `tracker.api_key` and path values resolves from environment.
- AC-2.3: `$VAR` resolving to empty string is treated as missing.
- AC-2.4: `~` expands to user home directory in path fields.
- AC-2.5: Integer fields accept both integer and string-integer YAML values.
- AC-2.6: `per_state_concurrency` map normalizes keys to lowercase, ignores invalid values.
- AC-2.7: Watch `WORKFLOW.md` for changes; re-read and re-apply without restart.
- AC-2.8: Invalid reload keeps last known good config and emits operator-visible error.

**Gemini-specific config additions under `gemini` key (replaces `codex`):**
- `gemini.command`: shell command to launch Gemini CLI in ACP mode. Default: `gemini --experimental-acp`
- `gemini.model`: Gemini model to use. Default: `gemini-3.1-pro-preview`
- `gemini.turn_timeout_ms`: total turn stream timeout. Default: `3600000` (1 hour)
- `gemini.read_timeout_ms`: request/response timeout during startup. Default: `5000`
- `gemini.stall_timeout_ms`: event inactivity timeout. Default: `300000` (5 min). `<= 0` disables.

**Note:** The `codex` config key from the spec is aliased to `gemini` for this implementation. If `codex` key is present in WORKFLOW.md, it is read as `gemini` config for backward compatibility.

### FR-3: Issue Tracker Client (Linear)

**What:** GraphQL client for Linear that fetches candidate issues, refreshes states, and fetches terminal issues.

**Acceptance Criteria:**
- AC-3.1: `fetch_candidate_issues()` — fetch issues in active states for configured project slug using `slugId` filter.
- AC-3.2: `fetch_issue_states_by_ids(ids)` — fetch current states for running issue IDs using `[ID!]` GraphQL typing.
- AC-3.3: `fetch_issues_by_states(states)` — fetch issues in terminal states (startup cleanup).
- AC-3.4: Empty ID/state lists return empty results without API call.
- AC-3.5: Paginate candidate issues with page size 50; preserve order.
- AC-3.6: Normalize issues to domain model: id, identifier, title, description, priority (int or null), state, branch_name, url, labels (lowercase), blocked_by (from inverse `blocks` relations), created_at, updated_at.
- AC-3.7: Network timeout: 30s.
- AC-3.8: Map errors to typed categories: `unsupported_tracker_kind`, `missing_tracker_api_key`, `missing_tracker_project_slug`, `linear_api_request`, `linear_api_status`, `linear_graphql_errors`, `linear_unknown_payload`, `linear_missing_end_cursor`.

### FR-4: Orchestrator

**What:** Single-authority state machine that owns polling, dispatch, concurrency, retry, and reconciliation.

**Acceptance Criteria:**

#### Polling
- AC-4.1: Validate config at startup; fail startup on validation error.
- AC-4.2: Run startup terminal workspace cleanup.
- AC-4.3: Schedule immediate first tick, then repeat every `polling.interval_ms`.
- AC-4.4: Each tick: reconcile → validate → fetch candidates → sort → dispatch.
- AC-4.5: If per-tick validation fails, skip dispatch but still reconcile.
- AC-4.6: If candidate fetch fails, log and skip dispatch for that tick.

#### Dispatch
- AC-4.7: Sort candidates: priority ascending (null last) → created_at oldest → identifier lexicographic.
- AC-4.8: Dispatch only if: has required fields, state is active & not terminal, not running, not claimed, global slots available, per-state slots available.
- AC-4.9: `Todo` state with non-terminal blockers is NOT eligible.
- AC-4.10: `Todo` state with all-terminal blockers IS eligible.
- AC-4.11: Global concurrency: `max(max_concurrent_agents - running_count, 0)`.
- AC-4.12: Per-state concurrency: `max_concurrent_agents_by_state[lowercase(state)]` if set.

#### Reconciliation
- AC-4.13: Stall detection: if time since last event > `gemini.stall_timeout_ms`, kill worker and queue retry.
- AC-4.14: Stall detection disabled when `stall_timeout_ms <= 0`.
- AC-4.15: Tracker state refresh for all running issues each tick.
- AC-4.16: Terminal state → stop worker + clean workspace.
- AC-4.17: Still active → update in-memory issue snapshot.
- AC-4.18: Neither active nor terminal → stop worker, no workspace cleanup.
- AC-4.19: State refresh failure → keep workers running, retry next tick.

#### Retry
- AC-4.20: Normal worker exit → continuation retry at attempt=1 with 1s delay.
- AC-4.21: Abnormal exit → exponential backoff: `min(10000 * 2^(attempt-1), max_retry_backoff_ms)`.
- AC-4.22: Retry timer fires → fetch candidates, find issue, dispatch or release claim.
- AC-4.23: Slot exhaustion during retry → requeue with error "no available orchestrator slots".

#### State
- AC-4.24: Maintain `running` map, `claimed` set, `retry_attempts` map, `completed` set.
- AC-4.25: Maintain `codex_totals` (token counts + runtime seconds) and `rate_limits`.
- AC-4.26: Dynamic config reload updates `poll_interval_ms`, `max_concurrent_agents`, and all other runtime settings.

### FR-5: Workspace Manager

**What:** Create, reuse, and clean per-issue workspace directories under a configurable root.

**Acceptance Criteria:**
- AC-5.1: Workspace key = issue identifier with non-`[A-Za-z0-9._-]` chars replaced by `_`.
- AC-5.2: Workspace path = `<workspace.root>/<workspace_key>`.
- AC-5.3: Create directory if missing; mark `created_now=true`.
- AC-5.4: Reuse existing directory; mark `created_now=false`.
- AC-5.5: **Safety invariant:** workspace path must be inside workspace root (absolute prefix check).
- AC-5.6: **Safety invariant:** agent cwd must equal workspace path.
- AC-5.7: Run `after_create` hook only when `created_now=true`. Failure is fatal.
- AC-5.8: Run `before_run` hook before each attempt. Failure aborts attempt.
- AC-5.9: Run `after_run` hook after each attempt. Failure logged, ignored.
- AC-5.10: Run `before_remove` hook before deletion. Failure logged, ignored.
- AC-5.11: Hooks execute via `sh -lc <script>` with workspace as cwd.
- AC-5.12: Hooks respect `hooks.timeout_ms` (default 60s).
- AC-5.13: Startup terminal cleanup removes workspaces for issues in terminal states.

### FR-6: Agent Runner (Gemini CLI via ACP)

**What:** Launch Gemini CLI as an ACP subprocess, manage sessions and turns, stream events back to orchestrator.

**Acceptance Criteria:**

#### Launch
- AC-6.1: Launch via `bash -lc <gemini.command>` with workspace as cwd.
- AC-6.2: Default command: `gemini --experimental-acp`.
- AC-6.3: Communicate over stdin (write) / stdout (read), line-delimited JSON-RPC 2.0.
- AC-6.4: Stderr is diagnostic only, not protocol-parsed.

#### ACP Handshake
- AC-6.5: Send `initialize` with `protocolVersion: 1`, `clientInfo: {name: "symphony-go", version: "1.0"}`, `clientCapabilities: {fs: {readTextFile: true, writeTextFile: true}, terminal: true}`.
- AC-6.6: Wait for `initialize` response (within `read_timeout_ms`). Verify protocol version compatibility.
- AC-6.7: Send `session/new` with `cwd: <absolute_workspace_path>`, `mcpServers: []`.
- AC-6.8: Read `sessionId` from response.

#### Turn Management
- AC-6.9: Send `session/prompt` with `sessionId` and `prompt: [{type: "text", text: "<rendered_prompt>"}]`.
- AC-6.10: First turn uses full rendered task prompt from workflow template.
- AC-6.11: Continuation turns send continuation guidance, not the original prompt.
- AC-6.12: Stream `session/update` notifications until turn completes (StopReason received).
- AC-6.13: Turn completion reasons: `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`.
- AC-6.14: After successful turn, re-check issue state. If still active and under `max_turns`, start another turn.
- AC-6.15: Reuse same `sessionId` for all continuation turns within one worker.

#### Permission & Tool Handling
- AC-6.16: Auto-approve `session/request_permission` requests (high-trust mode). Respond with `{outcome: "selected", optionId: <first-allow-option>}`.
- AC-6.17: Log all tool call updates (`tool_call`, `tool_call_update`) for observability.
- AC-6.18: Unsupported tool calls do not stall the session.

#### Events to Orchestrator
- AC-6.19: Emit structured events: `session_started`, `turn_completed`, `turn_failed`, `turn_cancelled`, `notification`, `malformed`.
- AC-6.20: Each event includes: event type, timestamp, session_id, optional usage/token data.
- AC-6.21: Extract token usage from session/update notifications when available.

#### Timeouts
- AC-6.22: `read_timeout_ms` for handshake responses.
- AC-6.23: `turn_timeout_ms` for total turn duration.
- AC-6.24: `stall_timeout_ms` enforced by orchestrator (not agent runner).

### FR-7: Prompt Rendering

**What:** Render workflow template with issue data using strict variable checking.

**Acceptance Criteria:**
- AC-7.1: Render with `issue` object (all normalized fields) and `attempt` (null or integer).
- AC-7.2: Unknown variables fail rendering (strict mode).
- AC-7.3: Unknown filters fail rendering.
- AC-7.4: Nested arrays/maps (labels, blockers) accessible in templates.
- AC-7.5: Empty prompt body falls back to `"You are working on an issue from Linear."`.
- AC-7.6: Use Go template engine with Liquid-compatible semantics (e.g., `text/template` or a Liquid port).
- AC-7.7: Template errors fail the run attempt.

### FR-8: Logging & Observability

**What:** Structured logging with issue/session context.

**Acceptance Criteria:**
- AC-8.1: Structured JSON logs to stderr (default sink).
- AC-8.2: All issue-related logs include `issue_id` and `issue_identifier`.
- AC-8.3: Session lifecycle logs include `session_id`.
- AC-8.4: Stable `key=value` phrasing with action outcomes.
- AC-8.5: No raw API tokens in logs.
- AC-8.6: Log sink failures do not crash the orchestrator.

### FR-9: CLI Entrypoint

**What:** CLI binary that accepts workflow path and starts the service.

**Acceptance Criteria:**
- AC-9.1: Accept optional positional argument: path to `WORKFLOW.md`.
- AC-9.2: Default to `./WORKFLOW.md` when no argument provided.
- AC-9.3: Error on nonexistent explicit path or missing default.
- AC-9.4: Clean startup failure output.
- AC-9.5: Exit 0 on normal shutdown.
- AC-9.6: Exit nonzero on startup failure or abnormal exit.
- AC-9.7: Handle SIGINT/SIGTERM for graceful shutdown (stop workers, run cleanup hooks).

---

## 4. Extension Requirements (Optional, Recommended)

### EXT-1: HTTP Server

- AC-E1.1: Start when `--port` CLI flag or `server.port` config is present.
- AC-E1.2: CLI `--port` overrides `server.port`.
- AC-E1.3: Bind to `127.0.0.1` by default.
- AC-E1.4: `GET /` — human-readable dashboard.
- AC-E1.5: `GET /api/v1/state` — JSON system state snapshot.
- AC-E1.6: `GET /api/v1/<issue_identifier>` — issue-specific debug details.
- AC-E1.7: `POST /api/v1/refresh` — trigger immediate poll cycle.
- AC-E1.8: `404` for unknown issues, `405` for unsupported methods.
- AC-E1.9: Port `0` for ephemeral binding.

### EXT-2: Linear GraphQL Tool

- AC-E2.1: Expose `linear_graphql` as a client-side tool available to Gemini sessions.
- AC-E2.2: Accept `{query, variables}` input, execute against configured Linear auth.
- AC-E2.3: Return structured results (success/failure with GraphQL body).
- AC-E2.4: Reject multi-operation documents.

---

## 5. Out of Scope

- Web UI beyond the simple dashboard extension
- Non-Linear issue trackers
- Codex app-server compatibility
- Persistent orchestrator database
- Multi-tenant or distributed deployment
- SSH worker extension (Appendix A of spec) — may be added later

---

## 6. Technical Constraints

- **Language:** Go (latest stable, currently 1.24)
- **Agent:** Gemini CLI in ACP mode (`gemini --experimental-acp`)
- **Model:** `gemini-3.1-pro-preview` (configurable)
- **Protocol:** Agent Client Protocol (ACP) — JSON-RPC 2.0 over stdio
- **Template engine:** Liquid-compatible (strict mode) — evaluate `github.com/osteele/liquid` or `text/template` with strict option
- **YAML parsing:** `gopkg.in/yaml.v3`
- **GraphQL:** Raw HTTP POST to Linear API (no heavy GraphQL client library needed)
- **File watching:** `fsnotify/fsnotify`
- **Structured logging:** `log/slog` (stdlib)
- **HTTP server:** `net/http` (stdlib)
- **Build output:** Single static binary

---

## 7. Mapping: SPEC.md Section → Functional Requirement

| SPEC Section | FR |
|---|---|
| 5. Workflow Specification | FR-1: Workflow Loader |
| 6. Configuration | FR-2: Configuration Layer |
| 11. Issue Tracker Integration | FR-3: Linear Client |
| 7-8. Orchestration & Polling | FR-4: Orchestrator |
| 9. Workspace Management | FR-5: Workspace Manager |
| 10. Agent Runner Protocol | FR-6: Agent Runner (ACP) |
| 12. Prompt Construction | FR-7: Prompt Rendering |
| 13. Logging & Observability | FR-8: Logging |
| 17.7 CLI & Host Lifecycle | FR-9: CLI |
| 13.7 HTTP Server | EXT-1: HTTP Server |
| 10.5 linear_graphql tool | EXT-2: Linear GraphQL Tool |
