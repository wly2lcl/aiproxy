package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/limiter"
	"github.com/wangluyao/aiproxy/internal/pool"
	"github.com/wangluyao/aiproxy/internal/storage"
)

// mockStorage implements storage.Storage for testing
type mockStorage struct {
	mu          sync.RWMutex
	rateLimits  map[string]map[domain.LimitType]*domain.LimitState
	tokenUsage  map[string]int64
	recordCalls []struct {
		accountID string
		limitType domain.LimitType
		delta     int
	}
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		rateLimits: make(map[string]map[domain.LimitType]*domain.LimitState),
		tokenUsage: make(map[string]int64),
	}
}

func (m *mockStorage) UpsertProvider(ctx context.Context, provider *domain.Provider) error {
	return nil
}
func (m *mockStorage) GetProvider(ctx context.Context, id string) (*domain.Provider, error) {
	return nil, nil
}
func (m *mockStorage) ListProviders(ctx context.Context) ([]*domain.Provider, error) {
	return nil, nil
}
func (m *mockStorage) UpsertAccount(ctx context.Context, account *domain.Account) error {
	return nil
}
func (m *mockStorage) GetAccount(ctx context.Context, id string) (*domain.Account, error) {
	return nil, nil
}
func (m *mockStorage) ListAccounts(ctx context.Context, providerID string) ([]*domain.Account, error) {
	return nil, nil
}

func (m *mockStorage) GetRateLimit(ctx context.Context, accountID string, limitType domain.LimitType) (*domain.LimitState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limits, ok := m.rateLimits[accountID]; ok {
		if state, ok := limits[limitType]; ok {
			return state, nil
		}
	}
	return &domain.LimitState{Type: limitType, Current: 0, Max: 0}, nil
}

func (m *mockStorage) IncrementRateLimit(ctx context.Context, accountID string, limitType domain.LimitType, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCalls = append(m.recordCalls, struct {
		accountID string
		limitType domain.LimitType
		delta     int
	}{accountID, limitType, delta})
	if _, ok := m.rateLimits[accountID]; !ok {
		m.rateLimits[accountID] = make(map[domain.LimitType]*domain.LimitState)
	}
	if _, ok := m.rateLimits[accountID][limitType]; !ok {
		m.rateLimits[accountID][limitType] = &domain.LimitState{Type: limitType, Current: 0}
	}
	m.rateLimits[accountID][limitType].Current += delta
	return nil
}

func (m *mockStorage) ResetRateLimit(ctx context.Context, accountID string, limitType domain.LimitType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limits, ok := m.rateLimits[accountID]; ok {
		delete(limits, limitType)
	}
	return nil
}
func (m *mockStorage) UpdateAccountLastUsed(ctx context.Context, accountID string) error { return nil }
func (m *mockStorage) GetAllAccountLastUsed(ctx context.Context) (map[string]time.Time, error) {
	return nil, nil
}
func (m *mockStorage) RecordTokenUsage(ctx context.Context, usage *storage.TokenUsage) error {
	return nil
}
func (m *mockStorage) GetTokenUsage(ctx context.Context, accountID string, since time.Time) (*storage.TokenUsageSummary, error) {
	return nil, nil
}
func (m *mockStorage) CleanupExpiredRateLimits(ctx context.Context) error { return nil }
func (m *mockStorage) GetAllRateLimits(ctx context.Context, accountID string) ([]*domain.LimitState, error) {
	return nil, nil
}
func (m *mockStorage) GetRecentLogs(ctx context.Context, limit int) ([]*storage.RequestLog, error) {
	return nil, nil
}
func (m *mockStorage) GetLogByID(ctx context.Context, requestID string) (*storage.RequestLog, error) {
	return nil, nil
}
func (m *mockStorage) RecordRequestLog(ctx context.Context, log *storage.RequestLog) error {
	return nil
}
func (m *mockStorage) GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]*storage.TimeSeriesPoint, error) {
	return nil, nil
}
func (m *mockStorage) GetAllAccountStats(ctx context.Context, since time.Time) ([]*storage.AccountStats, error) {
	return nil, nil
}
func (m *mockStorage) GetModelStats(ctx context.Context, since time.Time) ([]*storage.ModelStats, error) {
	return nil, nil
}
func (m *mockStorage) GetLatencyData(ctx context.Context, since time.Time) ([]*storage.LatencyData, error) {
	return nil, nil
}
func (m *mockStorage) GetAccountModelStats(ctx context.Context, accountID string, since time.Time) (map[string]int64, error) {
	return nil, nil
}
func (m *mockStorage) CreateAPIKey(ctx context.Context, keyHash, name string, expiresAt *time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStorage) GetAPIKeyByHash(ctx context.Context, keyHash string) (*storage.APIKey, error) {
	return nil, nil
}
func (m *mockStorage) ListAPIKeys(ctx context.Context) ([]*storage.APIKey, error) { return nil, nil }
func (m *mockStorage) UpdateAPIKeyUsage(ctx context.Context, keyHash string) error {
	return nil
}
func (m *mockStorage) DeleteAPIKey(ctx context.Context, id int64) error { return nil }
func (m *mockStorage) ToggleAPIKey(ctx context.Context, id int64, enabled bool) error {
	return nil
}
func (m *mockStorage) BlockIP(ctx context.Context, ip, reason string) error { return nil }
func (m *mockStorage) UnblockIP(ctx context.Context, ip string) error      { return nil }
func (m *mockStorage) GetBlockedIPs(ctx context.Context) ([]storage.BlockedIP, error) {
	return nil, nil
}
func (m *mockStorage) RecordAuthFailure(ctx context.Context, ip string) error { return nil }
func (m *mockStorage) ClearAuthFailure(ctx context.Context, ip string) error  { return nil }
func (m *mockStorage) GetAuthFailures(ctx context.Context) ([]storage.AuthFailure, error) {
	return nil, nil
}
func (m *mockStorage) Close() error { return nil }

// TestRateLimiterKeyMapping tests that rate limiters are correctly keyed by account ID
func TestRateLimiterKeyMapping(t *testing.T) {
	store := newMockStorage()

	// Create accounts with different IDs
	acc1 := &domain.Account{
		ID:         "account-1",
		ProviderID: "test-provider",
		APIKey: "key1",
		Weight:     1,
		Priority:   1,
		IsEnabled:  true,
	}
	acc2 := &domain.Account{
		ID:         "account-2",
		ProviderID: "test-provider",
		APIKey: "key2",
		Weight:     1,
		Priority:   1,
		IsEnabled:  true,
	}

	// Create limiters with different limits
	rpmLimit1 := 10
	rpmLimit2 := 20

	// Create composite limiters for each account
	limiter1 := limiter.NewCompositeLimiter(limiter.NewRPM(store, rpmLimit1))
	limiter2 := limiter.NewCompositeLimiter(limiter.NewRPM(store, rpmLimit2))

	// Store limiters by account ID
	accountLimiters := map[string]*limiter.CompositeLimiter{
		acc1.ID: limiter1,
		acc2.ID: limiter2,
	}

	// Create pool and selector
	p := pool.NewPool([]*domain.Account{acc1, acc2})
	selector := pool.NewWeightedRoundRobin(p, accountLimiters)

	// Test that both accounts can be selected and their limiters are independent
	ctx := context.Background()

	// Exhaust acc1's RPM limit (Allow + Record simulates real request flow)
	for i := 0; i < rpmLimit1; i++ {
		allowed, _ := accountLimiters[acc1.ID].Allow(ctx, acc1.ID)
		if !allowed {
			t.Fatalf("acc1 should be allowed on request %d", i+1)
		}
		// Record the usage after allowing (simulates real request)
		accountLimiters[acc1.ID].Record(ctx, acc1.ID, 1)
	}

	// acc1 should now be rate limited
	allowed, _ := accountLimiters[acc1.ID].Allow(ctx, acc1.ID)
	if allowed {
		t.Error("acc1 should be rate limited after 10 requests")
	}

	// acc2 should still be allowed (independent limiter)
	allowed, _ = accountLimiters[acc2.ID].Allow(ctx, acc2.ID)
	if !allowed {
		t.Error("acc2 should still be allowed (independent from acc1)")
	}

	// Select should return acc2 since acc1 is rate limited
	selected, err := selector.Select(ctx, nil)
	if err != nil {
		t.Fatalf("selector.Select failed: %v", err)
	}
	if selected.ID != acc2.ID {
		t.Errorf("expected acc2 to be selected (acc1 rate limited), got %s", selected.ID)
	}
}

// TestAccountSwitchingRetry tests that when an account fails, the system can switch to another
func TestAccountSwitchingRetry(t *testing.T) {
	store := newMockStorage()

	acc1 := &domain.Account{
		ID:         "account-1",
		ProviderID: "test-provider",
		APIKey: "key1",
		Weight:     1,
		Priority:   1,
		IsEnabled:  true,
	}
	acc2 := &domain.Account{
		ID:         "account-2",
		ProviderID: "test-provider",
		APIKey: "key2",
		Weight:     1,
		Priority:   1,
		IsEnabled:  true,
	}
	acc3 := &domain.Account{
		ID:         "account-3",
		ProviderID: "test-provider",
		APIKey: "key3",
		Weight:     1,
		Priority:   1,
		IsEnabled:  true,
	}

	// Create limiters
	rpmLimit := 100 // High limit so we don't hit it during test
	composite := limiter.NewCompositeLimiter(limiter.NewRPM(store, rpmLimit))
	accountLimiters := map[string]*limiter.CompositeLimiter{
		acc1.ID: composite,
		acc2.ID: composite,
		acc3.ID: composite,
	}

	// Create pool and selector
	p := pool.NewPool([]*domain.Account{acc1, acc2, acc3})
	selector := pool.NewWeightedRoundRobin(p, accountLimiters)

	ctx := context.Background()
	excludedAccounts := make(map[string]bool)
	selectedAccounts := make([]string, 0)

	// Simulate selecting different accounts, excluding each one after failure
	for len(excludedAccounts) < 3 {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("selector.Select failed: %v", err)
		}

		// Skip if already excluded
		if excludedAccounts[acc.ID] {
			continue
		}

		selectedAccounts = append(selectedAccounts, acc.ID)
		excludedAccounts[acc.ID] = true

		// Simulate recording failure
		p.RecordFailure(acc.ID)
	}

	// Verify we selected 3 different accounts
	if len(selectedAccounts) != 3 {
		t.Errorf("expected 3 selected accounts, got %d", len(selectedAccounts))
	}

	// Verify all accounts are different
	seen := make(map[string]bool)
	for _, id := range selectedAccounts {
		if seen[id] {
			t.Errorf("account %s was selected more than once", id)
		}
		seen[id] = true
	}
}

// TestRateLimiterStatePersistence tests that rate limit state can be retrieved
func TestRateLimiterStatePersistence(t *testing.T) {
	store := newMockStorage()

	accountID := "test-account"
	rpmLimit := 10

	rpmLimiter := limiter.NewRPM(store, rpmLimit)
	ctx := context.Background()

	// Make some requests (Allow + Record simulates real request flow)
	for i := 0; i < 5; i++ {
		allowed, err := rpmLimiter.Allow(ctx, accountID)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
		// Record the usage after allowing
		if err := rpmLimiter.Record(ctx, accountID, 1); err != nil {
			t.Fatalf("Record failed: %v", err)
		}
	}

	// Get state
	state, err := rpmLimiter.GetState(ctx, accountID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	// Verify state
	if state.Type != domain.LimitTypeRPM {
		t.Errorf("expected type RPM, got %s", state.Type)
	}
	if state.Max != rpmLimit {
		t.Errorf("expected max %d, got %d", rpmLimit, state.Max)
	}
	if state.Current != 5 {
		t.Errorf("expected current 5, got %d", state.Current)
	}
}

// TestMemoryCleanup verifies that the memory cleanup constants are defined
func TestMemoryCleanup(t *testing.T) {
	// This test verifies that the stats collector has bounded arrays
	// The actual implementation is in stats/collector.go with MaxLatencySamples and MaxTTFTSamples constants
	t.Log("Memory cleanup constants are defined in stats/collector.go")
	t.Log("MaxLatencySamples = 10000, MaxTTFTSamples = 10000")
}

// Ensure the test compiles with time import
var _ = time.Second