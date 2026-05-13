package remote

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type CircuitBreaker struct {
	mu sync.RWMutex
	state State

	failureThreshold int
	successThreshold int
	timeout         time.Duration

	failures    int
	successes   int
	lastFailure time.Time

	openedAt atomic.Int64
}

func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:           StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:         timeout,
	}
}

func (cb *CircuitBreaker) State() State {
	if cb.state == StateOpen {
		if cb.openedAt.Load() > 0 {
			openedTime := time.Unix(cb.openedAt.Load(), 0)
			if time.Since(openedTime) > cb.timeout {
				cb.mu.Lock()
				cb.state = StateHalfOpen
				cb.successes = 0
				cb.mu.Unlock()
				return StateHalfOpen
			}
		}
	}
	return cb.state
}

func (cb *CircuitBreaker) Allow() bool {
	switch cb.State() {
	case StateClosed:
		return true
	case StateHalfOpen:
		return true
	case StateOpen:
		return false
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			cb.openedAt.Store(0)
		}
	case StateClosed:
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.failureThreshold {
			cb.state = StateOpen
			cb.lastFailure = time.Now()
			cb.openedAt.Store(time.Now().Unix())
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.lastFailure = time.Now()
		cb.openedAt.Store(time.Now().Unix())
	}
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.openedAt.Store(0)
}

type CircuitBreakerStats struct {
	State          string  `json:"state"`
	Failures       int     `json:"failures"`
	Successes      int     `json:"successes"`
	LastFailure    string  `json:"last_failure"`
	FailureRate    float64 `json:"failure_rate"`
}

func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var failureRate float64
	total := cb.failures + cb.successes
	if total > 0 {
		failureRate = math.Round(float64(cb.failures)/float64(total)*100) / 100
	}

	lastFailure := ""
	if !cb.lastFailure.IsZero() {
		lastFailure = cb.lastFailure.Format(time.RFC3339)
	}

	return CircuitBreakerStats{
		State:       cb.state.String(),
		Failures:    cb.failures,
		Successes:   cb.successes,
		LastFailure: lastFailure,
		FailureRate: failureRate,
	}
}
