package pool

import (
	"context"
	"sync"
	"testing"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/limiter"
)

type mockLimiter struct {
	mu          sync.RWMutex
	allowed     map[string]bool
	allowErrors map[string]error
}

func newMockLimiter() *mockLimiter {
	return &mockLimiter{
		allowed:     make(map[string]bool),
		allowErrors: make(map[string]error),
	}
}

func (m *mockLimiter) setAllowed(key string, allowed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowed[key] = allowed
}

func (m *mockLimiter) setAllowError(key string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowErrors[key] = err
}

func (m *mockLimiter) Allow(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err, ok := m.allowErrors[key]; ok {
		return false, err
	}
	if allowed, ok := m.allowed[key]; ok {
		return allowed, nil
	}
	return true, nil
}

func (m *mockLimiter) Record(ctx context.Context, key string, delta int) error {
	return nil
}

func (m *mockLimiter) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	return &domain.LimitState{Type: domain.LimitTypeRPM, Current: 0, Max: 100}, nil
}

func (m *mockLimiter) Reset(ctx context.Context, key string) error {
	return nil
}

func (m *mockLimiter) LimitType() domain.LimitType {
	return domain.LimitTypeRPM
}

func createTestAccount(id string, weight int, enabled bool) *domain.Account {
	return &domain.Account{
		ID:         id,
		ProviderID: "test-provider",
		APIKeyHash: "hash-" + id,
		Weight:     weight,
		Priority:   0,
		IsEnabled:  enabled,
	}
}

func TestPool_Add(t *testing.T) {
	p := NewPool(nil)

	acc := createTestAccount("acc1", 1, true)
	p.Add(acc)

	if len(p.List()) != 1 {
		t.Errorf("expected 1 account, got %d", len(p.List()))
	}

	got, err := p.Get("acc1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "acc1" {
		t.Errorf("expected ID acc1, got %s", got.ID)
	}
}

func TestPool_Remove(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	})

	if len(p.List()) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(p.List()))
	}

	p.Remove("acc1")

	if len(p.List()) != 1 {
		t.Errorf("expected 1 account after remove, got %d", len(p.List()))
	}

	_, err := p.Get("acc1")
	if err == nil {
		t.Error("expected error for removed account")
	}
}

func TestPool_Get(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	acc, err := p.Get("acc1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if acc.ID != "acc1" {
		t.Errorf("expected ID acc1, got %s", acc.ID)
	}

	_, err = p.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_List(t *testing.T) {
	accounts := []*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 2, true),
		createTestAccount("acc3", 3, true),
	}
	p := NewPool(accounts)

	list := p.List()
	if len(list) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(list))
	}

	for i, expectedID := range []string{"acc1", "acc2", "acc3"} {
		if list[i].ID != expectedID {
			t.Errorf("expected account %s at position %d, got %s", expectedID, i, list[i].ID)
		}
	}
}

func TestPool_SetEnabled(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	err := p.SetEnabled("acc1", false)
	if err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}

	acc, _ := p.Get("acc1")
	if acc.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}

	err = p.SetEnabled("nonexistent", false)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_UpdateWeight(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	err := p.UpdateWeight("acc1", 5)
	if err != nil {
		t.Fatalf("UpdateWeight failed: %v", err)
	}

	acc, _ := p.Get("acc1")
	if acc.Weight != 5 {
		t.Errorf("expected weight 5, got %d", acc.Weight)
	}

	err = p.UpdateWeight("nonexistent", 5)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_RecordSuccess_Failure(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	p.RecordFailure("acc1")
	p.RecordFailure("acc1")
	state := p.GetState("acc1")
	if state.ConsecutiveFailures != 2 {
		t.Errorf("expected 2 consecutive failures, got %d", state.ConsecutiveFailures)
	}

	p.RecordSuccess("acc1")
	state = p.GetState("acc1")
	if state.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures after success, got %d", state.ConsecutiveFailures)
	}
}

func TestPool_ResetFailures(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	p.RecordFailure("acc1")
	p.RecordFailure("acc1")
	p.RecordFailure("acc1")

	state := p.GetState("acc1")
	if state.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", state.ConsecutiveFailures)
	}

	p.ResetFailures("acc1")
	state = p.GetState("acc1")
	if state.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures after reset, got %d", state.ConsecutiveFailures)
	}
}

func TestSelector_WeightedRoundRobin(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	})

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()
	selected := make(map[string]int)

	for i := 0; i < 100; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selected[acc.ID]++
	}

	if selected["acc1"] == 0 || selected["acc2"] == 0 {
		t.Error("expected both accounts to be selected")
	}
}

func TestSelector_NoAvailableAccount(t *testing.T) {
	p := NewPool(nil)

	selector := NewWeightedRoundRobin(p, map[string]*limiter.CompositeLimiter{})

	ctx := context.Background()
	_, err := selector.Select(ctx, nil)
	if err != ErrNoAvailableAccount {
		t.Errorf("expected ErrNoAvailableAccount, got %v", err)
	}
}

func TestSelector_RateLimitFilter(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	})

	ml := newMockLimiter()
	ml.setAllowed("acc1", false)

	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if acc.ID != "acc2" {
			t.Errorf("expected acc2, got %s", acc.ID)
		}
	}
}

func TestSelector_CircuitBreakerFilter(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	})

	for i := 0; i < domain.CircuitBreakerThreshold; i++ {
		p.RecordFailure("acc1")
	}

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if acc.ID != "acc2" {
			t.Errorf("expected acc2 (acc1 should be in circuit breaker), got %s", acc.ID)
		}
	}
}

func TestSelector_WeightDistribution(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 3, true),
		createTestAccount("acc2", 1, true),
	})

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()
	selected := make(map[string]int)

	for i := 0; i < 1000; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		selected[acc.ID]++
	}

	ratio := float64(selected["acc1"]) / float64(selected["acc2"])
	if ratio < 2.0 || ratio > 4.0 {
		t.Errorf("expected weight ratio ~3, got %.2f (acc1: %d, acc2: %d)", ratio, selected["acc1"], selected["acc2"])
	}
}

func TestPool_ConcurrentAccess(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	})

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				p.List()
				p.Get("acc1")
				p.RecordSuccess("acc1")
				p.RecordFailure("acc2")
			}
		}()
	}

	wg.Wait()

	state1 := p.GetState("acc1")
	state2 := p.GetState("acc2")

	if state1.ConsecutiveFailures != 0 {
		t.Errorf("expected acc1 consecutive failures to be 0, got %d", state1.ConsecutiveFailures)
	}

	if state2.ConsecutiveFailures != numGoroutines*numOperations {
		t.Errorf("expected acc2 consecutive failures to be %d, got %d", numGoroutines*numOperations, state2.ConsecutiveFailures)
	}
}

func TestSelector_DisabledAccount(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, false),
		createTestAccount("acc2", 1, true),
	})

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if acc.ID != "acc2" {
			t.Errorf("expected acc2 (acc1 is disabled), got %s", acc.ID)
		}
	}
}

func TestSelector_ZeroWeight(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 0, true),
		createTestAccount("acc2", 1, true),
	})

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if acc.ID != "acc2" {
			t.Errorf("expected acc2 (acc1 has weight 0), got %s", acc.ID)
		}
	}
}

func TestSelector_AllFiltered(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	ml := newMockLimiter()
	ml.setAllowed("acc1", false)

	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()
	_, err := selector.Select(ctx, nil)
	if err != ErrNoAvailableAccount {
		t.Errorf("expected ErrNoAvailableAccount when all accounts filtered, got %v", err)
	}
}

func TestSelector_NilLimiter(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
	})

	selector := NewWeightedRoundRobin(p, nil)

	ctx := context.Background()
	acc, err := selector.Select(ctx, nil)
	if err != nil {
		t.Fatalf("Select failed with nil limiter: %v", err)
	}
	if acc.ID != "acc1" {
		t.Errorf("expected acc1, got %s", acc.ID)
	}
}

func TestPool_OrderPreserved(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
		createTestAccount("acc3", 1, true),
	})

	p.Remove("acc2")
	p.Add(createTestAccount("acc2", 1, true))

	list := p.List()
	expected := []string{"acc1", "acc3", "acc2"}
	for i, acc := range list {
		if acc.ID != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, acc.ID)
		}
	}
}

// TestAccountSwitchingRetry tests that the system can switch to different accounts
// when one fails, simulating the retry behavior in main.go
func TestAccountSwitchingRetry(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
		createTestAccount("acc3", 1, true),
	})

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
		"acc3": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()
	excludedAccounts := make(map[string]bool)
	selectedAccounts := make([]string, 0, 3)

	// Simulate selecting 3 different accounts, excluding each one after use
	for len(excludedAccounts) < 3 {
		// Try to find a non-excluded account
		found := false
		for i := 0; i < 10; i++ { // Max tries to avoid infinite loop
			acc, err := selector.Select(ctx, nil)
			if err != nil {
				t.Fatalf("Select failed: %v", err)
			}
			if !excludedAccounts[acc.ID] {
				selectedAccounts = append(selectedAccounts, acc.ID)
				excludedAccounts[acc.ID] = true
				// Simulate recording failure to mark account as unavailable
				p.RecordFailure(acc.ID)
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Could not find non-excluded account")
		}
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

	// Verify we have all 3 accounts
	for _, expectedID := range []string{"acc1", "acc2", "acc3"} {
		if !seen[expectedID] {
			t.Errorf("expected account %s to be selected", expectedID)
		}
	}
}

// TestAccountSwitchingWithCircuitBreaker tests that accounts with circuit breaker open
// are excluded from selection
func TestAccountSwitchingWithCircuitBreaker(t *testing.T) {
	p := NewPool([]*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
		createTestAccount("acc3", 1, true),
	})

	// Simulate acc1 and acc2 reaching circuit breaker threshold
	for i := 0; i < domain.CircuitBreakerThreshold; i++ {
		p.RecordFailure("acc1")
		p.RecordFailure("acc2")
	}

	ml := newMockLimiter()
	composite := limiter.NewCompositeLimiter(ml)
	limiters := map[string]*limiter.CompositeLimiter{
		"acc1": composite,
		"acc2": composite,
		"acc3": composite,
	}
	selector := NewWeightedRoundRobin(p, limiters)

	ctx := context.Background()

	// Only acc3 should be available
	for i := 0; i < 5; i++ {
		acc, err := selector.Select(ctx, nil)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if acc.ID != "acc3" {
			t.Errorf("expected acc3 (others have circuit breaker open), got %s", acc.ID)
		}
	}
}
