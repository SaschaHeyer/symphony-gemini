# Step 3: Surface Lifecycle + Log Writing

**Covers:** FR-2 (Agent Surface Lifecycle), FR-3 (Agent Event Streaming to Log)
**Packages:** `internal/cmux`

---

## 1. Tasks

### 1.1 Log writing

- [ ] Create `internal/cmux/log.go` with:
  ```go
  // timePrefix returns "[HH:MM:SS] " for the current time.
  func timePrefix() string

  // WriteEvent writes a timestamped event line to the issue's log file.
  func (m *Manager) WriteEvent(issueID string, content string)

  // WriteAnnotation writes a timestamped Symphony annotation line.
  func (m *Manager) WriteAnnotation(issueID string, message string)

  // LogWriter returns an io.Writer for the issue's log file.
  // Returns io.Discard if cmux is disabled or no log file exists for the issue.
  func (m *Manager) LogWriter(issueID string) io.Writer
  ```
- [ ] `WriteEvent` implementation:
  - Lock mutex, get logFile from `m.logFiles[issueID]`, unlock
  - If nil, return silently (no log file yet — surface not created)
  - Write: `fmt.Fprintf(f, "[%s] %s\n", timePrefix(), content)`
- [ ] `WriteAnnotation` implementation:
  - Same as WriteEvent but format: `[%s] [SYMPHONY] %s\n`
- [ ] `LogWriter` implementation:
  - If disabled or no logFile for issueID, return `io.Discard`
  - Return a `timestampWriter` wrapper that prepends `[HH:MM:SS] ` to each line written
- [ ] Implement `timestampWriter` struct wrapping `*os.File`:
  ```go
  type timestampWriter struct {
      f  *os.File
      mu *sync.Mutex
  }
  func (w *timestampWriter) Write(p []byte) (int, error)
  ```
  - Prepends timestamp to each line in `p` (split on `\n`)

### 1.2 Surface lifecycle

- [ ] Create `internal/cmux/surface.go` with full `CreateSurface` implementation:
  1. Lock mutex
  2. If `m.surfaces[issueID]` already exists, unlock and return nil (reuse on retry)
  3. Create log file: `os.OpenFile(filepath.Join(workspacePath, ".symphony-agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)`
  4. Store in `m.logFiles[issueID]`
  5. Unlock mutex
  6. Call `m.run("new-surface", "--type", "terminal", "--workspace", m.workspaceRef)` — parse surface ref
  7. Call `m.run("rename-tab", "--surface", surfaceRef, identifier)`
  8. Build tail command: `fmt.Sprintf("tail -f %s\n", logPath)`
  9. Call `m.run("surface.send_text", tailCmd, "--surface", surfaceRef)`
  10. Lock, store `m.surfaces[issueID] = surfaceRef`, unlock

- [ ] Implement `CloseSurface(issueID string)`:
  1. Lock, get surfaceRef and logFile, unlock
  2. If no surface, return
  3. Write final annotation: `"Session ended"`
  4. Spawn goroutine:
     - Sleep `m.closeDelayMs`
     - Call `m.run("close-surface", "--surface", surfaceRef)`
     - Lock, close logFile, delete from both maps, unlock

- [ ] Implement `Shutdown()`:
  1. Lock mutex
  2. For each surface: call `m.run("close-surface", "--surface", ref)` (best-effort, ignore errors)
  3. For each logFile: close file handle
  4. Clear both maps
  5. Unlock

### 1.3 Tests

- [ ] Create `internal/cmux/log_test.go`:
  - `TestWriteEvent` — create temp file, call WriteEvent, verify line written with timestamp
  - `TestWriteAnnotation` — verify `[SYMPHONY]` prefix in output
  - `TestWriteEventNoFile` — call WriteEvent with unknown issueID, verify no panic
  - `TestWriteEventConcurrent` — 10 goroutines writing 100 events each, verify no data corruption (all lines complete)
  - `TestLogWriter` — write bytes via LogWriter, verify timestamped lines in file
  - `TestLogWriterDisabled` — disabled manager returns io.Discard

- [ ] Create `internal/cmux/surface_test.go`:
  - `TestCreateSurface` — with mock cmux: verify new-surface, rename-tab, surface.send_text calls in order
  - `TestCreateSurfaceCreatesLogFile` — verify `.symphony-agent.log` file created at expected path
  - `TestCreateSurfaceReuse` — call twice with same issueID, verify only one new-surface call
  - `TestCreateSurfaceFailure` — mock that fails on new-surface: verify error returned, log file still cleaned up
  - `TestCloseSurface` — verify close-surface called after delay, file handle closed, maps cleaned
  - `TestCloseSurfaceUnknownID` — call with unknown issueID, no panic
  - `TestShutdown` — create 3 surfaces, call Shutdown, verify all close-surface calls made and files closed

## 2. Definition of Done

- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/cmux/...` passes
- [ ] Can create surfaces, write events to log files, and close surfaces with mock cmux
- [ ] Concurrent log writing is safe
- [ ] Surface reuse on retry works correctly
