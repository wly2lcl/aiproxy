package resilience

import (
	"context"
	"sync"
	"time"
)

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

type CircuitBreakerConfig struct {
	FailureThreshold int
	SuccessThreshold int
	RecoveryTimeout  time.Duration
}

type CircuitBreaker struct {
	mu               sync.RWMutex
	state            State
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int
	recoveryTimeout  time.Duration
	lastFailureTime  time.Time
	timer            *time.Timer
	timerMu          sync.Mutex
}

func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}
	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = 30 * time.Second
	}

	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: config.FailureThreshold,
		successThreshold: config.SuccessThreshold,
		recoveryTimeout:  config.RecoveryTimeout,
	}
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
			cb.transitionToHalfOpen()
		}
	}

	return cb.state
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateClosed {
		return true
	}

	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) >= cb.recoveryTimeout {
			cb.transitionToHalfOpen()
			return true
		}
		return false
	}

	return cb.state == StateHalfOpen
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.transitionToClosed()
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.transitionToOpen()
		}
	case StateHalfOpen:
		cb.transitionToOpen()
	}
}

func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

func (cb *CircuitBreaker) transitionToOpen() {
	cb.state = StateOpen
	cb.failureCount = 0
	cb.successCount = 0
}

func (cb *CircuitBreaker) transitionToClosed() {
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
}

func (cb *CircuitBreaker) transitionToHalfOpen() {
	cb.state = StateHalfOpen
	cb.successCount = 0
}

var ErrCircuitOpen = &CircuitBreakerError{}

type CircuitBreakerError struct{}

func (e *CircuitBreakerError) Error() string {
	return "circuit breaker is open"
}
