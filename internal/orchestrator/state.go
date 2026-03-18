package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/symphony-go/symphony/internal/tracker"
)

// State is the single authoritative in-memory orchestrator state.
type State struct {
	mu sync.RWMutex

	PollIntervalMs      int
	MaxConcurrentAgents int

	Running       map[string]*RunningEntry
	Claimed       map[string]struct{}
	RetryAttempts map[string]*RetryEntry
	Completed     map[string]struct{}

	AgentTotals TokenTotals
	RateLimits  map[string]any

	// Config info for dashboard
	AgentModel   string
	AgentCommand string
	BackendKind  string
	ProjectSlug  string
}

// RunningEntry tracks a running worker.
type RunningEntry struct {
	IssueID      string
	Identifier   string
	Issue        *tracker.Issue
	Cancel       context.CancelFunc
	SessionID    string
	AgentPID     string
	LastMessage  string
	LastEvent    string
	LastEventAt  *time.Time
	StartedAt    time.Time
	TurnCount    int
	RetryAttempt int
	State        string // tracker state

	InputTokens              int64
	OutputTokens             int64
	TotalTokens              int64
	LastReportedInputTokens  int64
	LastReportedOutputTokens int64
	LastReportedTotalTokens  int64
}

// RetryEntry tracks a scheduled retry for an issue.
type RetryEntry struct {
	IssueID     string
	Identifier  string
	Attempt     int
	DueAt       time.Time
	TimerCancel context.CancelFunc
	Error       string
}

// TokenTotals tracks aggregate token and runtime metrics.
type TokenTotals struct {
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	SecondsRunning float64
}

// NewState creates a fresh orchestrator state.
func NewState(pollIntervalMs, maxConcurrentAgents int) *State {
	return &State{
		PollIntervalMs:      pollIntervalMs,
		MaxConcurrentAgents: maxConcurrentAgents,
		Running:             make(map[string]*RunningEntry),
		Claimed:             make(map[string]struct{}),
		RetryAttempts:       make(map[string]*RetryEntry),
		Completed:           make(map[string]struct{}),
	}
}

// StateSnapshot is a read-consistent copy for external consumers (HTTP API).
type StateSnapshot struct {
	GeneratedAt  time.Time              `json:"generated_at"`
	Config       ConfigSnapshot         `json:"config"`
	Counts       SnapshotCounts         `json:"counts"`
	Running      []RunningSnapshot      `json:"running"`
	Retrying     []RetrySnapshot        `json:"retrying"`
	AgentTotals  TokenTotalsSnapshot    `json:"agent_totals"`
	RateLimits   map[string]any         `json:"rate_limits"`
}

type ConfigSnapshot struct {
	AgentModel   string `json:"agent_model"`
	AgentCommand string `json:"agent_command"`
	BackendKind  string `json:"backend_kind"`
	ProjectSlug  string `json:"project_slug"`
	PollIntervalMs int   `json:"poll_interval_ms"`
	MaxConcurrent  int   `json:"max_concurrent_agents"`
}

type SnapshotCounts struct {
	Running  int `json:"running"`
	Retrying int `json:"retrying"`
}

type RunningSnapshot struct {
	IssueID         string    `json:"issue_id"`
	IssueIdentifier string    `json:"issue_identifier"`
	State           string    `json:"state"`
	SessionID       string    `json:"session_id"`
	TurnCount       int       `json:"turn_count"`
	LastEvent       string    `json:"last_event"`
	LastMessage     string    `json:"last_message"`
	StartedAt       time.Time `json:"started_at"`
	LastEventAt     *time.Time `json:"last_event_at"`
	Tokens          TokensSnapshot `json:"tokens"`
}

type TokensSnapshot struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type RetrySnapshot struct {
	IssueID         string    `json:"issue_id"`
	IssueIdentifier string    `json:"issue_identifier"`
	Attempt         int       `json:"attempt"`
	DueAt           time.Time `json:"due_at"`
	Error           string    `json:"error"`
}

type TokenTotalsSnapshot struct {
	InputTokens    int64   `json:"input_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	TotalTokens    int64   `json:"total_tokens"`
	SecondsRunning float64 `json:"seconds_running"`
}

// Snapshot returns a read-consistent copy of the state.
func (s *State) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UTC()

	running := make([]RunningSnapshot, 0, len(s.Running))
	liveSeconds := 0.0
	for _, entry := range s.Running {
		running = append(running, RunningSnapshot{
			IssueID:         entry.IssueID,
			IssueIdentifier: entry.Identifier,
			State:           entry.State,
			SessionID:       entry.SessionID,
			TurnCount:       entry.TurnCount,
			LastEvent:       entry.LastEvent,
			LastMessage:     entry.LastMessage,
			StartedAt:       entry.StartedAt,
			LastEventAt:     entry.LastEventAt,
			Tokens: TokensSnapshot{
				InputTokens:  entry.InputTokens,
				OutputTokens: entry.OutputTokens,
				TotalTokens:  entry.TotalTokens,
			},
		})
		liveSeconds += now.Sub(entry.StartedAt).Seconds()
	}

	retrying := make([]RetrySnapshot, 0, len(s.RetryAttempts))
	for _, entry := range s.RetryAttempts {
		retrying = append(retrying, RetrySnapshot{
			IssueID:         entry.IssueID,
			IssueIdentifier: entry.Identifier,
			Attempt:         entry.Attempt,
			DueAt:           entry.DueAt,
			Error:           entry.Error,
		})
	}

	return StateSnapshot{
		GeneratedAt: now,
		Config: ConfigSnapshot{
			AgentModel:     s.AgentModel,
			AgentCommand:   s.AgentCommand,
			BackendKind:    s.BackendKind,
			ProjectSlug:    s.ProjectSlug,
			PollIntervalMs: s.PollIntervalMs,
			MaxConcurrent:  s.MaxConcurrentAgents,
		},
		Counts: SnapshotCounts{
			Running:  len(s.Running),
			Retrying: len(s.RetryAttempts),
		},
		Running:  running,
		Retrying: retrying,
		AgentTotals: TokenTotalsSnapshot{
			InputTokens:    s.AgentTotals.InputTokens,
			OutputTokens:   s.AgentTotals.OutputTokens,
			TotalTokens:    s.AgentTotals.TotalTokens,
			SecondsRunning: s.AgentTotals.SecondsRunning + liveSeconds,
		},
		RateLimits: s.RateLimits,
	}
}
