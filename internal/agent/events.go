package agent

import "time"

// Event type constants for orchestrator communication.
const (
	EventSessionStarted = "session_started"
	EventTurnCompleted  = "turn_completed"
	EventTurnFailed     = "turn_failed"
	EventTurnCancelled  = "turn_cancelled"
	EventNotification   = "notification"
	EventToolCall       = "tool_call"
	EventMalformed      = "malformed"
)

// AgentEvent is a structured event emitted from the agent runner to the orchestrator.
type AgentEvent struct {
	Type      string
	Timestamp time.Time
	SessionID string
	Message   string
	Usage     *TokenUsage
}

// TokenUsage holds token count data extracted from agent events.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}
