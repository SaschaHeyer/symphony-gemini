# Step 3: Workspace Manager

**Covers:** FR-5 (Workspace Manager)
**Package:** `internal/workspace`

---

## 1. Tasks

### 1.1 Safety Utilities

- [ ] `internal/workspace/safety.go`:
  - `SanitizeIdentifier(identifier string) string`:
    - Replace any char not in `[A-Za-z0-9._-]` with `_`
  - `ValidateWorkspacePath(workspacePath, workspaceRoot string) error`:
    - Resolve both to absolute paths via `filepath.Abs`
    - Check `workspacePath` has `workspaceRoot` as directory prefix using `filepath.Rel` or string prefix after cleaning
    - Reject path traversal

- [ ] `internal/workspace/safety_test.go`:
  - Test: `ABC-123` → `ABC-123` (no change)
  - Test: `foo/bar` → `foo_bar`
  - Test: `a b` → `a_b`
  - Test: `café` → `caf_`
  - Test: path inside root → passes
  - Test: path outside root → error
  - Test: path traversal `../` → error

### 1.2 Hook Execution

- [ ] `internal/workspace/hooks.go`:
  - `RunHook(name string, script string, cwd string, timeoutMs int) error`:
    - Execute `sh -lc <script>` with `cwd` as working directory
    - Apply timeout via `context.WithTimeout`
    - On timeout: kill process, return timeout error
    - Capture stdout/stderr for logging (truncate if large)
    - Return exit code error on non-zero exit
  - Semantics per hook name (enforced by caller, not this function):
    - `after_create`: failure is fatal (caller returns error)
    - `before_run`: failure is fatal (caller returns error)
    - `after_run`: caller logs and ignores error
    - `before_remove`: caller logs and ignores error

- [ ] `internal/workspace/hooks_test.go`:
  - Test: successful hook returns nil
  - Test: failing hook (`exit 1`) returns error
  - Test: hook timeout kills process and returns error
  - Test: hook runs with correct cwd (script that writes a file, check path)
  - Test: hook with nil/empty script is no-op

### 1.3 Workspace Manager

- [ ] `internal/workspace/manager.go`:
  - `Manager` struct: `root string`, `hooksConfig *config.HooksConfig`
  - `NewManager(root string, hooks *config.HooksConfig) *Manager`
  - `CreateForIssue(identifier string) (*Workspace, error)`:
    1. `key := SanitizeIdentifier(identifier)`
    2. `path := filepath.Join(m.root, key)`
    3. `ValidateWorkspacePath(path, m.root)` → error if outside root
    4. `os.MkdirAll(path, 0755)` — check if dir existed before call
    5. If newly created (`CreatedNow=true`) and `after_create` hook configured → run hook. On failure: remove dir, return error.
    6. Return `&Workspace{Path: path, WorkspaceKey: key, CreatedNow: created}`
  - `CleanWorkspace(identifier string) error`:
    1. `key := SanitizeIdentifier(identifier)`
    2. `path := filepath.Join(m.root, key)`
    3. `ValidateWorkspacePath(path, m.root)` — safety check even on delete
    4. If `before_remove` hook configured and dir exists → run hook (log+ignore failure)
    5. `os.RemoveAll(path)`
  - `RunBeforeRun(workspacePath string) error`:
    - If `before_run` configured → `RunHook(...)`, return error on failure
  - `RunAfterRun(workspacePath string)`:
    - If `after_run` configured → `RunHook(...)`, log and ignore error

- [ ] `internal/workspace/manager_test.go`:
  - Test: deterministic path for same identifier
  - Test: creates dir when missing, `CreatedNow=true`
  - Test: reuses existing dir, `CreatedNow=false`
  - Test: `after_create` runs only on new creation
  - Test: `after_create` NOT run on reuse
  - Test: `after_create` failure removes dir and returns error
  - Test: `before_run` failure returns error
  - Test: `after_run` failure does not return error
  - Test: `CleanWorkspace` removes directory
  - Test: `CleanWorkspace` runs `before_remove` hook
  - Test: workspace path outside root rejected

---

## 2. Dependencies

No new dependencies. Uses `os`, `os/exec`, `filepath`, `context` from stdlib.

---

## 3. Definition of Done

- [ ] `go test ./internal/workspace/...` — all pass
- [ ] Workspace creation/reuse is deterministic and safe
- [ ] Hooks execute with correct cwd and timeout
- [ ] Path containment enforced on create and delete
