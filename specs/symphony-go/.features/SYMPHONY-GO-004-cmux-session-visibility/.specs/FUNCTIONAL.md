# FUNCTIONAL SPEC: cmux Session Visibility

**Status:** Draft v1
**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-004
**Depends on:** SYMPHONY-GO-001, SYMPHONY-GO-002

---

## 1. Overview

When Symphony runs multiple agents in parallel, the only visibility into their work is structured JSON log lines printed to stdout. This makes debugging and monitoring difficult — you can't see what an agent is actually doing, what tools it's calling, or where it's stuck.

This feature adds optional **cmux integration** so that each dispatched agent session gets its own visible terminal surface in cmux. Users can see all running agents side-by-side in real-time, watch their output stream, and understand what's happening across the system at a glance.

### 1.1 Key Approach

Symphony keeps its existing process management unchanged — agents still run as hidden subprocesses with pipe-based I/O. When cmux visibility is enabled, Symphony additionally:

1. Creates a dedicated cmux **workspace** on startup
2. Creates a **terminal surface** per dispatched issue (named after the issue identifier, e.g., "AIE-10")
3. Runs `tail -f <workspace>/.symphony-agent.log` in each surface
4. **Writes all agent protocol traffic** to that log file as events arrive — for both Gemini (ACP JSON-RPC messages) and Claude Code (NDJSON events)
5. Cleans up surfaces when agents complete or issues reach terminal states

This approach works **uniformly for both backends** because it mirrors events to a visible log rather than trying to run the agent process inside the terminal. The user sees the same real-time stream regardless of whether the backend is Gemini or Claude Code.

### 1.2 What This Is NOT

- **Not a replacement for the HTTP dashboard** — the dashboard shows aggregate state; cmux shows live agent output
- **Not interactive** — users observe agent output but don't type into agent sessions
- **Not required** — cmux visibility is opt-in; Symphony works identically without it

## 2. Goals

1. Let operators see what each agent is doing in real-time via cmux terminal surfaces — for both Gemini and Claude Code backends
2. Maintain all existing orchestrator capabilities (events, token tracking, retries, stall detection) with zero changes to process management
3. Make cmux visibility fully optional — zero impact when disabled
4. Automatically manage cmux surface lifecycle (create on dispatch, clean up on completion)

## 3. Functional Requirements

### FR-1: cmux Workspace Management

**What:** Symphony creates and manages a dedicated cmux workspace for agent visibility.

**Acceptance Criteria:**
- AC-1.1: On startup (if cmux enabled), Symphony creates a cmux workspace named "Symphony" (or configured name)
- AC-1.2: If the workspace already exists (from a prior run), it is reused
- AC-1.3: If cmux is not available (binary not found or socket not responding), Symphony logs a warning and continues without cmux visibility — it must never crash or fail due to cmux unavailability
- AC-1.4: The workspace name is configurable via the `cmux.workspace_name` config field

### FR-2: Agent Surface Lifecycle

**What:** Each dispatched agent gets a dedicated cmux terminal surface, named after its issue identifier.

**Acceptance Criteria:**
- AC-2.1: When an agent is dispatched for issue `AIE-10`, a terminal surface named "AIE-10" is created in the Symphony workspace, running `tail -f <workspace>/.symphony-agent.log`
- AC-2.2: If the agent completes (success or failure) and the issue moves to a terminal state, the surface is closed after a configurable delay (default: 30 seconds) to allow reading final output
- AC-2.3: If the agent is retried, the same surface is reused (new turn output appends below previous output in the same log file)
- AC-2.4: On Symphony shutdown, all Symphony-created surfaces are cleaned up
- AC-2.5: Surface creation failure is non-fatal — the agent proceeds without visibility (logged as warning)

### FR-3: Agent Event Streaming to Log

**What:** As Symphony receives events from the agent process (via its existing pipe-based I/O), it writes them to the per-issue log file for cmux display.

**Acceptance Criteria:**
- AC-3.1: For Claude Code backend: each NDJSON event received from the agent is written to `<workspace>/.symphony-agent.log` as it arrives
- AC-3.2: For Gemini backend: each ACP JSON-RPC message (both sent and received) is written to `<workspace>/.symphony-agent.log` as it arrives
- AC-3.3: Symphony annotates the log with its own status messages: turn started, turn completed, retry scheduled, issue state changed, etc.
- AC-3.4: Log entries include a timestamp prefix for readability (e.g., `[06:40:40] {"type":"assistant",...}`)
- AC-3.5: The log file is append-only and persists across retries for the same issue (full history visible)
- AC-3.6: No changes to existing process management — agents still run as hidden subprocesses with pipe-based I/O

### FR-4: Configuration

**What:** cmux visibility is controlled via WORKFLOW.md configuration.

**Acceptance Criteria:**
- AC-4.1: New `cmux` config section in WORKFLOW.md YAML front matter:
  ```yaml
  cmux:
    enabled: true
    workspace_name: "Symphony"
    close_delay_ms: 30000
  ```
- AC-4.2: `cmux.enabled` defaults to `false` — opt-in only
- AC-4.3: Configuration is hot-reloadable — toggling `cmux.enabled` takes effect on next dispatch (does not affect running agents)
- AC-4.4: When `cmux.enabled: true` but cmux is unavailable, Symphony logs a warning once and falls back silently

### FR-5: Process Control via cmux

**What:** Symphony maintains its existing process lifecycle management; cmux surfaces are display-only.

**Acceptance Criteria:**
- AC-5.1: Agent process start/stop is managed via Go's `exec.Command` as today — cmux is not in the process lifecycle
- AC-5.2: Stall detection continues to work via the existing event channel — the log file is a mirror, not the source of truth
- AC-5.3: Turn completion and retry scheduling work identically to non-cmux mode
- AC-5.4: When a running agent is cancelled (context cancellation), a final log line is written: `[SYMPHONY] Agent cancelled`

## 4. Out of Scope

- Interactive user input to agent sessions via cmux
- Custom cmux layout management (pane splitting, window arrangement) — uses default tab layout
- cmux-based dashboard replacement (the HTTP dashboard remains the aggregate view)
- Windows/Linux support (cmux is macOS-only)
- Log formatting beyond timestamp + raw event (no pretty-printing of JSON)

## 5. Technical Constraints

- cmux communicates via Unix socket at `/tmp/cmux.sock`
- cmux CLI binary is at `/Applications/cmux.app/Contents/Resources/bin/cmux` (or in PATH)
- Each cmux CLI invocation is a subprocess call — should be used sparingly (workspace/surface lifecycle only, not per-event)
- Writing to the log file is the hot path — must use buffered I/O and not block the agent event loop
- The `tail -f` command in the cmux surface handles real-time display; Symphony only writes to the file

## 6. Mapping: Functional Requirements to Implementation

| FR | Primary Area | Notes |
|----|-------------|-------|
| FR-1 | New `internal/cmux/` package | Workspace create/reuse/detect via cmux CLI |
| FR-2 | `internal/cmux/` + orchestrator | Surface CRUD tied to dispatch/completion lifecycle |
| FR-3 | `internal/agent/runner.go`, `claude_runner.go` | Write events to log file alongside existing pipe processing |
| FR-4 | `internal/config/` | New CmuxConfig struct + defaults + validation |
| FR-5 | No changes needed | Existing process lifecycle is preserved as-is |
