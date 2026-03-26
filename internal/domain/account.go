package domain

import (
	"errors"
	"time"
)

const CircuitBreakerThreshold = 5

type Account struct {
	ID         string
	ProviderID string
	APIKeyHash string
	Weight     int
	Priority   int
	IsEnabled  bool
}

type AccountState struct {
	Account             Account
	CurrentLimits       map[string]int
	LastUsedAt          time.Time
	ConsecutiveFailures int
}

func (a *Account) Validate() error {
	if a.ID == "" {
		return errors.New("account id is required")
	}
	if a.ProviderID == "" {
		return errors.New("provider id is required")
	}
	if a.APIKeyHash == "" {
		return errors.New("api key hash is required")
	}
	if a.Weight < 0 {
		return errors.New("weight must be non-negative")
	}
	if a.Priority < 0 {
		return errors.New("priority must be non-negative")
	}
	return nil
}

func (s *AccountState) IsAvailable() bool {
	if !s.Account.IsEnabled {
		return false
	}
	if s.ConsecutiveFailures >= CircuitBreakerThreshold {
		return false
	}
	return true
}
