package domain

import (
	"errors"
	"time"
)

type RetryConfig struct {
	MaxRetries  int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

type CircuitBreakerConfig struct {
	Threshold        int
	Timeout          time.Duration
	HalfOpenRequests int
	OnStateChange    func(from, to string)
}

type ProviderConfig struct {
	RetryConfig          RetryConfig
	CircuitBreakerConfig CircuitBreakerConfig
}

type Provider struct {
	ID        string
	APIBase   string
	Models    []string
	IsDefault bool
	IsEnabled bool
	Headers   map[string]string
}

func (p *Provider) Validate() error {
	if p.ID == "" {
		return errors.New("provider id is required")
	}
	if p.APIBase == "" {
		return errors.New("api base is required")
	}
	return nil
}
