package orchestrator

import (
	"math"
	"time"
)

// ComputeBackoffDelay calculates the retry delay.
// Continuation retries use a fixed 1s delay.
// Failure retries use exponential backoff: min(10000 * 2^(attempt-1), maxBackoffMs).
func ComputeBackoffDelay(attempt int, isContinuation bool, maxBackoffMs int) time.Duration {
	if isContinuation {
		return 1 * time.Second
	}

	if attempt < 1 {
		attempt = 1
	}

	delayMs := 10000.0 * math.Pow(2, float64(attempt-1))
	if delayMs > float64(maxBackoffMs) {
		delayMs = float64(maxBackoffMs)
	}

	return time.Duration(delayMs) * time.Millisecond
}

// ScheduleRetry creates a retry entry and starts a timer.
func ScheduleRetry(
	state *State,
	issueID string,
	identifier string,
	attempt int,
	errMsg string,
	delay time.Duration,
	onFire func(),
) {
	// Cancel existing retry timer for same issue
	if existing, ok := state.RetryAttempts[issueID]; ok {
		if existing.TimerCancel != nil {
			existing.TimerCancel()
		}
	}

	timer := time.AfterFunc(delay, onFire)

	state.RetryAttempts[issueID] = &RetryEntry{
		IssueID:     issueID,
		Identifier:  identifier,
		Attempt:     attempt,
		DueAt:       time.Now().Add(delay),
		TimerCancel: func() { timer.Stop() },
		Error:       errMsg,
	}
}
