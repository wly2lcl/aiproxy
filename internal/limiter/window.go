package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

type Window5h struct {
	store      storage.Storage
	max        int
	mu         sync.RWMutex
	windows    map[string]*rollingWindow
	windowSz   time.Duration
	lastAccess map[string]time.Time
}

type rollingWindow struct {
	counts     []int
	timestamps []time.Time
}

func NewWindow5h(store storage.Storage, max int) *Window5h {
	return &Window5h{
		store:      store,
		max:        max,
		windows:    make(map[string]*rollingWindow),
		windowSz:   5 * time.Hour,
		lastAccess: make(map[string]time.Time),
	}
}

func NewWindow5hWithDuration(store storage.Storage, max int, windowDuration time.Duration) *Window5h {
	if windowDuration <= 0 {
		windowDuration = 5 * time.Hour
	}
	return &Window5h{
		store:      store,
		max:        max,
		windows:    make(map[string]*rollingWindow),
		windowSz:   windowDuration,
		lastAccess: make(map[string]time.Time),
	}
}

func (w *Window5h) Allow(ctx context.Context, key string) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()
	windowStart := now.Add(-w.windowSz)

	// Track last access
	w.lastAccess[key] = now

	rw, exists := w.windows[key]
	if !exists {
		rw = &rollingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
		}
		w.windows[key] = rw
	}

	w.pruneWindow(rw, windowStart)

	total := w.countInWindow(rw, windowStart, now)
	return total < w.max, nil
}

func (w *Window5h) Record(ctx context.Context, key string, delta int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()

	// Track last access
	w.lastAccess[key] = now

	rw, exists := w.windows[key]
	if !exists {
		rw = &rollingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
		}
		w.windows[key] = rw
	}

	rw.counts = append(rw.counts, delta)
	rw.timestamps = append(rw.timestamps, now)
	return w.store.IncrementRateLimit(ctx, key, domain.LimitTypeWindow5h, delta)
}

func (w *Window5h) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	now := time.Now().UTC()
	windowStart := now.Add(-w.windowSz)
	windowEnd := now

	rw, exists := w.windows[key]
	if !exists {
		return &domain.LimitState{
			Type:        domain.LimitTypeWindow5h,
			Current:     0,
			Max:         w.max,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
		}, nil
	}

	total := w.countInWindow(rw, windowStart, now)
	return &domain.LimitState{
		Type:        domain.LimitTypeWindow5h,
		Current:     total,
		Max:         w.max,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, nil
}

func (w *Window5h) Reset(ctx context.Context, key string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.windows, key)
	return w.store.ResetRateLimit(ctx, key, domain.LimitTypeWindow5h)
}

func (w *Window5h) LimitType() domain.LimitType {
	return domain.LimitTypeWindow5h
}

func (w *Window5h) countInWindow(rw *rollingWindow, windowStart, now time.Time) int {
	total := 0
	for i, ts := range rw.timestamps {
		if ts.After(windowStart) && ts.Before(now.Add(time.Nanosecond)) {
			total += rw.counts[i]
		}
	}
	return total
}

// pruneWindow 原地删除窗口起始时间之前的过期记录，避免切片无限增长
func (w *Window5h) pruneWindow(rw *rollingWindow, windowStart time.Time) {
	validIdx := 0
	for i, ts := range rw.timestamps {
		if ts.After(windowStart) {
			rw.counts[validIdx] = rw.counts[i]
			rw.timestamps[validIdx] = ts
			validIdx++
		}
	}
	rw.counts = rw.counts[:validIdx]
	rw.timestamps = rw.timestamps[:validIdx]
}

// LoadState loads persisted state from database into memory
// For sliding window, we can only restore the count, not the timestamps
func (w *Window5h) LoadState(ctx context.Context, key string, state *domain.LimitState) error {
	if state == nil || state.Current <= 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()

	// Only load state if it's from within the window
	if state.WindowStart.After(now.Add(-w.windowSz)) {
		rw := &rollingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
		}

		// Distribute the count across the window as a single entry
		// This is an approximation for sliding window
		rw.counts = append(rw.counts, state.Current)
		rw.timestamps = append(rw.timestamps, state.WindowStart.Add(w.windowSz/2))
		w.windows[key] = rw
		w.lastAccess[key] = now
	}

	return nil
}

// CleanupStale removes entries that haven't been accessed for more than maxAge
func (w *Window5h) CleanupStale(maxAge time.Duration) int {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-maxAge)
	removed := 0

	for key, lastAccess := range w.lastAccess {
		if lastAccess.Before(cutoff) {
			delete(w.windows, key)
			delete(w.lastAccess, key)
			removed++
		}
	}

	return removed
}
