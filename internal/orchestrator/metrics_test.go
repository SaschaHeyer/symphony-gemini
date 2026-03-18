package orchestrator

import (
	"testing"
	"time"

	"github.com/symphony-go/symphony/internal/agent"
)

func TestUpdateTokens_DeltaTracking(t *testing.T) {
	state := NewState(30000, 10)
	state.Running["issue-1"] = &RunningEntry{
		IssueID:    "issue-1",
		StartedAt:  time.Now(),
	}

	// First update: absolute totals
	UpdateTokens(state, "issue-1", &agent.TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
	})

	entry := state.Running["issue-1"]
	if entry.InputTokens != 100 {
		t.Errorf("expected input=100, got %d", entry.InputTokens)
	}
	if state.AgentTotals.InputTokens != 100 {
		t.Errorf("expected total input=100, got %d", state.AgentTotals.InputTokens)
	}

	// Second update: delta computed from last reported
	UpdateTokens(state, "issue-1", &agent.TokenUsage{
		InputTokens:  250,
		OutputTokens: 120,
		TotalTokens:  370,
	})

	if entry.InputTokens != 250 {
		t.Errorf("expected input=250, got %d", entry.InputTokens)
	}
	// Total should be 100 (first) + 150 (delta) = 250
	if state.AgentTotals.InputTokens != 250 {
		t.Errorf("expected total input=250, got %d", state.AgentTotals.InputTokens)
	}
}

func TestUpdateTokens_NoDoubleCount(t *testing.T) {
	state := NewState(30000, 10)
	state.Running["issue-1"] = &RunningEntry{
		IssueID:   "issue-1",
		StartedAt: time.Now(),
	}

	// Same values reported twice — delta should be zero
	usage := &agent.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}
	UpdateTokens(state, "issue-1", usage)
	UpdateTokens(state, "issue-1", usage)

	if state.AgentTotals.InputTokens != 100 {
		t.Errorf("expected no double-count, got total input=%d", state.AgentTotals.InputTokens)
	}
}

func TestUpdateTokens_NilUsage(t *testing.T) {
	state := NewState(30000, 10)
	state.Running["issue-1"] = &RunningEntry{IssueID: "issue-1", StartedAt: time.Now()}

	// Should not panic
	UpdateTokens(state, "issue-1", nil)

	if state.AgentTotals.InputTokens != 0 {
		t.Error("expected zero tokens for nil usage")
	}
}

func TestSnapshot_IncludesLiveElapsedTime(t *testing.T) {
	state := NewState(30000, 10)
	state.Running["issue-1"] = &RunningEntry{
		IssueID:   "issue-1",
		StartedAt: time.Now().Add(-10 * time.Second), // 10 seconds ago
	}
	state.AgentTotals.SecondsRunning = 5.0 // 5s from ended sessions

	snapshot := state.Snapshot()

	// Total should be ~15s (5 ended + ~10 live)
	if snapshot.AgentTotals.SecondsRunning < 14.0 {
		t.Errorf("expected ~15s running, got %.1f", snapshot.AgentTotals.SecondsRunning)
	}
}
