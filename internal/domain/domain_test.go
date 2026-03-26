package domain

import (
	"testing"
	"time"
)

func TestAccount_Validate(t *testing.T) {
	tests := []struct {
		name    string
		account Account
		wantErr bool
	}{
		{
			name: "valid account",
			account: Account{
				ID:         "acc-123",
				ProviderID: "provider-1",
				APIKeyHash: "hash123",
				Weight:     10,
				Priority:   1,
				IsEnabled:  true,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			account: Account{
				ProviderID: "provider-1",
				APIKeyHash: "hash123",
			},
			wantErr: true,
		},
		{
			name: "missing provider id",
			account: Account{
				ID:         "acc-123",
				APIKeyHash: "hash123",
			},
			wantErr: true,
		},
		{
			name: "missing api key hash",
			account: Account{
				ID:         "acc-123",
				ProviderID: "provider-1",
			},
			wantErr: true,
		},
		{
			name: "negative weight",
			account: Account{
				ID:         "acc-123",
				ProviderID: "provider-1",
				APIKeyHash: "hash123",
				Weight:     -1,
			},
			wantErr: true,
		},
		{
			name: "negative priority",
			account: Account{
				ID:         "acc-123",
				ProviderID: "provider-1",
				APIKeyHash: "hash123",
				Priority:   -1,
			},
			wantErr: true,
		},
		{
			name: "zero weight is valid",
			account: Account{
				ID:         "acc-123",
				ProviderID: "provider-1",
				APIKeyHash: "hash123",
				Weight:     0,
				Priority:   0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.account.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Account.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAccountState_IsAvailable(t *testing.T) {
	tests := []struct {
		name  string
		state AccountState
		want  bool
	}{
		{
			name: "available account",
			state: AccountState{
				Account: Account{
					IsEnabled: true,
				},
				ConsecutiveFailures: 0,
			},
			want: true,
		},
		{
			name: "disabled account",
			state: AccountState{
				Account: Account{
					IsEnabled: false,
				},
				ConsecutiveFailures: 0,
			},
			want: false,
		},
		{
			name: "in circuit breaker",
			state: AccountState{
				Account: Account{
					IsEnabled: true,
				},
				ConsecutiveFailures: CircuitBreakerThreshold,
			},
			want: false,
		},
		{
			name: "below circuit breaker threshold",
			state: AccountState{
				Account: Account{
					IsEnabled: true,
				},
				ConsecutiveFailures: CircuitBreakerThreshold - 1,
			},
			want: true,
		},
		{
			name: "above circuit breaker threshold",
			state: AccountState{
				Account: Account{
					IsEnabled: true,
				},
				ConsecutiveFailures: CircuitBreakerThreshold + 1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsAvailable(); got != tt.want {
				t.Errorf("AccountState.IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_Validate(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		wantErr  bool
	}{
		{
			name: "valid provider",
			provider: Provider{
				ID:        "provider-1",
				APIBase:   "https://api.example.com",
				Models:    []string{"gpt-4", "gpt-3.5-turbo"},
				IsDefault: true,
				IsEnabled: true,
				Headers:   map[string]string{"Authorization": "Bearer token"},
			},
			wantErr: false,
		},
		{
			name: "missing id",
			provider: Provider{
				APIBase: "https://api.example.com",
			},
			wantErr: true,
		},
		{
			name: "missing api base",
			provider: Provider{
				ID: "provider-1",
			},
			wantErr: true,
		},
		{
			name: "empty models is valid",
			provider: Provider{
				ID:      "provider-1",
				APIBase: "https://api.example.com",
				Models:  []string{},
			},
			wantErr: false,
		},
		{
			name: "nil headers is valid",
			provider: Provider{
				ID:      "provider-1",
				APIBase: "https://api.example.com",
				Headers: nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Provider.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLimitState_IsExceeded(t *testing.T) {
	tests := []struct {
		name  string
		state LimitState
		want  bool
	}{
		{
			name: "not exceeded",
			state: LimitState{
				Current: 50,
				Max:     100,
			},
			want: false,
		},
		{
			name: "exceeded",
			state: LimitState{
				Current: 100,
				Max:     100,
			},
			want: true,
		},
		{
			name: "over limit",
			state: LimitState{
				Current: 150,
				Max:     100,
			},
			want: true,
		},
		{
			name: "zero current",
			state: LimitState{
				Current: 0,
				Max:     100,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsExceeded(); got != tt.want {
				t.Errorf("LimitState.IsExceeded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLimitState_Remaining(t *testing.T) {
	tests := []struct {
		name  string
		state LimitState
		want  int
	}{
		{
			name: "normal remaining",
			state: LimitState{
				Current: 30,
				Max:     100,
			},
			want: 70,
		},
		{
			name: "exactly at limit",
			state: LimitState{
				Current: 100,
				Max:     100,
			},
			want: 0,
		},
		{
			name: "over limit returns zero",
			state: LimitState{
				Current: 150,
				Max:     100,
			},
			want: 0,
		},
		{
			name: "zero current",
			state: LimitState{
				Current: 0,
				Max:     100,
			},
			want: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Remaining(); got != tt.want {
				t.Errorf("LimitState.Remaining() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDomainError_Error(t *testing.T) {
	err := &DomainError{
		Code:    ErrCodeAccountNotFound,
		Message: "account not found",
		Details: map[string]interface{}{"id": "acc-123"},
	}

	if err.Error() != "account not found" {
		t.Errorf("DomainError.Error() = %v, want %v", err.Error(), "account not found")
	}
}

func TestNewDomainError(t *testing.T) {
	err := NewDomainError(ErrCodeInvalidConfig, "invalid configuration")

	if err.Code != ErrCodeInvalidConfig {
		t.Errorf("NewDomainError().Code = %v, want %v", err.Code, ErrCodeInvalidConfig)
	}
	if err.Message != "invalid configuration" {
		t.Errorf("NewDomainError().Message = %v, want %v", err.Message, "invalid configuration")
	}
	if err.Details == nil {
		t.Error("NewDomainError().Details should not be nil")
	}
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError(string(LimitTypeRPM), 60, 100)

	if err.Code != ErrCodeRateLimitExceeded {
		t.Errorf("NewRateLimitError().Code = %v, want %v", err.Code, ErrCodeRateLimitExceeded)
	}
	if err.Message != "rate limit exceeded" {
		t.Errorf("NewRateLimitError().Message = %v, want %v", err.Message, "rate limit exceeded")
	}
	if err.Details["limit_type"] != string(LimitTypeRPM) {
		t.Errorf("NewRateLimitError().Details[limit_type] = %v, want %v", err.Details["limit_type"], string(LimitTypeRPM))
	}
	if err.Details["current"] != 60 {
		t.Errorf("NewRateLimitError().Details[current] = %v, want %v", err.Details["current"], 60)
	}
	if err.Details["max"] != 100 {
		t.Errorf("NewRateLimitError().Details[max] = %v, want %v", err.Details["max"], 100)
	}
}

func TestLimitTypeConstants(t *testing.T) {
	tests := []struct {
		name      string
		limitType LimitType
		expected  string
	}{
		{"rpm", LimitTypeRPM, "rpm"},
		{"daily", LimitTypeDaily, "daily"},
		{"window_5h", LimitTypeWindow5h, "window_5h"},
		{"monthly", LimitTypeMonthly, "monthly"},
		{"token_daily", LimitTypeTokenDaily, "token_daily"},
		{"token_monthly", LimitTypeTokenMonthly, "token_monthly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.limitType) != tt.expected {
				t.Errorf("LimitType = %v, want %v", string(tt.limitType), tt.expected)
			}
		})
	}
}

func TestAccountLimits(t *testing.T) {
	rpm := 100
	daily := 1000
	monthly := 30000

	limits := AccountLimits{
		RPM:     &rpm,
		Daily:   &daily,
		Monthly: &monthly,
	}

	if *limits.RPM != 100 {
		t.Errorf("AccountLimits.RPM = %v, want %v", *limits.RPM, 100)
	}
	if *limits.Daily != 1000 {
		t.Errorf("AccountLimits.Daily = %v, want %v", *limits.Daily, 1000)
	}
	if *limits.Monthly != 30000 {
		t.Errorf("AccountLimits.Monthly = %v, want %v", *limits.Monthly, 30000)
	}
	if limits.TokenDaily != nil {
		t.Errorf("AccountLimits.TokenDaily should be nil")
	}
}

func TestProviderConfig(t *testing.T) {
	config := ProviderConfig{
		RetryConfig: RetryConfig{
			MaxRetries:  3,
			InitialWait: 100 * time.Millisecond,
			MaxWait:     5 * time.Second,
			Multiplier:  2.0,
		},
		CircuitBreakerConfig: CircuitBreakerConfig{
			Threshold:        5,
			Timeout:          30 * time.Second,
			HalfOpenRequests: 3,
		},
	}

	if config.RetryConfig.MaxRetries != 3 {
		t.Errorf("RetryConfig.MaxRetries = %v, want %v", config.RetryConfig.MaxRetries, 3)
	}
	if config.CircuitBreakerConfig.Threshold != 5 {
		t.Errorf("CircuitBreakerConfig.Threshold = %v, want %v", config.CircuitBreakerConfig.Threshold, 5)
	}
}

func TestLimitStateWithTime(t *testing.T) {
	now := time.Now()
	state := LimitState{
		Type:        LimitTypeRPM,
		Current:     50,
		Max:         60,
		WindowStart: now,
		WindowEnd:   now.Add(time.Minute),
	}

	if state.Type != LimitTypeRPM {
		t.Errorf("LimitState.Type = %v, want %v", state.Type, LimitTypeRPM)
	}
	if !state.WindowStart.Equal(now) {
		t.Errorf("LimitState.WindowStart should be %v", now)
	}
	if !state.WindowEnd.Equal(now.Add(time.Minute)) {
		t.Errorf("LimitState.WindowEnd should be %v", now.Add(time.Minute))
	}
}

func TestAccountStateWithLimits(t *testing.T) {
	account := Account{
		ID:         "acc-123",
		ProviderID: "provider-1",
		APIKeyHash: "hash123",
		Weight:     10,
		Priority:   1,
		IsEnabled:  true,
	}

	state := AccountState{
		Account: account,
		CurrentLimits: map[string]int{
			"rpm":   50,
			"daily": 100,
		},
		LastUsedAt:          time.Now(),
		ConsecutiveFailures: 0,
	}

	if state.Account.ID != "acc-123" {
		t.Errorf("AccountState.Account.ID = %v, want acc-123", state.Account.ID)
	}
	if state.CurrentLimits["rpm"] != 50 {
		t.Errorf("AccountState.CurrentLimits[rpm] = %v, want 50", state.CurrentLimits["rpm"])
	}
	if !state.IsAvailable() {
		t.Error("AccountState should be available")
	}
}

func TestErrorCodes(t *testing.T) {
	codes := []struct {
		name     string
		code     string
		expected string
	}{
		{"account not found", ErrCodeAccountNotFound, "ACCOUNT_NOT_FOUND"},
		{"rate limit exceeded", ErrCodeRateLimitExceeded, "RATE_LIMIT_EXCEEDED"},
		{"no available account", ErrCodeNoAvailableAccount, "NO_AVAILABLE_ACCOUNT"},
		{"provider not found", ErrCodeProviderNotFound, "PROVIDER_NOT_FOUND"},
		{"invalid config", ErrCodeInvalidConfig, "INVALID_CONFIG"},
	}

	for _, tt := range codes {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("ErrorCode = %v, want %v", tt.code, tt.expected)
			}
		})
	}
}
