package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

type Monthly struct {
	store      storage.Storage
	max        int
	mu         sync.RWMutex
	counts     map[string]int
	windows    map[string]time.Time
	lastAccess map[string]time.Time
}

func NewMonthly(store storage.Storage, max int) *Monthly {
	return &Monthly{
		store:      store,
		max:        max,
		counts:     make(map[string]int),
		windows:    make(map[string]time.Time),
		lastAccess: make(map[string]time.Time),
	}
}

func (m *Monthly) Allow(ctx context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	windowStart := m.getWindowStart(now)

	// Track last access
	m.lastAccess[key] = now

	currentWindow, exists := m.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		m.counts[key] = 0
		m.windows[key] = windowStart
	}

	return m.counts[key] < m.max, nil
}

func (m *Monthly) Record(ctx context.Context, key string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	windowStart := m.getWindowStart(now)

	// Track last access
	m.lastAccess[key] = now

	currentWindow, exists := m.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		m.counts[key] = 0
		m.windows[key] = windowStart
	}

	m.counts[key] += delta
	return m.store.IncrementRateLimit(ctx, key, domain.LimitTypeMonthly, delta)
}

func (m *Monthly) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().UTC()
	windowStart := m.getWindowStart(now)
	windowEnd := m.getWindowEnd(now)

	currentWindow, exists := m.windows[key]
	count := 0
	if exists && currentWindow.Equal(windowStart) {
		count = m.counts[key]
	}

	return &domain.LimitState{
		Type:        domain.LimitTypeMonthly,
		Current:     count,
		Max:         m.max,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, nil
}

func (m *Monthly) Reset(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.counts, key)
	delete(m.windows, key)
	return m.store.ResetRateLimit(ctx, key, domain.LimitTypeMonthly)
}

func (m *Monthly) LimitType() domain.LimitType {
	return domain.LimitTypeMonthly
}

func (m *Monthly) getWindowStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (m *Monthly) getWindowEnd(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// LoadState loads persisted state from database into memory
func (m *Monthly) LoadState(ctx context.Context, key string, state *domain.LimitState) error {
	if state == nil || state.Current <= 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	windowStart := m.getWindowStart(now)

	// Only load state if it's from the current month
	if state.WindowStart.Equal(windowStart) {
		m.counts[key] = state.Current
		m.windows[key] = windowStart
		m.lastAccess[key] = now
	}

	return nil
}

// CleanupStale removes entries that haven't been accessed for more than maxAge
func (m *Monthly) CleanupStale(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-maxAge)
	removed := 0

	for key, lastAccess := range m.lastAccess {
		if lastAccess.Before(cutoff) {
			delete(m.counts, key)
			delete(m.windows, key)
			delete(m.lastAccess, key)
			removed++
		}
	}

	return removed
}
