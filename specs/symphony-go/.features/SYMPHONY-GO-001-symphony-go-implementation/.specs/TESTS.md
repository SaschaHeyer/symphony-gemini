# TEST SPEC: Symphony Go

**Status:** Draft v1
**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-001
**Depends on:** FUNCTIONAL.md v1, TECH.md v1

---

## 1. Testing Strategy

- **Unit tests** for each `internal/` package — pure logic, no subprocesses, no network
- **Integration tests** for cross-package flows (orchestrator + mock tracker + mock agent)
- **E2E tests** (Real Integration Profile) — requires `LINEAR_API_KEY` and `gemini` CLI installed, skipped when unavailable
- All tests run via `go test ./...`
- Use table-driven tests where there are multiple input/output variations
- Use `t.Helper()` and `testify/assert` for readable assertions

---

## 2. Unit Tests

### 2.1 Workflow Loader (`internal/workflow`)

| Test | Why |
|---|---|
| Parses valid YAML front matter + prompt body | Core parsing path must work |
| Empty front matter → empty config, full body as prompt | Spec requires graceful handling of no-config workflows |
| No `---` delimiters → entire file is prompt body | Spec: "If front matter is absent, treat entire file as prompt body" |
| Invalid YAML returns `ErrWorkflowParseError` | Must surface typed errors for operator visibility |
| Non-map YAML returns `ErrFrontMatterNotMap` | Spec explicitly requires map; integer/list YAML front matter is an error |
| Missing file returns `ErrMissingWorkflowFile` | Must fail cleanly, not panic |
| Prompt body is trimmed | Spec: "Prompt body is trimmed before use" |
| Unknown top-level keys are preserved (not rejected) | Forward compatibility: spec says ignore unknown keys |

### 2.2 Config Layer (`internal/config`)

| Test | Why |
|---|---|
| All defaults apply when config is empty | Ensures every field has a safe default per spec Section 6.4 |
| `$VAR` resolves from environment for `tracker.api_key` | Core secret resolution mechanism |
| `$VAR` resolving to empty string treated as missing | Spec: "If $VAR_NAME resolves to empty string, treat key as missing" |
| `~` expands to home dir in `workspace.root` | Path expansion is spec-required |
| String integer coerced to int (`"30000"` → `30000`) | YAML may produce string integers; spec requires both forms |
| `per_state_concurrency` normalizes keys to lowercase | Spec: "State keys normalized (lowercase) for lookup" |
| `per_state_concurrency` ignores non-positive/non-numeric values | Spec: "Invalid entries ignored" |
| `codex` key aliased to `gemini` | Backward compat with spec's codex-centric WORKFLOW.md files |
| `gemini.command` preserved as shell string (not split) | Will be passed to `bash -lc`, must remain a single string |

### 2.3 Config Validation (`internal/config`)

| Test | Why |
|---|---|
| Valid config passes validation | Happy path |
| Missing `tracker.kind` fails | Required for dispatch |
| Unsupported `tracker.kind` (e.g. `"jira"`) fails | Only `linear` is supported |
| Missing `tracker.api_key` after resolution fails | Can't poll without auth |
| Missing `tracker.project_slug` fails | Required for Linear queries |
| Missing `gemini.command` fails | Can't launch agent without command |

### 2.4 Workspace Manager (`internal/workspace`)

| Test | Why |
|---|---|
| Deterministic path for same identifier | Workspace reuse depends on stable paths |
| Creates directory when missing, `CreatedNow=true` | Core workspace creation flow |
| Reuses existing directory, `CreatedNow=false` | Spec: workspaces persist across runs |
| Sanitizes identifier: `ABC-123` → `ABC-123`, `foo/bar` → `foo_bar`, `a b` → `a_b` | Safety invariant: only `[A-Za-z0-9._-]` in dir names |
| Rejects workspace path outside root | Safety invariant: path containment |
| Rejects path traversal (`../`) in workspace key | Prevents escaping workspace root |

### 2.5 Workspace Hooks (`internal/workspace`)

| Test | Why |
|---|---|
| `after_create` runs only when `CreatedNow=true` | Spec: hook gated on new creation |
| `after_create` NOT run when `CreatedNow=false` | Must not re-run setup on reuse |
| `before_run` failure aborts attempt (returns error) | Spec: failure is fatal to current attempt |
| `after_run` failure is logged but returns nil | Spec: failure logged and ignored |
| `before_remove` failure is logged but returns nil | Spec: failure logged and ignored |
| Hook timeout kills hook and returns error | Prevents hanging orchestrator |
| Hook runs with workspace as cwd | Spec: "Execute with workspace directory as cwd" |

### 2.6 Linear Client (`internal/tracker`)

| Test | Why |
|---|---|
| Candidate fetch constructs correct GraphQL with `slugId` filter | Spec: must use `slugId` not `slug` |
| State refresh uses `[ID!]` variable type | Spec Section 11.2 requires this exact GraphQL typing |
| Empty ID list returns empty without API call | Optimization + prevents invalid queries |
| Empty state list returns empty without API call | Same |
| Pagination follows `endCursor` across pages | Must not lose issues across page boundaries |
| Labels normalized to lowercase | Spec: "labels → lowercase strings" |
| Blockers extracted from inverse `blocks` relations | Spec: "blocked_by → derived from inverse relations where type is blocks" |
| Priority non-integer becomes nil | Spec: "priority → integer only, non-integers become null" |
| Timestamps parsed from ISO-8601 | Spec: "parse ISO-8601 timestamps" |
| HTTP error mapped to `linear_api_request` | Typed error categories for operator debugging |
| Non-200 status mapped to `linear_api_status` | Separate from transport errors |
| GraphQL errors mapped to `linear_graphql_errors` | Distinguishes API-level vs transport errors |
| Missing `endCursor` mapped to `linear_missing_end_cursor` | Pagination integrity check |

### 2.7 Orchestrator Dispatch (`internal/orchestrator`)

| Test | Why |
|---|---|
| Sort: priority ascending, null last | Spec dispatch priority rule |
| Sort: same priority → oldest `created_at` first | Spec secondary sort |
| Sort: same priority+time → identifier lexicographic | Spec tie-breaker |
| Skips issue already in `running` | Prevents duplicate dispatch |
| Skips issue already in `claimed` | Prevents dispatch during retry window |
| Respects global concurrency limit | Spec: `max(max_concurrent - running, 0)` |
| Respects per-state concurrency limit | Spec: per-state cap |
| `Todo` with non-terminal blocker NOT dispatched | Spec: blocker rule |
| `Todo` with all-terminal blockers IS dispatched | Spec: only non-terminal blockers block |
| Issue missing required fields skipped | Spec: must have id, identifier, title, state |

### 2.8 Orchestrator Reconciliation (`internal/orchestrator`)

| Test | Why |
|---|---|
| Terminal state stops worker + cleans workspace | Spec: terminal → stop + cleanup |
| Active state updates in-memory issue snapshot | Spec: keep snapshot current |
| Neither active nor terminal stops worker, no cleanup | Spec: stop but preserve workspace |
| No running issues → reconciliation is no-op | Avoid unnecessary tracker calls |
| State refresh failure keeps workers running | Spec: "keep workers running, try next tick" |
| Stall detected → kill worker + schedule retry | Spec: stall_timeout_ms enforcement |
| `stall_timeout_ms <= 0` disables stall detection | Spec: "If <= 0, skip stall detection" |

### 2.9 Orchestrator Retry (`internal/orchestrator`)

| Test | Why |
|---|---|
| Normal exit → continuation retry, attempt=1, delay=1s | Spec: short continuation retry |
| Abnormal exit → exponential backoff `min(10000*2^(n-1), max)` | Spec: failure-driven backoff formula |
| Backoff capped at `max_retry_backoff_ms` | Spec: "Power capped by configured max" |
| Retry fires → issue still active → dispatch | Happy retry path |
| Retry fires → issue not found → release claim | Spec: "If not found, release claim" |
| Retry fires → no slots → requeue with error | Spec: "requeue with error 'no available orchestrator slots'" |
| Existing retry timer cancelled on new retry | Spec: "Cancel any existing retry timer for the same issue" |

### 2.10 ACP Client (`internal/agent`)

| Test | Why |
|---|---|
| `initialize` sends correct JSON-RPC with protocol version | ACP handshake must be well-formed |
| `initialize` response parsed correctly | Must extract agent capabilities |
| `session/new` sends cwd and receives sessionId | Session setup is prerequisite for turns |
| `session/prompt` sends prompt and receives StopReason | Core turn flow |
| `session/update` notifications dispatched to handler | Streaming events drive observability |
| `session/request_permission` auto-approved with first allow option | High-trust mode: no stalling on permissions |
| Read timeout enforced on handshake | Prevents hanging on broken subprocess |
| Turn timeout enforced | Prevents runaway turns |
| Partial JSON lines buffered until newline | Spec: line-delimited protocol |
| Stderr lines logged but not parsed as protocol | Spec: "stderr not part of protocol stream" |
| Subprocess exit during turn → error | Must detect and report agent crash |

### 2.11 Prompt Rendering (`internal/prompt`)

| Test | Why |
|---|---|
| Renders `issue.identifier`, `issue.title`, etc. | Core template variable access |
| Renders `issue.labels` as iterable list | Must preserve nested arrays |
| Renders `attempt` as null on first run | Spec: null/absent on first attempt |
| Renders `attempt` as integer on retry | Spec: integer on retry |
| Unknown variable → error | Spec: strict mode |
| Unknown filter → error | Spec: strict mode |
| Empty template → fallback prompt | Spec: minimal default prompt |

### 2.12 Token Accounting (`internal/orchestrator`)

| Test | Why |
|---|---|
| Absolute totals accumulated correctly across events | Spec: prefer absolute totals |
| Delta tracking avoids double-counting | Spec: "track deltas relative to last reported" |
| Runtime seconds added on worker exit | Spec: "Add run duration seconds on session end" |
| Snapshot includes live elapsed time from running entries | Spec: "live aggregate at snapshot time" |

---

## 3. Integration Tests

| Test | Why |
|---|---|
| Orchestrator tick with mock tracker + mock agent: dispatches eligible issue | Validates end-to-end dispatch flow without real services |
| Worker lifecycle: workspace create → hook → ACP session → turn → exit → retry | Validates the full worker attempt sequence |
| Config reload changes poll interval on next tick | Validates hot reload reaches the orchestrator |
| Invalid config reload keeps last good config | Spec: "keep operating with last known good config" |
| Graceful shutdown cancels workers and exits cleanly | Validates SIGTERM handling |
| HTTP API `/api/v1/state` returns current snapshot | Validates server reads orchestrator state |

### Mock Strategy

- **Mock tracker:** In-memory implementation of `LinearClient` interface returning canned issues
- **Mock agent subprocess:** Small Go binary or shell script that speaks minimal ACP (responds to `initialize`, `session/new`, `session/prompt` with canned responses)
- **Mock via interface:** Define interfaces for `TrackerClient`, `AgentLauncher` to allow injection

```go
type TrackerClient interface {
    FetchCandidateIssues(ctx context.Context, projectSlug string, activeStates []string) ([]Issue, error)
    FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]Issue, error)
    FetchIssuesByStates(ctx context.Context, projectSlug string, states []string) ([]Issue, error)
}

type AgentLauncher interface {
    Launch(ctx context.Context, params RunParams, eventCh chan<- Event) error
}
```

---

## 4. E2E / Real Integration Tests

**Profile:** Real Integration (skipped when credentials unavailable)

| Test | Why |
|---|---|
| Linear API smoke: fetch candidate issues with real credentials | Validates real GraphQL query against live Linear API |
| Linear API: fetch issue states by IDs | Validates state refresh query with real typing |
| Gemini CLI ACP smoke: initialize + session/new + single prompt | Validates real ACP handshake with installed Gemini CLI |

**Skip conditions:**
- `LINEAR_API_KEY` not set → skip Linear tests
- `gemini` not on PATH → skip Gemini tests
- Skipped tests reported as `t.Skip("reason")`, not silent pass

**Cleanup:**
- Use isolated test identifiers/workspaces
- Clean up workspace directories in `t.Cleanup()`

---

## 5. Test File Layout

```
go/
├── internal/
│   ├── workflow/
│   │   └── loader_test.go
│   ├── config/
│   │   ├── config_test.go
│   │   ├── resolve_test.go
│   │   └── validate_test.go
│   ├── tracker/
│   │   ├── client_test.go
│   │   └── normalize_test.go
│   ├── orchestrator/
│   │   ├── dispatch_test.go
│   │   ├── reconcile_test.go
│   │   ├── retry_test.go
│   │   ├── metrics_test.go
│   │   └── orchestrator_integration_test.go
│   ├── workspace/
│   │   ├── manager_test.go
│   │   ├── hooks_test.go
│   │   └── safety_test.go
│   ├── agent/
│   │   ├── acp_test.go
│   │   ├── session_test.go
│   │   └── events_test.go
│   ├── prompt/
│   │   └── render_test.go
│   └── server/
│       └── api_test.go
├── test/
│   ├── e2e/
│   │   ├── linear_live_test.go
│   │   └── gemini_live_test.go
│   └── testutil/
│       ├── mock_tracker.go
│       ├── mock_agent.go
│       └── fixtures.go
```

---

## 6. Coverage Expectations

- Unit tests: **every acceptance criterion** in FUNCTIONAL.md has at least one corresponding test
- Integration tests: **3 cross-package flows** (dispatch, worker lifecycle, config reload)
- E2E: **3 smoke tests** (Linear fetch, Linear state refresh, Gemini ACP handshake)
- No test depends on test execution order
- No test requires network access except E2E (which skip gracefully)
