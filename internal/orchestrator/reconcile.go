package orchestrator

import (
	"log/slog"
	"strings"
	"time"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/tracker"
	"github.com/symphony-go/symphony/internal/workspace"
)

// ReconcileRunningIssues performs stall detection and tracker state refresh.
func ReconcileRunningIssues(
	state *State,
	trackerClient tracker.TrackerClient,
	workspaceMgr *workspace.Manager,
	cfg *config.Config,
	retryFn func(issueID, identifier, errMsg string, attempt int),
) {
	reconcileStalls(state, cfg, retryFn)
	reconcileTrackerStates(state, trackerClient, workspaceMgr, cfg)
}

func reconcileStalls(state *State, cfg *config.Config, retryFn func(string, string, string, int)) {
	stallTimeoutMs := cfg.Gemini.StallTimeoutMs
	if cfg.Backend == "claude" {
		stallTimeoutMs = cfg.Claude.StallTimeoutMs
	}
	if stallTimeoutMs <= 0 {
		return
	}

	stallTimeout := time.Duration(stallTimeoutMs) * time.Millisecond
	now := time.Now()

	var stalled []string
	for id, entry := range state.Running {
		refTime := entry.StartedAt
		if entry.LastEventAt != nil {
			refTime = *entry.LastEventAt
		}
		if now.Sub(refTime) > stallTimeout {
			stalled = append(stalled, id)
		}
	}

	for _, id := range stalled {
		entry := state.Running[id]
		slog.Warn("stall detected, killing worker",
			"issue_id", id,
			"issue_identifier", entry.Identifier,
			"elapsed_since_last_event", time.Since(entry.StartedAt),
		)

		if entry.Cancel != nil {
			entry.Cancel()
		}
		attempt := entry.RetryAttempt + 1
		identifier := entry.Identifier

		removeRunning(state, id)
		retryFn(id, identifier, "stall detected", attempt)
	}
}

func reconcileTrackerStates(
	state *State,
	trackerClient tracker.TrackerClient,
	workspaceMgr *workspace.Manager,
	cfg *config.Config,
) {
	ids := make([]string, 0, len(state.Running))
	for id := range state.Running {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return
	}

	refreshed, err := trackerClient.FetchIssueStatesByIDs(ids)
	if err != nil {
		slog.Debug("state refresh failed, keeping workers running", "error", err)
		return
	}

	// Build lookup
	stateByID := make(map[string]*tracker.Issue)
	for i := range refreshed {
		stateByID[refreshed[i].ID] = &refreshed[i]
	}

	for id, entry := range state.Running {
		refreshedIssue, found := stateByID[id]
		if !found {
			continue
		}

		lowerState := strings.ToLower(refreshedIssue.State)

		if containsLower(cfg.Tracker.TerminalStates, lowerState) {
			// Terminal → stop worker + clean workspace
			slog.Info("issue entered terminal state, stopping worker",
				"issue_id", id,
				"issue_identifier", entry.Identifier,
				"state", refreshedIssue.State,
			)
			if entry.Cancel != nil {
				entry.Cancel()
			}
			removeRunning(state, id)
			releaseClaim(state, id)
			if workspaceMgr != nil {
				workspaceMgr.CleanWorkspace(entry.Identifier)
			}

		} else if containsLower(cfg.Tracker.ActiveStates, lowerState) {
			// Still active → update snapshot
			entry.State = refreshedIssue.State
			if refreshedIssue.Identifier != "" {
				entry.Issue = refreshedIssue
			}

		} else {
			// Neither active nor terminal → stop worker, no cleanup
			slog.Info("issue in non-active/non-terminal state, stopping worker",
				"issue_id", id,
				"issue_identifier", entry.Identifier,
				"state", refreshedIssue.State,
			)
			if entry.Cancel != nil {
				entry.Cancel()
			}
			removeRunning(state, id)
			releaseClaim(state, id)
		}
	}
}

func removeRunning(state *State, issueID string) {
	entry, ok := state.Running[issueID]
	if !ok {
		return
	}

	// Add runtime seconds
	elapsed := time.Since(entry.StartedAt).Seconds()
	state.AgentTotals.SecondsRunning += elapsed

	delete(state.Running, issueID)
}

func releaseClaim(state *State, issueID string) {
	delete(state.Claimed, issueID)
}
