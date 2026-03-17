package orchestrator

import (
	"fmt"
	"testing"
	"time"

	"github.com/symphony-go/symphony/internal/tracker"
)

// mockTracker implements tracker.TrackerClient for tests.
type mockTracker struct {
	candidateIssues []tracker.Issue
	stateIssues     []tracker.Issue
	terminalIssues  []tracker.Issue
	fetchErr        error
	stateErr        error
}

func (m *mockTracker) FetchCandidateIssues(projectSlug string, activeStates []string) ([]tracker.Issue, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return m.candidateIssues, nil
}

func (m *mockTracker) FetchIssueStatesByIDs(ids []string) ([]tracker.Issue, error) {
	if m.stateErr != nil {
		return nil, m.stateErr
	}
	return m.stateIssues, nil
}

func (m *mockTracker) FetchIssuesByStates(projectSlug string, states []string) ([]tracker.Issue, error) {
	return m.terminalIssues, nil
}

func TestReconcile_TerminalState_StopsAndCleansWorkspace(t *testing.T) {
	cfg := testConfig()
	state := NewState(30000, 10)

	cancelled := false
	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		Identifier: "MT-1",
		State:      "In Progress",
		StartedAt:  time.Now(),
		Cancel:     func() { cancelled = true },
	}
	state.Claimed["issue-1"] = struct{}{}

	mock := &mockTracker{
		stateIssues: []tracker.Issue{
			{ID: "issue-1", Identifier: "MT-1", State: "Done"},
		},
	}

	ReconcileRunningIssues(state, mock, nil, cfg, func(string, string, string, int) {})

	if !cancelled {
		t.Error("expected worker to be cancelled")
	}
	if _, ok := state.Running["issue-1"]; ok {
		t.Error("expected issue removed from running")
	}
	if _, ok := state.Claimed["issue-1"]; ok {
		t.Error("expected issue released from claimed")
	}
}

func TestReconcile_ActiveState_UpdatesSnapshot(t *testing.T) {
	cfg := testConfig()
	state := NewState(30000, 10)

	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		Identifier: "MT-1",
		State:      "Todo",
		StartedAt:  time.Now(),
	}

	mock := &mockTracker{
		stateIssues: []tracker.Issue{
			{ID: "issue-1", Identifier: "MT-1", State: "In Progress"},
		},
	}

	ReconcileRunningIssues(state, mock, nil, cfg, func(string, string, string, int) {})

	entry := state.Running["issue-1"]
	if entry == nil {
		t.Fatal("expected issue still in running")
	}
	if entry.State != "In Progress" {
		t.Errorf("expected state=In Progress, got %q", entry.State)
	}
}

func TestReconcile_NeitherActiveNorTerminal_StopsNoCleanup(t *testing.T) {
	cfg := testConfig()
	state := NewState(30000, 10)

	cancelled := false
	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		Identifier: "MT-1",
		State:      "In Progress",
		StartedAt:  time.Now(),
		Cancel:     func() { cancelled = true },
	}
	state.Claimed["issue-1"] = struct{}{}

	// "Review" is neither active nor terminal
	mock := &mockTracker{
		stateIssues: []tracker.Issue{
			{ID: "issue-1", Identifier: "MT-1", State: "Review"},
		},
	}

	ReconcileRunningIssues(state, mock, nil, cfg, func(string, string, string, int) {})

	if !cancelled {
		t.Error("expected worker cancelled")
	}
	if _, ok := state.Running["issue-1"]; ok {
		t.Error("expected removed from running")
	}
}

func TestReconcile_NoRunningIssues_Noop(t *testing.T) {
	cfg := testConfig()
	state := NewState(30000, 10)

	mock := &mockTracker{
		stateErr: fmt.Errorf("should not be called"),
	}

	// Should not call tracker since no running issues
	ReconcileRunningIssues(state, mock, nil, cfg, func(string, string, string, int) {})
}

func TestReconcile_StateRefreshFailure_KeepsWorkers(t *testing.T) {
	cfg := testConfig()
	state := NewState(30000, 10)

	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		Identifier: "MT-1",
		State:      "Todo",
		StartedAt:  time.Now(),
	}

	mock := &mockTracker{
		stateErr: fmt.Errorf("network error"),
	}

	ReconcileRunningIssues(state, mock, nil, cfg, func(string, string, string, int) {})

	if _, ok := state.Running["issue-1"]; !ok {
		t.Error("expected issue still in running after state refresh failure")
	}
}

func TestReconcile_StallDetected(t *testing.T) {
	cfg := testConfig()
	cfg.Gemini.StallTimeoutMs = 100 // 100ms for test speed

	state := NewState(30000, 10)

	cancelled := false
	pastTime := time.Now().Add(-200 * time.Millisecond) // stalled
	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		Identifier: "MT-1",
		State:      "Todo",
		StartedAt:  pastTime,
		Cancel:     func() { cancelled = true },
	}

	// Use empty mock — reconcile_stalls runs before tracker state refresh
	mock := &mockTracker{}

	retried := false
	ReconcileRunningIssues(state, mock, nil, cfg, func(issueID, identifier, errMsg string, attempt int) {
		retried = true
		if errMsg != "stall detected" {
			t.Errorf("expected 'stall detected', got %q", errMsg)
		}
	})

	if !cancelled {
		t.Error("expected stalled worker cancelled")
	}
	if !retried {
		t.Error("expected retry scheduled for stalled issue")
	}
}

func TestReconcile_StallDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Gemini.StallTimeoutMs = 0 // disabled

	state := NewState(30000, 10)

	pastTime := time.Now().Add(-1 * time.Hour) // very old
	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		Identifier: "MT-1",
		State:      "Todo",
		StartedAt:  pastTime,
	}

	mock := &mockTracker{
		stateIssues: []tracker.Issue{
			{ID: "issue-1", Identifier: "MT-1", State: "Todo"},
		},
	}

	ReconcileRunningIssues(state, mock, nil, cfg, func(string, string, string, int) {
		t.Error("should not schedule retry when stall detection disabled")
	})

	if _, ok := state.Running["issue-1"]; !ok {
		t.Error("expected issue still running (stall disabled)")
	}
}
