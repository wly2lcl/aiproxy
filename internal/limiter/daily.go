package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

type Daily struct {
	store   storage.Storage
	max     int
	mu      sync.RWMutex
	counts  map[string]int
	windows map[string]time.Time
}

func NewDaily(store storage.Storage, max int) *Daily {
	return &Daily{
		store:   store,
		max:     max,
		counts:  make(map[string]int),
		windows: make(map[string]time.Time),
	}
}

func (d *Daily) Allow(ctx context.Context, key string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	windowStart := d.getWindowStart(now)

	currentWindow, exists := d.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		d.counts[key] = 0
		d.windows[key] = windowStart
	}

	if d.counts[key] >= d.max {
		return false, nil
	}

	d.counts[key]++
	return true, nil
}

func (d *Daily) Record(ctx context.Context, key string, delta int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	windowStart := d.getWindowStart(now)

	currentWindow, exists := d.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		d.counts[key] = 0
		d.windows[key] = windowStart
	}

	d.counts[key] += delta
	return d.store.IncrementRateLimit(ctx, key, domain.LimitTypeDaily, delta)
}

func (d *Daily) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	now := time.Now().UTC()
	windowStart := d.getWindowStart(now)
	windowEnd := windowStart.Add(24 * time.Hour)

	currentWindow, exists := d.windows[key]
	count := 0
	if exists && currentWindow.Equal(windowStart) {
		count = d.counts[key]
	}

	return &domain.LimitState{
		Type:        domain.LimitTypeDaily,
		Current:     count,
		Max:         d.max,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, nil
}

func (d *Daily) Reset(ctx context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.counts, key)
	delete(d.windows, key)
	return d.store.ResetRateLimit(ctx, key, domain.LimitTypeDaily)
}

func (d *Daily) LimitType() domain.LimitType {
	return domain.LimitTypeDaily
}

func (d *Daily) getWindowStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
