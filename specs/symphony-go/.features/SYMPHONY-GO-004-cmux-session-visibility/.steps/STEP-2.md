# Step 2: cmux CLI Exec + Workspace Management

**Covers:** FR-1 (cmux Workspace Management)
**Packages:** `internal/cmux`

---

## 1. Tasks

### 1.1 cmux CLI exec helper

- [ ] Create `internal/cmux/exec.go` with:
  ```go
  // run executes a cmux CLI command with a 5-second timeout.
  // Returns trimmed stdout output.
  func (m *Manager) run(args ...string) (string, error)
  ```
  - Use `exec.CommandContext` with 5-second timeout
  - Set command to `m.cmuxBin` with given args
  - Return `strings.TrimSpace(string(output))`, err
- [ ] Add helper `parseRef(output, prefix string) string` to extract refs from cmux output:
  - `"OK workspace:5 ..."` → `"workspace:5"` when prefix is `"workspace:"`
  - `"OK surface:12 pane:3 workspace:5"` → `"surface:12"` when prefix is `"surface:"`
  - Splits output on spaces, finds token starting with prefix

### 1.2 Mock cmux binary for tests

- [ ] Create `internal/cmux/testdata/mock_cmux.sh`:
  ```bash
  #!/bin/bash
  echo "$@" >> "${CMUX_TEST_LOG}"
  case "$1" in
    ping)             echo "OK" ;;
    version)          echo "cmux 0.61.0 (73)" ;;
    new-workspace)    echo "OK workspace:1" ;;
    list-workspaces)  echo "[]" ;;
    rename-tab)       echo "OK" ;;
    new-surface)      echo "OK surface:1 pane:1 workspace:1" ;;
    close-surface)    echo "OK" ;;
    surface.send_text) echo "OK" ;;
    *)                echo "OK" ;;
  esac
  ```
- [ ] Add test helper `newTestManager(t *testing.T) (*Manager, string)`:
  - Creates temp dir for test log
  - Returns Manager with `cmuxBin` pointed at mock script
  - Sets `CMUX_TEST_LOG` env var to capture invocations

### 1.3 Init() implementation

- [ ] Implement `Init()` in `internal/cmux/manager.go`:
  1. Call `m.run("ping")` — if error, log warning, set `m.enabled = false`, return error
  2. Call `m.run("list-workspaces", "--json")` — parse JSON array, look for workspace matching `m.workspaceName`
  3. If found: store its ref in `m.workspaceRef`
  4. If not found: call `m.run("new-workspace")` — parse output for workspace ref, store in `m.workspaceRef`
  5. Call `m.run("rename-tab", "--workspace", m.workspaceRef, m.workspaceName)` to set workspace tab name
  6. Log info: "cmux workspace initialized" with workspace ref

### 1.4 Tests

- [ ] Create `internal/cmux/exec_test.go`:
  - `TestRunSuccess` — run mock cmux with "ping", verify "OK" returned
  - `TestRunTimeout` — create a mock that sleeps 10s, verify timeout error within ~5s
  - `TestParseRef` — test ref extraction from various output formats
- [ ] Add to `internal/cmux/manager_test.go`:
  - `TestInitSuccess` — Init() with mock binary: verify ping, new-workspace, rename-tab calls logged
  - `TestInitPingFails` — mock binary that exits 1 on ping: verify Init() returns error and disables manager
  - `TestInitReusesWorkspace` — mock that returns a workspace in list-workspaces: verify no new-workspace call

## 2. Definition of Done

- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/cmux/...` passes
- [ ] Init() successfully creates workspace with mock cmux binary
- [ ] Init() gracefully handles cmux unavailability
