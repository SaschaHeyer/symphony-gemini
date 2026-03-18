package orchestrator

import "github.com/symphony-go/symphony/internal/agent"

// UpdateTokens applies token usage from an agent event to the running entry and totals.
func UpdateTokens(state *State, issueID string, usage *agent.TokenUsage) {
	if usage == nil {
		return
	}

	entry, ok := state.Running[issueID]
	if !ok {
		return
	}

	// Compute deltas from last reported to avoid double-counting
	deltaInput := usage.InputTokens - entry.LastReportedInputTokens
	deltaOutput := usage.OutputTokens - entry.LastReportedOutputTokens
	deltaTotal := usage.TotalTokens - entry.LastReportedTotalTokens

	if deltaInput < 0 {
		deltaInput = 0
	}
	if deltaOutput < 0 {
		deltaOutput = 0
	}
	if deltaTotal < 0 {
		deltaTotal = 0
	}

	// Update entry
	entry.InputTokens += deltaInput
	entry.OutputTokens += deltaOutput
	entry.TotalTokens += deltaTotal
	entry.LastReportedInputTokens = usage.InputTokens
	entry.LastReportedOutputTokens = usage.OutputTokens
	entry.LastReportedTotalTokens = usage.TotalTokens

	// Update totals
	state.AgentTotals.InputTokens += deltaInput
	state.AgentTotals.OutputTokens += deltaOutput
	state.AgentTotals.TotalTokens += deltaTotal
}
