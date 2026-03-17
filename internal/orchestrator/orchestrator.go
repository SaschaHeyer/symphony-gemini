package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/symphony-go/symphony/internal/agent"
	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/tracker"
	"github.com/symphony-go/symphony/internal/workflow"
	"github.com/symphony-go/symphony/internal/workspace"
)

// Orchestrator owns the poll loop, dispatch, reconciliation, and retry logic.
type Orchestrator struct {
	state        *State
	cfg          *config.Config
	wf           *workflow.WorkflowDefinition
	cfgMu        sync.RWMutex
	tracker      tracker.TrackerClient
	launcher     agent.AgentLauncher
	workspaceMgr *workspace.Manager

	events    chan agent.OrchestratorEvent
	reloadCh  chan ReloadPayload
	refreshCh chan struct{}
}

// ReloadPayload carries new config + workflow from file watcher.
type ReloadPayload struct {
	Config   *config.Config
	Workflow *workflow.WorkflowDefinition
}

// New creates a new Orchestrator.
func New(
	cfg *config.Config,
	wf *workflow.WorkflowDefinition,
	trackerClient tracker.TrackerClient,
	launcher agent.AgentLauncher,
	workspaceMgr *workspace.Manager,
) *Orchestrator {
	state := NewState(cfg.Polling.IntervalMs, cfg.Agent.MaxConcurrentAgents)
	state.GeminiModel = cfg.Gemini.Model
	state.GeminiCommand = cfg.Gemini.Command
	state.ProjectSlug = cfg.Tracker.ProjectSlug

	return &Orchestrator{
		state:        state,
		cfg:          cfg,
		wf:           wf,
		tracker:      trackerClient,
		launcher:     launcher,
		workspaceMgr: workspaceMgr,
		events:       make(chan agent.OrchestratorEvent, 100),
		reloadCh:     make(chan ReloadPayload, 1),
		refreshCh:    make(chan struct{}, 1),
	}
}

// ReloadCh returns the channel for sending config reloads.
func (o *Orchestrator) ReloadCh() chan<- ReloadPayload {
	return o.reloadCh
}

// RefreshCh returns the channel for triggering manual refreshes.
func (o *Orchestrator) RefreshCh() chan<- struct{} {
	return o.refreshCh
}

// Snapshot returns a read-consistent snapshot of the state.
func (o *Orchestrator) Snapshot() StateSnapshot {
	return o.state.Snapshot()
}

// FindIssueByIdentifier finds a running or retrying issue by identifier.
func (o *Orchestrator) FindIssueByIdentifier(identifier string) (*RunningEntry, *RetryEntry) {
	o.state.mu.RLock()
	defer o.state.mu.RUnlock()

	for _, entry := range o.state.Running {
		if entry.Identifier == identifier {
			return entry, nil
		}
	}
	for _, entry := range o.state.RetryAttempts {
		if entry.Identifier == identifier {
			return nil, entry
		}
	}
	return nil, nil
}

// Run starts the orchestrator main loop. Blocks until ctx is cancelled.
func (o *Orchestrator) Run(ctx context.Context) error {
	// Startup validation
	if err := config.ValidateDispatchConfig(o.getConfig()); err != nil {
		return fmt.Errorf("startup validation failed: %w", err)
	}

	// Startup terminal cleanup
	o.startupTerminalCleanup()

	ticker := time.NewTicker(time.Duration(o.state.PollIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	// Immediate first tick
	o.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			o.shutdown()
			return nil

		case <-ticker.C:
			o.tick(ctx)

		case ev := <-o.events:
			o.handleEvent(ev)

		case reload := <-o.reloadCh:
			o.applyReload(reload, ticker)

		case <-o.refreshCh:
			o.tick(ctx)
		}
	}
}

func (o *Orchestrator) getConfig() *config.Config {
	o.cfgMu.RLock()
	defer o.cfgMu.RUnlock()
	return o.cfg
}

func (o *Orchestrator) getWorkflow() *workflow.WorkflowDefinition {
	o.cfgMu.RLock()
	defer o.cfgMu.RUnlock()
	return o.wf
}

func (o *Orchestrator) tick(ctx context.Context) {
	cfg := o.getConfig()

	// Reconcile first
	ReconcileRunningIssues(o.state, o.tracker, o.workspaceMgr, cfg,
		func(issueID, identifier, errMsg string, attempt int) {
			delay := ComputeBackoffDelay(attempt, false, cfg.Agent.MaxRetryBackoffMs)
			ScheduleRetry(o.state, issueID, identifier, attempt, errMsg, delay, func() {
				o.events <- agent.OrchestratorEvent{
					Type:    agent.EventWorkerFailed,
					IssueID: issueID,
					Payload: "retry_fire",
				}
			})
		},
	)

	// Validate config
	if err := config.ValidateDispatchConfig(cfg); err != nil {
		slog.Error("dispatch config validation failed, skipping dispatch", "error", err)
		return
	}

	// Fetch candidates
	issues, err := o.tracker.FetchCandidateIssues(cfg.Tracker.ProjectSlug, cfg.Tracker.ActiveStates)
	if err != nil {
		slog.Error("failed to fetch candidate issues", "error", err)
		return
	}

	// Sort
	SortForDispatch(issues)

	// Dispatch
	for i := range issues {
		if AvailableSlots(o.state, cfg.Agent.MaxConcurrentAgents) <= 0 {
			break
		}
		if ShouldDispatch(&issues[i], o.state, cfg) {
			o.dispatchIssue(ctx, &issues[i], nil, cfg)
		}
	}
}

func (o *Orchestrator) dispatchIssue(ctx context.Context, issue *tracker.Issue, attempt *int, cfg *config.Config) {
	workerCtx, cancel := context.WithCancel(ctx)

	entry := &RunningEntry{
		IssueID:    issue.ID,
		Identifier: issue.Identifier,
		Issue:      issue,
		Cancel:     cancel,
		StartedAt:  time.Now().UTC(),
		State:      issue.State,
	}
	if attempt != nil {
		entry.RetryAttempt = *attempt
	}

	o.state.Running[issue.ID] = entry
	o.state.Claimed[issue.ID] = struct{}{}
	delete(o.state.RetryAttempts, issue.ID)

	wf := o.getWorkflow()
	geminiCfg := cfg.Gemini
	agentCfg := cfg.Agent

	slog.Info("dispatching issue",
		"issue_id", issue.ID,
		"issue_identifier", issue.Identifier,
		"state", issue.State,
	)

	go func() {
		params := agent.RunParams{
			Issue:         issue,
			Attempt:       attempt,
			Workflow:      wf,
			GeminiCfg:     &geminiCfg,
			AgentCfg:      &agentCfg,
			ActiveStates:  cfg.Tracker.ActiveStates,
			WorkspaceMgr:  o.workspaceMgr,
			WorkspaceRoot: cfg.Workspace.Root,
			ExtraEnv:      []string{},
			CheckIssueState: func(ctx context.Context, issueID string) (string, error) {
				issues, err := o.tracker.FetchIssueStatesByIDs([]string{issueID})
				if err != nil {
					return "", err
				}
				if len(issues) == 0 {
					return "", fmt.Errorf("issue %s not found", issueID)
				}
				return issues[0].State, nil
			},
		}

		err := o.launcher.Launch(workerCtx, params, o.events)
		if err != nil {
			o.events <- agent.OrchestratorEvent{
				Type:    agent.EventWorkerFailed,
				IssueID: issue.ID,
				Payload: err.Error(),
			}
		} else {
			o.events <- agent.OrchestratorEvent{
				Type:    agent.EventWorkerDone,
				IssueID: issue.ID,
			}
		}
	}()
}

func (o *Orchestrator) handleEvent(ev agent.OrchestratorEvent) {
	cfg := o.getConfig()

	switch ev.Type {
	case agent.EventWorkerDone:
		entry, ok := o.state.Running[ev.IssueID]
		if ok {
			slog.Info("worker completed normally",
				"issue_id", ev.IssueID,
				"issue_identifier", entry.Identifier,
			)
			removeRunning(o.state, ev.IssueID)
			o.state.Completed[ev.IssueID] = struct{}{}

			// Schedule continuation retry
			delay := ComputeBackoffDelay(1, true, cfg.Agent.MaxRetryBackoffMs)
			ScheduleRetry(o.state, ev.IssueID, entry.Identifier, 1, "", delay, func() {
				o.handleRetryFire(ev.IssueID)
			})
		}

	case agent.EventWorkerFailed:
		entry, ok := o.state.Running[ev.IssueID]

		// Check if this is a retry fire (not from a running worker)
		if payload, isString := ev.Payload.(string); isString && payload == "retry_fire" {
			o.handleRetryFire(ev.IssueID)
			return
		}

		if ok {
			errMsg := ""
			if payload, isString := ev.Payload.(string); isString {
				errMsg = payload
			}
			slog.Error("worker failed",
				"issue_id", ev.IssueID,
				"issue_identifier", entry.Identifier,
				"error", errMsg,
			)

			attempt := entry.RetryAttempt + 1
			identifier := entry.Identifier
			removeRunning(o.state, ev.IssueID)

			delay := ComputeBackoffDelay(attempt, false, cfg.Agent.MaxRetryBackoffMs)
			ScheduleRetry(o.state, ev.IssueID, identifier, attempt, errMsg, delay, func() {
				o.handleRetryFire(ev.IssueID)
			})
		}

	case agent.EventAgentUpdate:
		if agentEvt, ok := ev.Payload.(agent.AgentEvent); ok {
			entry, exists := o.state.Running[ev.IssueID]
			if exists {
				now := agentEvt.Timestamp
				entry.LastEvent = agentEvt.Type
				entry.LastMessage = agentEvt.Message
				entry.LastEventAt = &now
				if agentEvt.SessionID != "" {
					entry.SessionID = agentEvt.SessionID
				}
				if agentEvt.Type == agent.EventTurnCompleted {
					entry.TurnCount++
				}
				if agentEvt.Usage != nil {
					UpdateTokens(o.state, ev.IssueID, agentEvt.Usage)
				}
			}
		}
	}
}

func (o *Orchestrator) handleRetryFire(issueID string) {
	cfg := o.getConfig()

	retryEntry, ok := o.state.RetryAttempts[issueID]
	if !ok {
		return
	}
	delete(o.state.RetryAttempts, issueID)

	// Fetch active candidates
	issues, err := o.tracker.FetchCandidateIssues(cfg.Tracker.ProjectSlug, cfg.Tracker.ActiveStates)
	if err != nil {
		slog.Error("retry fetch failed, rescheduling", "issue_id", issueID, "error", err)
		delay := ComputeBackoffDelay(retryEntry.Attempt+1, false, cfg.Agent.MaxRetryBackoffMs)
		ScheduleRetry(o.state, issueID, retryEntry.Identifier, retryEntry.Attempt+1, "retry poll failed", delay, func() {
			o.handleRetryFire(issueID)
		})
		return
	}

	// Find our issue
	var found *tracker.Issue
	for i := range issues {
		if issues[i].ID == issueID {
			found = &issues[i]
			break
		}
	}

	if found == nil {
		slog.Info("issue not found in candidates, releasing claim",
			"issue_id", issueID,
			"issue_identifier", retryEntry.Identifier,
		)
		releaseClaim(o.state, issueID)
		return
	}

	if AvailableSlots(o.state, cfg.Agent.MaxConcurrentAgents) <= 0 {
		slog.Info("no available slots, requeuing retry",
			"issue_id", issueID,
			"issue_identifier", retryEntry.Identifier,
		)
		delay := ComputeBackoffDelay(retryEntry.Attempt+1, false, cfg.Agent.MaxRetryBackoffMs)
		ScheduleRetry(o.state, issueID, retryEntry.Identifier, retryEntry.Attempt+1,
			"no available orchestrator slots", delay, func() {
				o.handleRetryFire(issueID)
			})
		return
	}

	attempt := retryEntry.Attempt
	o.dispatchIssue(context.Background(), found, &attempt, cfg)
}

func (o *Orchestrator) applyReload(reload ReloadPayload, ticker *time.Ticker) {
	o.cfgMu.Lock()
	o.cfg = reload.Config
	o.wf = reload.Workflow
	o.cfgMu.Unlock()

	newInterval := time.Duration(reload.Config.Polling.IntervalMs) * time.Millisecond
	ticker.Reset(newInterval)
	o.state.PollIntervalMs = reload.Config.Polling.IntervalMs
	o.state.MaxConcurrentAgents = reload.Config.Agent.MaxConcurrentAgents
	o.state.GeminiModel = reload.Config.Gemini.Model
	o.state.GeminiCommand = reload.Config.Gemini.Command
	o.state.ProjectSlug = reload.Config.Tracker.ProjectSlug

	// Update workspace manager hooks
	o.workspaceMgr.UpdateConfig(&reload.Config.Hooks)

	slog.Info("config reloaded",
		"poll_interval_ms", reload.Config.Polling.IntervalMs,
		"max_concurrent", reload.Config.Agent.MaxConcurrentAgents,
	)
}

func (o *Orchestrator) startupTerminalCleanup() {
	cfg := o.getConfig()
	issues, err := o.tracker.FetchIssuesByStates(cfg.Tracker.ProjectSlug, cfg.Tracker.TerminalStates)
	if err != nil {
		slog.Warn("startup terminal cleanup failed", "error", err)
		return
	}

	for _, issue := range issues {
		slog.Debug("cleaning terminal workspace", "identifier", issue.Identifier)
		o.workspaceMgr.CleanWorkspace(issue.Identifier)
	}

	if len(issues) > 0 {
		slog.Info("startup terminal cleanup completed", "cleaned", len(issues))
	}
}

func (o *Orchestrator) shutdown() {
	slog.Info("orchestrator shutting down")

	// Cancel all running workers
	for id, entry := range o.state.Running {
		slog.Info("cancelling worker", "issue_id", id, "issue_identifier", entry.Identifier)
		if entry.Cancel != nil {
			entry.Cancel()
		}
	}

	// Cancel all retry timers
	for id, entry := range o.state.RetryAttempts {
		if entry.TimerCancel != nil {
			entry.TimerCancel()
		}
		delete(o.state.RetryAttempts, id)
	}
}
