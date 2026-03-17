package orchestrator

import (
	"testing"
	"time"
)

func TestComputeBackoffDelay_Continuation(t *testing.T) {
	delay := ComputeBackoffDelay(1, true, 300000)
	if delay != 1*time.Second {
		t.Errorf("expected 1s continuation delay, got %v", delay)
	}
}

func TestComputeBackoffDelay_ExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 10 * time.Second},       // 10000 * 2^0 = 10s
		{2, 20 * time.Second},       // 10000 * 2^1 = 20s
		{3, 40 * time.Second},       // 10000 * 2^2 = 40s
		{4, 80 * time.Second},       // 10000 * 2^3 = 80s
		{5, 160 * time.Second},      // 10000 * 2^4 = 160s
	}

	for _, tt := range tests {
		delay := ComputeBackoffDelay(tt.attempt, false, 300000)
		if delay != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, delay)
		}
	}
}

func TestComputeBackoffDelay_CappedAtMax(t *testing.T) {
	delay := ComputeBackoffDelay(100, false, 300000) // 2^99 would be huge
	expected := 300 * time.Second
	if delay != expected {
		t.Errorf("expected capped at %v, got %v", expected, delay)
	}
}

func TestScheduleRetry_CreatesEntry(t *testing.T) {
	state := NewState(30000, 10)
	state.Claimed["issue-1"] = struct{}{}

	fired := false
	ScheduleRetry(state, "issue-1", "MT-1", 1, "test error", 1*time.Hour, func() {
		fired = true
	})

	entry, ok := state.RetryAttempts["issue-1"]
	if !ok {
		t.Fatal("expected retry entry created")
	}
	if entry.Attempt != 1 {
		t.Errorf("expected attempt=1, got %d", entry.Attempt)
	}
	if entry.Error != "test error" {
		t.Errorf("expected error='test error', got %q", entry.Error)
	}
	if entry.Identifier != "MT-1" {
		t.Errorf("expected identifier=MT-1, got %q", entry.Identifier)
	}

	// Timer hasn't fired yet (1 hour delay)
	if fired {
		t.Error("timer should not have fired yet")
	}
}

func TestScheduleRetry_CancelsExistingTimer(t *testing.T) {
	state := NewState(30000, 10)

	cancelled := false
	state.RetryAttempts["issue-1"] = &RetryEntry{
		IssueID:     "issue-1",
		TimerCancel: func() { cancelled = true },
	}

	ScheduleRetry(state, "issue-1", "MT-1", 2, "new error", 1*time.Hour, func() {})

	if !cancelled {
		t.Error("expected existing timer to be cancelled")
	}
	if state.RetryAttempts["issue-1"].Attempt != 2 {
		t.Error("expected new entry to replace old one")
	}
}
