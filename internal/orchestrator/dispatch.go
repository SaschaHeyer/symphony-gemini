package orchestrator

import (
	"slices"
	"strings"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/tracker"
)

// SortForDispatch sorts issues by: priority ascending (nil last), created_at oldest, identifier lexicographic.
func SortForDispatch(issues []tracker.Issue) {
	slices.SortStableFunc(issues, func(a, b tracker.Issue) int {
		// Priority: ascending, nil last
		ap := priorityVal(a.Priority)
		bp := priorityVal(b.Priority)
		if ap != bp {
			return ap - bp
		}

		// Created_at: oldest first
		if a.CreatedAt != nil && b.CreatedAt != nil {
			if a.CreatedAt.Before(*b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.After(*b.CreatedAt) {
				return 1
			}
		} else if a.CreatedAt != nil {
			return -1
		} else if b.CreatedAt != nil {
			return 1
		}

		// Identifier: lexicographic
		return strings.Compare(a.Identifier, b.Identifier)
	})
}

func priorityVal(p *int) int {
	if p == nil {
		return 999999 // nil sorts last
	}
	return *p
}

// ShouldDispatch checks whether an issue is eligible for dispatch.
func ShouldDispatch(issue *tracker.Issue, state *State, cfg *config.Config) bool {
	// Required fields
	if issue.ID == "" || issue.Identifier == "" || issue.Title == "" || issue.State == "" {
		return false
	}

	lowerState := strings.ToLower(issue.State)

	// Must be in active states
	if !containsLower(cfg.Tracker.ActiveStates, lowerState) {
		return false
	}

	// Must NOT be in terminal states
	if containsLower(cfg.Tracker.TerminalStates, lowerState) {
		return false
	}

	// Not already running
	if _, running := state.Running[issue.ID]; running {
		return false
	}

	// Not already claimed
	if _, claimed := state.Claimed[issue.ID]; claimed {
		return false
	}

	// Global concurrency
	if len(state.Running) >= cfg.Agent.MaxConcurrentAgents {
		return false
	}

	// Per-state concurrency
	if limit, ok := cfg.Agent.MaxConcurrentAgentsByState[lowerState]; ok {
		count := countRunningByState(state, lowerState)
		if count >= limit {
			return false
		}
	}

	// Blocker rule: Todo with non-terminal blockers is NOT eligible
	if lowerState == "todo" && hasNonTerminalBlockers(issue, cfg) {
		return false
	}

	return true
}

// AvailableSlots returns the number of available global dispatch slots.
func AvailableSlots(state *State, maxConcurrent int) int {
	available := maxConcurrent - len(state.Running)
	if available < 0 {
		return 0
	}
	return available
}

func containsLower(list []string, lowerTarget string) bool {
	for _, s := range list {
		if strings.ToLower(s) == lowerTarget {
			return true
		}
	}
	return false
}

func countRunningByState(state *State, lowerState string) int {
	count := 0
	for _, entry := range state.Running {
		if strings.ToLower(entry.State) == lowerState {
			count++
		}
	}
	return count
}

func hasNonTerminalBlockers(issue *tracker.Issue, cfg *config.Config) bool {
	for _, blocker := range issue.BlockedBy {
		if blocker.State == nil {
			return true // unknown state is non-terminal
		}
		if !containsLower(cfg.Tracker.TerminalStates, strings.ToLower(*blocker.State)) {
			return true
		}
	}
	return false
}
