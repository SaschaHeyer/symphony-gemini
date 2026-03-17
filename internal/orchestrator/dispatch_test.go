package orchestrator

import (
	"testing"
	"time"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/tracker"
)

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

func testConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Tracker.Kind = "linear"
	cfg.Tracker.APIKey = "test"
	cfg.Tracker.ProjectSlug = "proj"
	return &cfg
}

func TestSortForDispatch_PriorityAscending(t *testing.T) {
	issues := []tracker.Issue{
		{ID: "3", Identifier: "MT-3", Title: "Low", Priority: intPtr(3), State: "Todo"},
		{ID: "1", Identifier: "MT-1", Title: "High", Priority: intPtr(1), State: "Todo"},
		{ID: "2", Identifier: "MT-2", Title: "Med", Priority: intPtr(2), State: "Todo"},
	}

	SortForDispatch(issues)

	if issues[0].Identifier != "MT-1" || issues[1].Identifier != "MT-2" || issues[2].Identifier != "MT-3" {
		t.Errorf("expected priority sort: MT-1, MT-2, MT-3 got %s, %s, %s",
			issues[0].Identifier, issues[1].Identifier, issues[2].Identifier)
	}
}

func TestSortForDispatch_NilPriorityLast(t *testing.T) {
	issues := []tracker.Issue{
		{ID: "1", Identifier: "MT-1", Title: "A", Priority: nil, State: "Todo"},
		{ID: "2", Identifier: "MT-2", Title: "B", Priority: intPtr(2), State: "Todo"},
	}

	SortForDispatch(issues)

	if issues[0].Identifier != "MT-2" {
		t.Errorf("expected non-nil priority first, got %s", issues[0].Identifier)
	}
}

func TestSortForDispatch_SamePriority_OldestFirst(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	issues := []tracker.Issue{
		{ID: "2", Identifier: "MT-2", Title: "B", Priority: intPtr(1), State: "Todo", CreatedAt: &t2},
		{ID: "1", Identifier: "MT-1", Title: "A", Priority: intPtr(1), State: "Todo", CreatedAt: &t1},
	}

	SortForDispatch(issues)

	if issues[0].Identifier != "MT-1" {
		t.Errorf("expected oldest first, got %s", issues[0].Identifier)
	}
}

func TestSortForDispatch_Tiebreaker_Identifier(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	issues := []tracker.Issue{
		{ID: "2", Identifier: "MT-2", Title: "B", Priority: intPtr(1), State: "Todo", CreatedAt: &ts},
		{ID: "1", Identifier: "MT-1", Title: "A", Priority: intPtr(1), State: "Todo", CreatedAt: &ts},
	}

	SortForDispatch(issues)

	if issues[0].Identifier != "MT-1" {
		t.Errorf("expected lexicographic tiebreaker, got %s", issues[0].Identifier)
	}
}

func TestShouldDispatch_SkipAlreadyRunning(t *testing.T) {
	state := NewState(30000, 10)
	state.Running["issue-1"] = &RunningEntry{IssueID: "issue-1"}

	issue := &tracker.Issue{ID: "issue-1", Identifier: "MT-1", Title: "T", State: "Todo"}
	if ShouldDispatch(issue, state, testConfig()) {
		t.Error("should not dispatch already running issue")
	}
}

func TestShouldDispatch_SkipAlreadyClaimed(t *testing.T) {
	state := NewState(30000, 10)
	state.Claimed["issue-1"] = struct{}{}

	issue := &tracker.Issue{ID: "issue-1", Identifier: "MT-1", Title: "T", State: "Todo"}
	if ShouldDispatch(issue, state, testConfig()) {
		t.Error("should not dispatch already claimed issue")
	}
}

func TestShouldDispatch_GlobalConcurrencyLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Agent.MaxConcurrentAgents = 1

	state := NewState(30000, 1)
	state.Running["other"] = &RunningEntry{IssueID: "other"}

	issue := &tracker.Issue{ID: "issue-1", Identifier: "MT-1", Title: "T", State: "Todo"}
	if ShouldDispatch(issue, state, cfg) {
		t.Error("should not dispatch when at global limit")
	}
}

func TestShouldDispatch_PerStateConcurrencyLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Agent.MaxConcurrentAgentsByState = map[string]int{"todo": 1}

	state := NewState(30000, 10)
	state.Running["other"] = &RunningEntry{IssueID: "other", State: "Todo"}

	issue := &tracker.Issue{ID: "issue-1", Identifier: "MT-1", Title: "T", State: "Todo"}
	if ShouldDispatch(issue, state, cfg) {
		t.Error("should not dispatch when at per-state limit")
	}
}

func TestShouldDispatch_TodoWithNonTerminalBlocker(t *testing.T) {
	state := NewState(30000, 10)
	issue := &tracker.Issue{
		ID:         "issue-1",
		Identifier: "MT-1",
		Title:      "T",
		State:      "Todo",
		BlockedBy: []tracker.Blocker{
			{ID: strPtr("b1"), State: strPtr("In Progress")},
		},
	}

	if ShouldDispatch(issue, state, testConfig()) {
		t.Error("should not dispatch Todo with non-terminal blocker")
	}
}

func TestShouldDispatch_TodoWithAllTerminalBlockers(t *testing.T) {
	state := NewState(30000, 10)
	issue := &tracker.Issue{
		ID:         "issue-1",
		Identifier: "MT-1",
		Title:      "T",
		State:      "Todo",
		BlockedBy: []tracker.Blocker{
			{ID: strPtr("b1"), State: strPtr("Done")},
			{ID: strPtr("b2"), State: strPtr("Closed")},
		},
	}

	if !ShouldDispatch(issue, state, testConfig()) {
		t.Error("should dispatch Todo when all blockers are terminal")
	}
}

func TestShouldDispatch_MissingRequiredFields(t *testing.T) {
	state := NewState(30000, 10)
	cfg := testConfig()

	tests := []struct {
		name  string
		issue tracker.Issue
	}{
		{"missing ID", tracker.Issue{Identifier: "MT-1", Title: "T", State: "Todo"}},
		{"missing Identifier", tracker.Issue{ID: "1", Title: "T", State: "Todo"}},
		{"missing Title", tracker.Issue{ID: "1", Identifier: "MT-1", State: "Todo"}},
		{"missing State", tracker.Issue{ID: "1", Identifier: "MT-1", Title: "T"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ShouldDispatch(&tt.issue, state, cfg) {
				t.Errorf("should not dispatch issue with %s", tt.name)
			}
		})
	}
}

func TestAvailableSlots(t *testing.T) {
	state := NewState(30000, 5)
	state.Running["a"] = &RunningEntry{}
	state.Running["b"] = &RunningEntry{}

	if got := AvailableSlots(state, 5); got != 3 {
		t.Errorf("expected 3 slots, got %d", got)
	}
}

func TestAvailableSlots_NoNegative(t *testing.T) {
	state := NewState(30000, 1)
	state.Running["a"] = &RunningEntry{}
	state.Running["b"] = &RunningEntry{}

	if got := AvailableSlots(state, 1); got != 0 {
		t.Errorf("expected 0 slots, got %d", got)
	}
}
