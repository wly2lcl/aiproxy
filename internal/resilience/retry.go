package resilience

import (
	"context"
	"errors"
	"math"
	"time"
)

type RetryConfig struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	Multiplier      float64
	RetryOnStatus   []int
	RetryableErrors []error
}

type Retry struct {
	config *RetryConfig
}

func NewRetry(config *RetryConfig) *Retry {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	return &Retry{config: config}
}

func (r *Retry) Do(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := r.calculateDelay(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !r.IsRetryableError(err, 0) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return lastErr
}

func (r *Retry) calculateDelay(attempt int) time.Duration {
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}
	return time.Duration(delay)
}

func (r *Retry) IsRetryableError(err error, status int) bool {
	if err == nil {
		return false
	}

	if status > 0 {
		for _, retryableStatus := range r.config.RetryOnStatus {
			if status == retryableStatus {
				return true
			}
		}
	}

	for _, retryableErr := range r.config.RetryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		for _, retryableStatus := range r.config.RetryOnStatus {
			if statusErr.StatusCode == retryableStatus {
				return true
			}
		}
	}

	return false
}

type StatusError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *StatusError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *StatusError) Unwrap() error {
	return e.Err
}

func NewStatusError(statusCode int, message string, err error) *StatusError {
	return &StatusError{
		StatusCode: statusCode,
		Message:    message,
		Err:        err,
	}
}
