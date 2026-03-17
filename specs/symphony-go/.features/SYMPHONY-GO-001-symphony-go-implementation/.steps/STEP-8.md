# Step 8: CLI Entrypoint + HTTP Server + Integration Tests

**Covers:** FR-9 (CLI), EXT-1 (HTTP Server), Integration Tests
**Packages:** `cmd/symphony`, `internal/server`, `test/`

---

## 1. Tasks

### 1.1 CLI Entrypoint

- [ ] `cmd/symphony/main.go`:
  - Parse arguments:
    - Positional arg 1: path to WORKFLOW.md (optional, default `./WORKFLOW.md`)
    - `--port` flag: HTTP server port (optional)
    - `--version` flag: print version and exit
  - Startup sequence:
    1. `logging.Setup()`
    2. Resolve workflow path (explicit or default)
    3. Check file exists → exit nonzero with clear message if not
    4. `workflow.LoadWorkflow(path)`
    5. `config.ParseConfig(wf.Config)` → `config.ResolveConfig(cfg)`
    6. `config.ValidateDispatchConfig(cfg)` → exit nonzero on failure
    7. Create components: `tracker.NewLinearClient(...)`, `workspace.NewManager(...)`, `agent.NewGeminiRunner(...)`, `orchestrator.New(...)`
    8. Start workflow watcher: `workflow.WatchWorkflow(path, orchestrator.reloadCh)`
    9. If port configured (CLI flag or `server.port`): start HTTP server
    10. `orchestrator.Run(ctx)` — blocks until shutdown
  - Signal handling:
    - Catch `SIGINT`, `SIGTERM` → cancel context
    - Orchestrator shutdown propagates to workers
  - Exit codes:
    - 0: normal shutdown
    - 1: startup failure
    - 1: abnormal runtime exit

### 1.2 HTTP Server

- [ ] `internal/server/server.go`:
  - `Server` struct: `port int`, `orchestrator *orchestrator.Orchestrator`, `httpServer *http.Server`
  - `New(port int, orch *orchestrator.Orchestrator) *Server`
  - `Start() error`:
    - Bind `127.0.0.1:<port>`
    - Port 0 → ephemeral (log actual bound port)
    - Register routes
    - `go httpServer.ListenAndServe()`
  - `Shutdown(ctx context.Context) error`

- [ ] `internal/server/api.go`:
  - `GET /api/v1/state`:
    - Call `orchestrator.Snapshot()`
    - Marshal to JSON per SPEC Section 13.7.2 shape:
      ```json
      {
        "generated_at": "...",
        "counts": {"running": N, "retrying": N},
        "running": [...],
        "retrying": [...],
        "codex_totals": {...},
        "rate_limits": null
      }
      ```
    - Running entries include `turn_count`
  - `GET /api/v1/{identifier}`:
    - Look up by identifier in running + retry state
    - Found → return issue detail JSON per spec
    - Not found → `404` with `{"error":{"code":"issue_not_found","message":"..."}}`
  - `POST /api/v1/refresh`:
    - Send signal to orchestrator's `refreshCh`
    - Return `202 Accepted` with `{"queued":true,"requested_at":"..."}`
  - Unsupported methods on defined routes → `405 Method Not Allowed`
  - All error responses use `{"error":{"code":"...","message":"..."}}` envelope

- [ ] `internal/server/dashboard.go`:
  - `GET /`:
    - Serve a simple server-rendered HTML page
    - Show: running sessions (table), retry queue, token totals, rate limits
    - Auto-refresh via `<meta http-equiv="refresh" content="5">`
    - Minimal CSS, no JS framework
    - Data sourced from `orchestrator.Snapshot()`

### 1.3 Integration Tests

- [ ] `test/testutil/mock_tracker.go`:
  - `MockTracker` implementing `TrackerClient`:
    - Configurable canned responses for each method
    - Tracks call counts and arguments for assertions

- [ ] `test/testutil/mock_agent.go`:
  - `MockAgentLauncher` implementing `AgentLauncher`:
    - Configurable behavior: succeed after N ms, fail with error, block until cancelled
    - Emits configurable events to eventCh
    - Tracks launch calls

- [ ] `test/testutil/fixtures.go`:
  - Helper functions to create test issues, configs, workflows
  - `TestIssue(id, identifier, state string) *tracker.Issue`
  - `TestConfig() *config.Config` — valid minimal config
  - `TestWorkflow(prompt string) *workflow.WorkflowDefinition`

- [ ] `internal/orchestrator/orchestrator_integration_test.go`:
  - Test: **Dispatch flow** — orchestrator tick with mock tracker (2 candidate issues) + mock agent → dispatches both, receives completion events, schedules continuation retries
  - Test: **Config reload** — change poll interval via `reloadCh`, verify ticker updates
  - Test: **Invalid reload** — send bad config via `reloadCh`, verify last good config kept
  - Test: **Graceful shutdown** — start orchestrator with running mock worker, send cancel, verify worker cancelled and `after_run` called

- [ ] `internal/server/api_test.go`:
  - Test: `GET /api/v1/state` returns valid JSON with expected shape
  - Test: `GET /api/v1/MT-123` for running issue returns detail
  - Test: `GET /api/v1/UNKNOWN` returns 404
  - Test: `POST /api/v1/refresh` returns 202
  - Test: `DELETE /api/v1/state` returns 405

### 1.4 E2E Test Stubs

- [ ] `test/e2e/linear_live_test.go`:
  - `TestLinearFetchCandidates` — skip if `LINEAR_API_KEY` not set
  - `TestLinearFetchStatesByIDs` — skip if no key
  - Use real `LinearClient` against live API with a test project slug

- [ ] `test/e2e/gemini_live_test.go`:
  - `TestGeminiACPHandshake` — skip if `gemini` not on PATH
  - Launch real `gemini --experimental-acp`, do initialize + session/new + simple prompt, verify response
  - Kill subprocess after test
  - Use temp directory as workspace

---

## 2. Dependencies

No new dependencies. `flag` (stdlib) for CLI arg parsing, `net/http` for server.

---

## 3. Definition of Done

- [ ] `go build ./cmd/symphony` produces working binary
- [ ] `./symphony` with no WORKFLOW.md → clear error, exit 1
- [ ] `./symphony path/to/WORKFLOW.md` → starts orchestrator
- [ ] `./symphony --port 8080` → starts with HTTP server
- [ ] SIGINT/SIGTERM → graceful shutdown, exit 0
- [ ] `go test ./...` — all unit + integration tests pass
- [ ] E2E tests skip gracefully when credentials unavailable
- [ ] HTTP API returns correct JSON shapes
- [ ] Dashboard renders basic HTML with live data
