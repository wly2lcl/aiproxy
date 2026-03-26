package limiter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

type mockStorage struct {
	mu          sync.RWMutex
	rateLimits  map[string]map[domain.LimitType]*domain.LimitState
	tokenUsage  map[string]*storage.TokenUsageSummary
	recordCalls []struct {
		accountID string
		limitType domain.LimitType
		delta     int
	}
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		rateLimits: make(map[string]map[domain.LimitType]*domain.LimitState),
		tokenUsage: make(map[string]*storage.TokenUsageSummary),
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
	return nil, nil
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

	if state, ok := m.rateLimits[accountID][limitType]; ok {
		state.Current += delta
	} else {
		m.rateLimits[accountID][limitType] = &domain.LimitState{
			Type:    limitType,
			Current: delta,
			Max:     1000,
		}
	}
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

func (m *mockStorage) RecordTokenUsage(ctx context.Context, usage *storage.TokenUsage) error {
	return nil
}

func (m *mockStorage) GetTokenUsage(ctx context.Context, accountID string, since time.Time) (*storage.TokenUsageSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if summary, ok := m.tokenUsage[accountID]; ok {
		return summary, nil
	}
	return &storage.TokenUsageSummary{}, nil
}

func (m *mockStorage) Close() error {
	return nil
}

func TestRPM_Allow_UnderLimit(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 10)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}
}

func TestRPM_Allow_OverLimit(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 5)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}

	allowed, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if allowed {
		t.Error("expected denied when over limit")
	}
}

func TestRPM_SlidingWindow(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 3)
	ctx := context.Background()
	key := "test-account"

	allowed, _ := limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected first request allowed")
	}

	allowed, _ = limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected second request allowed")
	}

	allowed, _ = limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected third request allowed")
	}

	allowed, _ = limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected fourth request denied")
	}
}

func TestRPM_Expiry(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 2)
	ctx := context.Background()
	key := "test-account"

	limiter.Allow(ctx, key)
	limiter.Allow(ctx, key)

	allowed, _ := limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected denied at limit")
	}

	limiter.mu.Lock()
	for k, sw := range limiter.windows {
		sw.timestamps[0] = time.Now().UTC().Add(-2 * time.Minute)
		sw.timestamps[1] = time.Now().UTC().Add(-90 * time.Second)
		limiter.windows[k] = sw
	}
	limiter.mu.Unlock()

	state, err := limiter.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.Current != 0 {
		t.Errorf("expected current 0 after expiry, got %d", state.Current)
	}
}

func TestRPM_Reset(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 2)
	ctx := context.Background()
	key := "test-account"

	limiter.Allow(ctx, key)
	limiter.Allow(ctx, key)

	allowed, _ := limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected denied at limit")
	}

	err := limiter.Reset(ctx, key)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	allowed, _ = limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected allowed after reset")
	}
}

func TestDaily_Allow_UnderLimit(t *testing.T) {
	store := newMockStorage()
	limiter := NewDaily(store, 10)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}
}

func TestDaily_Allow_OverLimit(t *testing.T) {
	store := newMockStorage()
	limiter := NewDaily(store, 3)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 3; i++ {
		allowed, _ := limiter.Allow(ctx, key)
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}

	allowed, _ := limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected denied when over limit")
	}
}

func TestDaily_ResetAtMidnight(t *testing.T) {
	store := newMockStorage()
	limiter := NewDaily(store, 2)
	ctx := context.Background()
	key := "test-account"

	limiter.Allow(ctx, key)
	limiter.Allow(ctx, key)

	allowed, _ := limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected denied at limit")
	}

	now := time.Now().UTC()
	limiter.mu.Lock()
	limiter.windows[key] = time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
	limiter.counts[key] = 100
	limiter.mu.Unlock()

	allowed, _ = limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected allowed after window reset")
	}
}

func TestWindow_Allow_UnderLimit(t *testing.T) {
	store := newMockStorage()
	limiter := NewWindow5h(store, 10)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}
}

func TestWindow_5HourRolling(t *testing.T) {
	store := newMockStorage()
	limiter := NewWindow5h(store, 2)
	ctx := context.Background()
	key := "test-account"

	limiter.Allow(ctx, key)
	limiter.Allow(ctx, key)

	allowed, _ := limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected denied at limit")
	}

	limiter.mu.Lock()
	for k, rw := range limiter.windows {
		rw.timestamps[0] = time.Now().UTC().Add(-6 * time.Hour)
		limiter.windows[k] = rw
	}
	limiter.mu.Unlock()

	state, err := limiter.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.Current != 1 {
		t.Errorf("expected current 1 after 5h expiry, got %d", state.Current)
	}
}

func TestMonthly_Allow_UnderLimit(t *testing.T) {
	store := newMockStorage()
	limiter := NewMonthly(store, 100)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 50; i++ {
		allowed, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}
}

func TestMonthly_FirstOfMonth(t *testing.T) {
	store := newMockStorage()
	limiter := NewMonthly(store, 2)
	ctx := context.Background()
	key := "test-account"

	limiter.Allow(ctx, key)
	limiter.Allow(ctx, key)

	allowed, _ := limiter.Allow(ctx, key)
	if allowed {
		t.Error("expected denied at limit")
	}

	now := time.Now().UTC()
	limiter.mu.Lock()
	limiter.windows[key] = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	limiter.counts[key] = 100
	limiter.mu.Unlock()

	allowed, _ = limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected allowed after month reset")
	}
}

func TestToken_TrackUsage(t *testing.T) {
	store := newMockStorage()
	limiter := NewTokenDaily(store, 1000)
	ctx := context.Background()
	key := "test-account"

	allowed, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !allowed {
		t.Error("expected allowed under limit")
	}

	err = limiter.EstimateAndRecord(ctx, key, "Hello world", "Hi there!")
	if err != nil {
		t.Fatalf("EstimateAndRecord failed: %v", err)
	}

	prompt, completion, estimated := limiter.GetUsage(ctx, key)
	if prompt != 3 {
		t.Errorf("expected prompt tokens 3, got %d", prompt)
	}
	if completion != 3 {
		t.Errorf("expected completion tokens 3, got %d", completion)
	}
	if !estimated {
		t.Error("expected estimated to be true")
	}
}

func TestToken_HybridMode(t *testing.T) {
	store := newMockStorage()
	limiter := NewTokenDaily(store, 1000)
	ctx := context.Background()
	key := "test-account"

	err := limiter.EstimateAndRecord(ctx, key, "This is a test prompt", "This is a test response")
	if err != nil {
		t.Fatalf("EstimateAndRecord failed: %v", err)
	}

	prompt, completion, estimated := limiter.GetUsage(ctx, key)
	if !estimated {
		t.Error("expected estimated to be true after initial record")
	}

	err = limiter.RecordActual(ctx, key, 10, 8)
	if err != nil {
		t.Fatalf("RecordActual failed: %v", err)
	}

	prompt, completion, estimated = limiter.GetUsage(ctx, key)
	if prompt != 10 {
		t.Errorf("expected prompt tokens 10, got %d", prompt)
	}
	if completion != 8 {
		t.Errorf("expected completion tokens 8, got %d", completion)
	}
	if estimated {
		t.Error("expected estimated to be false after RecordActual")
	}
}

func TestToken_EstimateTokens(t *testing.T) {
	store := newMockStorage()
	limiter := NewTokenDaily(store, 1000)

	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"a", 1},
		{"ab", 1},
		{"abc", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"Hello world", 3},
		{"This is a longer piece of text", 8},
	}

	for _, tt := range tests {
		result := limiter.EstimateTokens(tt.text)
		if result != tt.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.text, result, tt.expected)
		}
	}
}

func TestCompositeLimiter_AllLimits(t *testing.T) {
	store := newMockStorage()
	rpm := NewRPM(store, 5)
	daily := NewDaily(store, 10)

	composite := NewCompositeLimiter(rpm, daily)
	ctx := context.Background()
	key := "test-account"

	for i := 0; i < 5; i++ {
		allowed, err := composite.Allow(ctx, key)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed at iteration %d", i)
		}
	}

	allowed, _ := composite.Allow(ctx, key)
	if allowed {
		t.Error("expected denied when RPM limit reached")
	}
}

func TestCompositeLimiter_GetStates(t *testing.T) {
	store := newMockStorage()
	rpm := NewRPM(store, 5)
	daily := NewDaily(store, 10)

	composite := NewCompositeLimiter(rpm, daily)
	ctx := context.Background()
	key := "test-account"

	composite.Allow(ctx, key)
	composite.Allow(ctx, key)

	states, err := composite.GetStates(ctx, key)
	if err != nil {
		t.Fatalf("GetStates failed: %v", err)
	}

	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}

	rpmState := states[domain.LimitTypeRPM]
	if rpmState == nil {
		t.Fatal("expected RPM state")
	}
	if rpmState.Current != 2 {
		t.Errorf("expected RPM current 2, got %d", rpmState.Current)
	}

	dailyState := states[domain.LimitTypeDaily]
	if dailyState == nil {
		t.Fatal("expected Daily state")
	}
	if dailyState.Current != 2 {
		t.Errorf("expected Daily current 2, got %d", dailyState.Current)
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 1000)
	ctx := context.Background()
	key := "test-account"

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				limiter.Allow(ctx, key)
				limiter.GetState(ctx, key)
			}
		}()
	}

	wg.Wait()

	state, err := limiter.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if state.Current > 1000 {
		t.Errorf("current %d exceeds max", state.Current)
	}
}

func TestLimiter_ConcurrentAccess_Daily(t *testing.T) {
	store := newMockStorage()
	limiter := NewDaily(store, 1000)
	ctx := context.Background()
	key := "test-account"

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				limiter.Allow(ctx, key)
				limiter.GetState(ctx, key)
			}
		}()
	}

	wg.Wait()

	state, err := limiter.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if state.Current > 1000 {
		t.Errorf("current %d exceeds max", state.Current)
	}
}

func TestLimiter_ConcurrentAccess_Composite(t *testing.T) {
	store := newMockStorage()
	rpm := NewRPM(store, 500)
	daily := NewDaily(store, 500)
	composite := NewCompositeLimiter(rpm, daily)

	ctx := context.Background()
	key := "test-account"

	const numGoroutines = 10
	const numOperations = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				composite.Allow(ctx, key)
				composite.GetState(ctx, key)
			}
		}()
	}

	wg.Wait()

	state, err := composite.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if state.Current > 500 {
		t.Errorf("current %d exceeds max", state.Current)
	}
}

func TestRPM_Record(t *testing.T) {
	store := newMockStorage()
	limiter := NewRPM(store, 10)
	ctx := context.Background()
	key := "test-account"

	err := limiter.Record(ctx, key, 5)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if len(store.recordCalls) != 1 {
		t.Errorf("expected 1 record call, got %d", len(store.recordCalls))
	}

	if store.recordCalls[0].delta != 5 {
		t.Errorf("expected delta 5, got %d", store.recordCalls[0].delta)
	}

	state, _ := limiter.GetState(ctx, key)
	if state.Current != 5 {
		t.Errorf("expected current 5, got %d", state.Current)
	}
}

func TestToken_Monthly(t *testing.T) {
	store := newMockStorage()
	limiter := NewTokenMonthly(store, 1000)
	ctx := context.Background()
	key := "test-account"

	if limiter.LimitType() != domain.LimitTypeTokenMonthly {
		t.Errorf("expected LimitTypeTokenMonthly, got %s", limiter.LimitType())
	}

	allowed, _ := limiter.Allow(ctx, key)
	if !allowed {
		t.Error("expected allowed under limit")
	}

	err := limiter.EstimateAndRecord(ctx, key, "Test", "Response")
	if err != nil {
		t.Fatalf("EstimateAndRecord failed: %v", err)
	}

	state, err := limiter.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.Current == 0 {
		t.Error("expected non-zero current")
	}
}
