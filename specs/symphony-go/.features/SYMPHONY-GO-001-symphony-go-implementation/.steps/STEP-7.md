# Step 7: Orchestrator

**Covers:** FR-4 (Orchestrator)
**Package:** `internal/orchestrator`

---

## 1. Tasks

### 1.1 State

- [ ] `internal/orchestrator/state.go`:
  - `State` struct per TECH.md Section 2.5
  - `RunningEntry`, `RetryEntry`, `TokenTotals`, `RateLimitSnapshot` structs
  - `NewState(cfg *config.Config) *State` — initialize with config values and empty maps
  - `Snapshot() StateSnapshot` — returns read-consistent copy for HTTP API (under RWMutex read lock)
  - `StateSnapshot` struct: serializable copy of running/retry/totals for API consumers

### 1.2 Dispatch Logic

- [ ] `internal/orchestrator/dispatch.go`:
  - `SortForDispatch(issues []tracker.Issue) []tracker.Issue`:
    - Sort: priority ascending (nil last) → created_at oldest → identifier lexicographic
    - Use `slices.SortStableFunc`
  - `ShouldDispatch(issue *tracker.Issue, state *State, cfg *config.Config) bool`:
    - Has id, identifier, title, state
    - State in `active_states` (lowercase compare) and not in `terminal_states`
    - Not in `state.Running`
    - Not in `state.Claimed`
    - Global slots: `len(state.Running) < cfg.Agent.MaxConcurrentAgents`
    - Per-state slots: count running issues with same lowercase state < per-state limit (if configured)
    - Blocker rule: if state is `todo` (lowercase), reject if any blocker has non-terminal state
  - `DispatchIssue(issue *tracker.Issue, state *State, attempt *int, launcher AgentLauncher, ...) *State`:
    - Create worker context with cancel
    - Launch goroutine: `go launcher.Launch(workerCtx, params, eventCh)`
    - Add to `state.Running` and `state.Claimed`
    - Remove from `state.RetryAttempts` if present

### 1.3 Reconciliation

- [ ] `internal/orchestrator/reconcile.go`:
  - `ReconcileRunningIssues(ctx context.Context, state *State, tracker TrackerClient, cfg *config.Config) *State`:
    1. **Stall detection:**
       - For each running entry, compute elapsed since `LastEventAt` (or `StartedAt` if no events)
       - If elapsed > `cfg.Gemini.StallTimeoutMs` and stall detection enabled (> 0): cancel worker, schedule retry
    2. **Tracker state refresh:**
       - Collect all running issue IDs
       - If empty → return (no-op)
       - `tracker.FetchIssueStatesByIDs(ctx, ids)`
       - On fetch error → log, keep workers running, return
       - For each refreshed issue:
         - Terminal state → cancel worker, clean workspace
         - Active state → update `running[id].Issue`
         - Neither → cancel worker, no cleanup

### 1.4 Retry Logic

- [ ] `internal/orchestrator/retry.go`:
  - `ScheduleRetry(state *State, issueID string, attempt int, identifier string, err string, delayCfg DelayConfig) *State`:
    - Cancel existing retry timer for same issue (if any)
    - Compute delay:
      - Continuation: `1000ms` fixed
      - Failure: `min(10000 * 2^(attempt-1), cfg.Agent.MaxRetryBackoffMs)`
    - Create timer goroutine: `time.AfterFunc(delay, func() { retryFiredCh <- RetryFire{...} })`
    - Store `RetryEntry` in `state.RetryAttempts`
  - `HandleRetryFire(ctx context.Context, rf RetryFire, state *State, tracker TrackerClient, launcher AgentLauncher, cfg *config.Config) *State`:
    - Pop retry entry
    - Fetch candidate issues
    - On fetch error → reschedule with incremented attempt
    - Find issue by ID in candidates
    - Not found → release claim (remove from `Claimed`)
    - Found + slots available → `DispatchIssue`
    - Found + no slots → reschedule with error "no available orchestrator slots"

### 1.5 Metrics

- [ ] `internal/orchestrator/metrics.go`:
  - `AddRuntimeSeconds(state *State, entry *RunningEntry)`:
    - Compute `time.Since(entry.StartedAt).Seconds()`
    - Add to `state.GeminiTotals.SecondsRunning`
  - `UpdateTokens(state *State, issueID string, usage *TokenUsage)`:
    - Compute delta from `LastReported*` fields
    - Add delta to running entry's counters
    - Add delta to `state.GeminiTotals`
    - Update `LastReported*` fields

### 1.6 Orchestrator Main Loop

- [ ] `internal/orchestrator/orchestrator.go`:
  - `Orchestrator` struct per TECH.md Section 4.2
  - `New(cfg *config.Config, tracker TrackerClient, launcher AgentLauncher, workspaceMgr *workspace.Manager) *Orchestrator`
  - `Run(ctx context.Context) error`:
    - Startup validation → fail if invalid
    - Startup terminal cleanup: `tracker.FetchIssuesByStates(terminal)` → `workspaceMgr.CleanWorkspace` each
    - Immediate tick
    - Select loop per TECH.md Section 4.3:
      - `ticker.C` → `tick(ctx)`
      - `events` channel → `handleEvent`
      - `retryFired` channel → `HandleRetryFire`
      - `reloadCh` → `applyConfig` (update ticker, limits, all settings)
      - `refreshCh` → `tick(ctx)` (manual trigger)
      - `ctx.Done()` → shutdown
  - `tick(ctx)`:
    1. `ReconcileRunningIssues`
    2. `ValidateDispatchConfig` — skip dispatch on failure
    3. `tracker.FetchCandidateIssues` — skip dispatch on failure
    4. `SortForDispatch(issues)`
    5. Loop: `ShouldDispatch` → `DispatchIssue` until no slots
  - `handleEvent(ev)`:
    - `EventWorkerDone`: remove from Running, add runtime seconds, schedule continuation retry (attempt=1, 1s)
    - `EventWorkerFailed`: remove from Running, add runtime seconds, schedule exponential retry
    - `EventAgentUpdate`: update running entry fields (last event, message, timestamp, tokens, rate limits)
  - `applyConfig(cfg)`:
    - Update `state.PollIntervalMs`, `state.MaxConcurrentAgents`
    - Reset ticker interval: `ticker.Reset(newInterval)`
    - Update all other config references (gemini settings, hooks, etc.)
  - `shutdown()`:
    - Cancel all worker contexts
    - Wait for workers with timeout (5s)
    - Run `after_run` for interrupted workers

### 1.7 Tests

- [ ] `internal/orchestrator/dispatch_test.go`:
  - Test: sort by priority ascending, null last
  - Test: sort by created_at oldest first (same priority)
  - Test: sort by identifier (same priority + time)
  - Test: skip already running
  - Test: skip already claimed
  - Test: global concurrency limit respected
  - Test: per-state concurrency limit respected
  - Test: Todo + non-terminal blocker → not dispatched
  - Test: Todo + all-terminal blockers → dispatched
  - Test: missing required fields → skipped

- [ ] `internal/orchestrator/reconcile_test.go`:
  - Test: terminal state → cancel + cleanup
  - Test: active state → update snapshot
  - Test: neither active nor terminal → cancel, no cleanup
  - Test: no running → no-op
  - Test: state refresh failure → keep workers
  - Test: stall detected → cancel + retry
  - Test: stall disabled (<=0) → no stall check

- [ ] `internal/orchestrator/retry_test.go`:
  - Test: normal exit → continuation retry, attempt=1, delay≈1s
  - Test: abnormal exit → exponential backoff
  - Test: backoff capped at max
  - Test: retry fire → issue active → dispatch
  - Test: retry fire → issue gone → release claim
  - Test: retry fire → no slots → requeue
  - Test: existing timer cancelled on new retry

- [ ] `internal/orchestrator/metrics_test.go`:
  - Test: token delta tracking avoids double-count
  - Test: runtime seconds added on worker exit
  - Test: snapshot includes live elapsed time

---

## 2. Dependencies

No new dependencies. Composes all `internal/` packages.

---

## 3. Definition of Done

- [ ] `go test ./internal/orchestrator/...` — all pass
- [ ] Orchestrator poll loop drives dispatch, reconciliation, retry correctly
- [ ] All state mutations happen in the single orchestrator goroutine
- [ ] Config reload updates ticker and limits
- [ ] Graceful shutdown cancels workers and waits
