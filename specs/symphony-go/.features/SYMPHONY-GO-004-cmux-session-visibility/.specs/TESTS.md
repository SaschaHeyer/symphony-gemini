# TEST SPEC: cmux Session Visibility

**Status:** Draft v1
**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-004
**Depends on:** FUNCTIONAL.md v1, TECH.md v1

---

## 1. Testing Strategy

- **Unit tests** cover the cmux Manager, log writing, and config parsing in isolation
- **Mock cmux binary**: Tests use a shell script that records invocations to a log file, avoiding dependency on the real cmux app
- **No E2E tests**: cmux is macOS-only and optional — live cmux testing is manual only
- **Existing tests must not break**: cmux is additive; all existing orchestrator/agent tests pass unchanged

## 2. Unit Tests

### 2.1 Config (`internal/config`)

| Test | Why |
|------|-----|
| `TestParseCmuxConfig` | Verify CmuxConfig is parsed from YAML front matter correctly |
| `TestCmuxDefaults` | Verify defaults: enabled=false, workspace_name="Symphony", close_delay_ms=30000 |
| `TestCmuxDefaultsPreserved` | When cmux section is partially specified, unset fields keep defaults |
| `TestCmuxOmitted` | When cmux section is entirely absent, all defaults apply |

### 2.2 CmuxManager (`internal/cmux`)

| Test | Why |
|------|-----|
| `TestNewDisabled` | When enabled=false, New() returns a manager where all methods are no-ops |
| `TestNewBinaryNotFound` | When cmux binary doesn't exist, manager disables itself and logs warning |
| `TestInitPingFails` | When cmux ping fails, Init() returns error and manager disables itself |
| `TestInitCreatesWorkspace` | Init() calls `cmux new-workspace` and stores workspace ref |
| `TestInitReusesWorkspace` | When workspace already exists in `list-workspaces`, Init() reuses it |
| `TestCreateSurface` | CreateSurface() calls `new-surface`, `rename-tab`, `surface.send_text` with correct args |
| `TestCreateSurfaceReuse` | If surface already exists for issueID (retry), no new surface is created |
| `TestCreateSurfaceFailure` | If cmux new-surface fails, error is returned but manager stays operational |
| `TestCloseSurface` | CloseSurface() writes final annotation, waits delay, calls `close-surface` |
| `TestCloseSurfaceDelay` | Verify the close delay is respected (use short delay in test) |
| `TestShutdown` | Shutdown() closes all surfaces and log files immediately (no delay) |

### 2.3 Log Writing (`internal/cmux`)

| Test | Why |
|------|-----|
| `TestWriteEvent` | WriteEvent() writes timestamped line to log file |
| `TestWriteAnnotation` | WriteAnnotation() writes `[SYMPHONY]`-prefixed line |
| `TestWriteEventDisabled` | When manager is disabled, WriteEvent() is a no-op (no file writes) |
| `TestLogWriter` | LogWriter() returns an io.Writer that writes to the correct file |
| `TestLogWriterDisabled` | When disabled, LogWriter() returns io.Discard |
| `TestWriteEventConcurrent` | Multiple goroutines writing events concurrently don't corrupt the file |
| `TestLogFileCreatedOnCreateSurface` | Log file is created in append mode at the expected path |

### 2.4 cmux CLI Exec (`internal/cmux`)

| Test | Why |
|------|-----|
| `TestRunTimeout` | If cmux CLI hangs, the 5-second timeout kills the process |
| `TestRunParsesSurfaceRef` | Verify parsing of `new-surface` output to extract surface ref |

### 2.5 Orchestrator Integration (`internal/orchestrator`)

| Test | Why |
|------|-----|
| `TestDispatchCallsCreateSurface` | When cmux enabled, dispatchIssue() calls CreateSurface before launching worker |
| `TestHandleEventWritesEvent` | EventAgentUpdate triggers WriteEvent() on the cmux manager |
| `TestWorkerDoneClosesSurface` | EventWorkerDone triggers CloseSurface() with annotation |
| `TestWorkerFailedClosesSurface` | EventWorkerFailed triggers CloseSurface() with annotation |
| `TestShutdownCleansCmux` | Orchestrator shutdown calls cmuxMgr.Shutdown() |
| `TestDispatchWithoutCmux` | When cmux disabled, dispatch works identically to before (no panics, no calls) |

### 2.6 Agent Runner (`internal/agent`)

| Test | Why |
|------|-----|
| `TestClaudeRunnerWritesRawEvents` | When EventLogWriter is set, raw NDJSON lines are written to it |
| `TestGeminiRunnerWritesRawEvents` | When EventLogWriter is set, ACP messages are written to it |
| `TestRunnerNilLogWriter` | When EventLogWriter is nil, no writes attempted (no panic) |

## 3. Mock cmux Binary

Tests use a mock script that records invocations:

```bash
#!/bin/bash
# Mock cmux binary for testing
echo "$@" >> "${CMUX_TEST_LOG:-/tmp/cmux-test-calls.log}"

case "$1" in
  ping)        echo "OK" ;;
  version)     echo "cmux 0.61.0 (73)" ;;
  new-workspace) echo "OK workspace:1" ;;
  new-surface)   echo "OK surface:1 pane:1 workspace:1" ;;
  list-workspaces) echo "[]" ;;  # or pre-configured JSON
  close-surface) echo "OK" ;;
  rename-tab)    echo "OK" ;;
  surface.send_text) echo "OK" ;;
  *) echo "OK" ;;
esac
```

The mock binary path is injected into the Manager via a test helper, replacing the real cmux binary resolution.

## 4. Test File Layout

```
internal/cmux/
├── manager_test.go      — New, Init, Shutdown tests
├── surface_test.go      — CreateSurface, CloseSurface tests
├── log_test.go          — WriteEvent, WriteAnnotation, LogWriter tests
├── exec_test.go         — run() timeout and parsing tests
└── testdata/
    └── mock_cmux.sh     — mock cmux binary

internal/config/
└── config_test.go       — add TestParseCmuxConfig, TestCmuxDefaults (to existing file)

internal/orchestrator/
└── orchestrator_test.go — add cmux integration tests (to existing file or new cmux_test.go)

internal/agent/
├── runner_test.go       — add TestGeminiRunnerWritesRawEvents
└── claude_runner_test.go — add TestClaudeRunnerWritesRawEvents
```

## 5. Coverage Expectations

- **`internal/cmux/`**: >90% line coverage — new package, full control
- **`internal/config/`**: existing coverage maintained + new CmuxConfig tests
- **`internal/orchestrator/`**: existing coverage maintained + cmux integration paths
- **`internal/agent/`**: existing coverage maintained + EventLogWriter paths
