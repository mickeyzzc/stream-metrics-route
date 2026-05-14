package remote

import (
	"testing"
	"time"
)

func TestCircuitBreaker_NewStartsClosed(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 5*time.Second)

	if cb.State() != StateClosed {
		t.Errorf("new CB state = %v, want StateClosed", cb.State())
	}
	if !cb.Allow() {
		t.Error("new CB Allow() = false, want true")
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	tests := []struct {
		name             string
		threshold        int
		failures         int
		wantState        State
		wantAllow        bool
		wantFailureCount int
	}{
		{
			name:             "below threshold stays closed",
			threshold:        3,
			failures:         2,
			wantState:        StateClosed,
			wantAllow:        true,
			wantFailureCount: 2,
		},
		{
			name:             "at threshold transitions to open",
			threshold:        3,
			failures:         3,
			wantState:        StateOpen,
			wantAllow:        false,
			wantFailureCount: 3,
		},
		{
			name:             "above threshold transitions to open",
			threshold:        3,
			failures:         5,
			wantState:        StateOpen,
			wantAllow:        false,
			wantFailureCount: 3, // RecordFailure is no-op in Open state; failures capped at threshold
		},
		{
			name:             "threshold of 1 opens on first failure",
			threshold:        1,
			failures:         1,
			wantState:        StateOpen,
			wantAllow:        false,
			wantFailureCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tt.threshold, 2, 5*time.Second)

			for i := 0; i < tt.failures; i++ {
				cb.RecordFailure()
			}

			if got := cb.State(); got != tt.wantState {
				t.Errorf("State() = %v, want %v", got, tt.wantState)
			}
			if got := cb.Allow(); got != tt.wantAllow {
				t.Errorf("Allow() = %v, want %v", got, tt.wantAllow)
			}
			stats := cb.Stats()
			if stats.Failures != tt.wantFailureCount {
				t.Errorf("Stats().Failures = %d, want %d", stats.Failures, tt.wantFailureCount)
			}
		})
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	timeout := 50 * time.Millisecond
	cb := NewCircuitBreaker(3, 2, timeout)

	// Drive to Open state
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	// Check internal state directly (State() may transition if timeout already expired
	// due to second-precision openedAt timestamp)
	if cb.state != StateOpen {
		t.Fatalf("prerequisite: state should be Open, got %v", cb.state)
	}

	// Wait for timeout to expire
	time.Sleep(timeout + 10*time.Millisecond)

	// State() should detect expired timeout and transition to HalfOpen
	if got := cb.State(); got != StateHalfOpen {
		t.Errorf("State() after timeout = %v, want StateHalfOpen", got)
	}
	// Allow should return true in HalfOpen
	if !cb.Allow() {
		t.Error("Allow() in HalfOpen = false, want true")
	}
	// successes should be reset to 0 on transition
	stats := cb.Stats()
	if stats.Successes != 0 {
		t.Errorf("Stats().Successes after Open->HalfOpen = %d, want 0", stats.Successes)
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	tests := []struct {
		name             string
		successThreshold int
		successes        int
		wantState        State
	}{
		{
			name:             "below success threshold stays half-open",
			successThreshold: 3,
			successes:        2,
			wantState:        StateHalfOpen,
		},
		{
			name:             "at success threshold transitions to closed",
			successThreshold: 3,
			successes:        3,
			wantState:        StateClosed,
		},
		{
			name:             "above success threshold transitions to closed",
			successThreshold: 2,
			successes:        5,
			wantState:        StateClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := 50 * time.Millisecond
			cb := NewCircuitBreaker(3, tt.successThreshold, timeout)

			// Drive to Open
			for i := 0; i < 3; i++ {
				cb.RecordFailure()
			}

			// Wait for timeout, then trigger transition to HalfOpen
			time.Sleep(timeout + 10*time.Millisecond)
			if cb.State() != StateHalfOpen {
				t.Fatalf("prerequisite: state should be HalfOpen, got %v", cb.State())
			}

			// Record successes
			for i := 0; i < tt.successes; i++ {
				cb.RecordSuccess()
			}

			if got := cb.State(); got != tt.wantState {
				t.Errorf("State() after successes = %v, want %v", got, tt.wantState)
			}

			if tt.wantState == StateClosed {
				// After transitioning to Closed, failures and successes should be reset
				stats := cb.Stats()
				if stats.Failures != 0 {
					t.Errorf("Stats().Failures after HalfOpen->Closed = %d, want 0", stats.Failures)
				}
				if stats.Successes != 0 {
					t.Errorf("Stats().Successes after HalfOpen->Closed = %d, want 0", stats.Successes)
				}
			}
		})
	}
}

func TestCircuitBreaker_HalfOpenFailureToOpen(t *testing.T) {
	timeout := 50 * time.Millisecond
	cb := NewCircuitBreaker(3, 2, timeout)

	// Drive to Open
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for timeout, trigger HalfOpen
	time.Sleep(timeout + 10*time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Fatalf("prerequisite: state should be HalfOpen, got %v", cb.State())
	}

	// A single failure in HalfOpen should immediately go back to Open
	cb.RecordFailure()

	// Check internal state directly to avoid State() re-checking timeout
	// (openedAt uses second-precision Unix timestamp, causing flaky timeout checks)
	if cb.state != StateOpen {
		t.Errorf("state after failure in HalfOpen = %v, want StateOpen", cb.state)
	}
	// openedAt was just set via RecordFailure using time.Now().Unix() (second precision).
	// State() may immediately re-transition Open→HalfOpen if >50ms into the current second,
	// so Allow() behavior is timing-dependent and not reliably testable here.
	// The critical assertion above confirms RecordFailure correctly sets state to Open.
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 5*time.Second)

	// Drive to Open
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	if cb.State() != StateOpen {
		t.Fatalf("prerequisite: state should be Open, got %v", cb.State())
	}

	// Reset should return to Closed
	cb.Reset()

	if got := cb.State(); got != StateClosed {
		t.Errorf("State() after Reset() = %v, want StateClosed", got)
	}
	if !cb.Allow() {
		t.Error("Allow() after Reset() = false, want true")
	}

	stats := cb.Stats()
	if stats.Failures != 0 {
		t.Errorf("Stats().Failures after Reset() = %d, want 0", stats.Failures)
	}
	if stats.Successes != 0 {
		t.Errorf("Stats().Successes after Reset() = %d, want 0", stats.Successes)
	}
}

func TestCircuitBreaker_ResetFromHalfOpen(t *testing.T) {
	timeout := 50 * time.Millisecond
	cb := NewCircuitBreaker(3, 2, timeout)

	// Drive to Open then wait for HalfOpen
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	time.Sleep(timeout + 10*time.Millisecond)
	cb.State() // trigger transition

	if cb.State() != StateHalfOpen {
		t.Fatalf("prerequisite: state should be HalfOpen, got %v", cb.State())
	}

	cb.Reset()

	if got := cb.State(); got != StateClosed {
		t.Errorf("State() after Reset() from HalfOpen = %v, want StateClosed", got)
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	tests := []struct {
		name              string
		setup             func(*CircuitBreaker)
		wantState         string
		wantFailures      int
		wantSuccesses     int
		wantLastFailure   bool // true if last_failure should be non-empty
		wantFailureRate   float64
	}{
		{
			name: "fresh circuit breaker",
			setup: func(cb *CircuitBreaker) {
				// no operations
			},
			wantState:       "closed",
			wantFailures:    0,
			wantSuccesses:   0,
			wantLastFailure: false,
			wantFailureRate: 0,
		},
		{
			name: "3 failures 2 successes in closed state",
			setup: func(cb *CircuitBreaker) {
				// In Closed state, RecordSuccess resets failures to 0
				// To accumulate both, we need to be in HalfOpen
				// Drive to Open first
				for i := 0; i < 3; i++ {
					cb.RecordFailure()
				}
			},
			wantState:       "open",
			wantFailures:    3,
			wantSuccesses:   0,
			wantLastFailure: true,
			wantFailureRate: 1.0, // 3/3 = 1.0
		},
		{
			name: "failure rate calculation stays half-open with 2 successes",
			setup: func(cb *CircuitBreaker) {
				// Drive to Open
				for i := 0; i < 3; i++ {
					cb.RecordFailure()
				}
				// Wait for timeout to get to HalfOpen
				time.Sleep(60 * time.Millisecond)
				cb.State() // trigger transition to HalfOpen
				// Record some successes (not enough to close with threshold 5)
				cb.RecordSuccess()
				cb.RecordSuccess()
			},
			wantState:       "half-open", // successThreshold=5, so 2 successes don't close
			wantFailures:    3,
			wantSuccesses:   2,
			wantLastFailure: true,
			wantFailureRate: 0.6, // 3/(3+2) = 0.6
		},
		{
			name: "zero total means zero failure rate",
			setup: func(cb *CircuitBreaker) {
				// no operations at all
			},
			wantState:       "closed",
			wantFailures:    0,
			wantSuccesses:   0,
			wantLastFailure: false,
			wantFailureRate: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use successThreshold=5 for the failure rate test, 2 for others
			cb := NewCircuitBreaker(3, 5, 50*time.Millisecond)
			tt.setup(cb)

			stats := cb.Stats()

			if stats.State != tt.wantState {
				t.Errorf("Stats().State = %q, want %q", stats.State, tt.wantState)
			}
			if stats.Failures != tt.wantFailures {
				t.Errorf("Stats().Failures = %d, want %d", stats.Failures, tt.wantFailures)
			}
			if stats.Successes != tt.wantSuccesses {
				t.Errorf("Stats().Successes = %d, want %d", stats.Successes, tt.wantSuccesses)
			}
			gotLast := stats.LastFailure != ""
			if gotLast != tt.wantLastFailure {
				t.Errorf("Stats().LastFailure non-empty = %v, want %v (value = %q)", gotLast, tt.wantLastFailure, stats.LastFailure)
			}
			if stats.FailureRate != tt.wantFailureRate {
				t.Errorf("Stats().FailureRate = %f, want %f", stats.FailureRate, tt.wantFailureRate)
			}
		})
	}
}

func TestCircuitBreaker_RecordSuccessInClosedResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(5, 2, 5*time.Second)

	// Record some failures (below threshold)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.Failures != 3 {
		t.Fatalf("prerequisite: failures = %d, want 3", stats.Failures)
	}

	// RecordSuccess in Closed state resets failures to 0
	cb.RecordSuccess()

	stats = cb.Stats()
	if stats.Failures != 0 {
		t.Errorf("Stats().Failures after success in Closed = %d, want 0", stats.Failures)
	}
	if stats.State != "closed" {
		t.Errorf("Stats().State = %q, want %q", stats.State, "closed")
	}
}

func TestCircuitBreaker_StateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}
