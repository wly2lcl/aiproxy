package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

var testError = errors.New("test error")

func TestRetry_Success(t *testing.T) {
	retry := NewRetry(&RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		Multiplier:    2.0,
		RetryOnStatus: []int{429, 500, 502, 503, 504},
	})

	attempts := 0
	err := retry.Do(context.Background(), func() error {
		attempts++
		if attempts < 2 {
			return NewStatusError(429, "rate limited", nil)
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetry_MaxAttempts(t *testing.T) {
	retry := NewRetry(&RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		Multiplier:    2.0,
		RetryOnStatus: []int{500},
	})

	attempts := 0
	err := retry.Do(context.Background(), func() error {
		attempts++
		return NewStatusError(500, "server error", nil)
	})

	if err == nil {
		t.Error("expected error after max attempts")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	retry := NewRetry(&RetryConfig{
		MaxAttempts:   4,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		Multiplier:    2.0,
		RetryOnStatus: []int{500},
	})

	attempts := 0
	start := time.Now()
	err := retry.Do(context.Background(), func() error {
		attempts++
		if attempts < 4 {
			return NewStatusError(500, "server error", nil)
		}
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	expectedMin := 100*time.Millisecond + 200*time.Millisecond + 400*time.Millisecond
	if elapsed < expectedMin-50*time.Millisecond {
		t.Errorf("expected at least %v elapsed, got %v", expectedMin, elapsed)
	}
}

func TestRetry_StatusCodes(t *testing.T) {
	retry := NewRetry(&RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		Multiplier:    2.0,
		RetryOnStatus: []int{429, 500, 502, 503, 504},
	})

	attempts := 0
	err := retry.Do(context.Background(), func() error {
		attempts++
		if attempts == 1 {
			return NewStatusError(503, "service unavailable", nil)
		}
		if attempts == 2 {
			return NewStatusError(504, "gateway timeout", nil)
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	nonRetryableErr := NewStatusError(401, "unauthorized", nil)
	attempts = 0
	err = retry.Do(context.Background(), func() error {
		attempts++
		return nonRetryableErr
	})

	if err == nil {
		t.Error("expected error for non-retryable status")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt for non-retryable, got %d", attempts)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	retry := NewRetry(&RetryConfig{
		MaxAttempts:   10,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		Multiplier:    2.0,
		RetryOnStatus: []int{500},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	attempts := 0
	err := retry.Do(ctx, func() error {
		attempts++
		return NewStatusError(500, "server error", nil)
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context deadline exceeded, got %v", err)
	}
	if attempts > 2 {
		t.Errorf("expected at most 2 attempts with cancellation, got %d", attempts)
	}
}

func TestRetry_IsRetryableError(t *testing.T) {
	retry := NewRetry(&RetryConfig{
		RetryOnStatus:   []int{429, 500, 502, 503, 504},
		RetryableErrors: []error{testError},
	})

	if !retry.IsRetryableError(NewStatusError(429, "rate limited", nil), 0) {
		t.Error("expected 429 to be retryable")
	}

	if !retry.IsRetryableError(testError, 0) {
		t.Error("expected testError to be retryable")
	}

	if retry.IsRetryableError(errors.New("other error"), 0) {
		t.Error("expected other error to not be retryable")
	}

	if !retry.IsRetryableError(NewStatusError(500, "internal error", nil), 500) {
		t.Error("expected 500 status to be retryable")
	}
}

func TestCircuitBreaker_Closed(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		RecoveryTimeout:  100 * time.Millisecond,
	})

	if cb.State() != StateClosed {
		t.Errorf("expected initial state to be closed, got %v", cb.State())
	}

	for i := 0; i < 5; i++ {
		if !cb.Allow() {
			t.Error("expected Allow() to return true when closed")
		}
		cb.RecordSuccess()
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain closed after successes, got %v", cb.State())
	}
}

func TestCircuitBreaker_Open(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		RecoveryTimeout:  100 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be open after failures, got %v", cb.State())
	}

	if cb.Allow() {
		t.Error("expected Allow() to return false when open")
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		RecoveryTimeout:  50 * time.Millisecond,
	})

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("expected state to be open, got %v", cb.State())
	}

	time.Sleep(60 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state to be half-open after recovery timeout, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("expected Allow() to return true when half-open")
	}
}

func TestCircuitBreaker_Recovery(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		RecoveryTimeout:  50 * time.Millisecond,
	})

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Error("expected Allow() to return true when half-open")
	}

	cb.RecordSuccess()
	if cb.State() != StateHalfOpen {
		t.Errorf("expected state to remain half-open after 1 success, got %v", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("expected state to be closed after success threshold, got %v", cb.State())
	}
}

func TestCircuitBreaker_ThreadSafe(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 10,
		SuccessThreshold: 5,
		RecoveryTimeout:  100 * time.Millisecond,
	})

	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				cb.RecordSuccess()
			} else {
				cb.RecordFailure()
			}
			_ = cb.State()
			_ = cb.Allow()
		}(i)
	}

	wg.Wait()

	state := cb.State()
	if state != StateClosed && state != StateOpen && state != StateHalfOpen {
		t.Errorf("unexpected state: %v", state)
	}
}

func TestCircuitBreaker_Call_WrapsFunction(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		RecoveryTimeout:  100 * time.Millisecond,
	})

	calls := 0
	err := cb.Call(context.Background(), func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}

	calls = 0
	testErr := errors.New("test error")
	err = cb.Call(context.Background(), func() error {
		calls++
		return testErr
	})

	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	calls = 0
	err = cb.Call(context.Background(), func() error {
		calls++
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if calls != 0 {
		t.Errorf("expected 0 calls when open, got %d", calls)
	}
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		RecoveryTimeout:  50 * time.Millisecond,
	})

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected half-open state, got %v", cb.State())
	}

	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("expected state to be open after failure in half-open, got %v", cb.State())
	}
}

func TestRetry_DefaultConfig(t *testing.T) {
	retry := NewRetry(&RetryConfig{})

	if retry.config.MaxAttempts != 3 {
		t.Errorf("expected default MaxAttempts to be 3, got %d", retry.config.MaxAttempts)
	}
	if retry.config.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected default InitialDelay to be 100ms, got %v", retry.config.InitialDelay)
	}
	if retry.config.MaxDelay != 30*time.Second {
		t.Errorf("expected default MaxDelay to be 30s, got %v", retry.config.MaxDelay)
	}
	if retry.config.Multiplier != 2.0 {
		t.Errorf("expected default Multiplier to be 2.0, got %f", retry.config.Multiplier)
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{})

	if cb.failureThreshold != 5 {
		t.Errorf("expected default FailureThreshold to be 5, got %d", cb.failureThreshold)
	}
	if cb.successThreshold != 3 {
		t.Errorf("expected default SuccessThreshold to be 3, got %d", cb.successThreshold)
	}
	if cb.recoveryTimeout != 30*time.Second {
		t.Errorf("expected default RecoveryTimeout to be 30s, got %v", cb.recoveryTimeout)
	}
}

func TestCircuitBreakerError(t *testing.T) {
	err := ErrCircuitOpen
	if err.Error() != "circuit breaker is open" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, got)
		}
	}
}
