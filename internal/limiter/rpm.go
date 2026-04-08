package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

type RPM struct {
	store    storage.Storage
	max      int
	mu       sync.RWMutex
	windows  map[string]*slidingWindow
	windowSz time.Duration
	// Track last access time for cleanup
	lastAccess map[string]time.Time
}

type slidingWindow struct {
	counts     []int
	timestamps []time.Time
	startTime  time.Time
}

func NewRPM(store storage.Storage, max int) *RPM {
	return &RPM{
		store:      store,
		max:        max,
		windows:    make(map[string]*slidingWindow),
		windowSz:   time.Minute,
		lastAccess: make(map[string]time.Time),
	}
}

func (r *RPM) Allow(ctx context.Context, key string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	windowStart := now.Add(-r.windowSz)

	// Track last access
	r.lastAccess[key] = now

	sw, exists := r.windows[key]
	if !exists || sw.startTime.Before(windowStart) {
		sw = &slidingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
			startTime:  now,
		}
		r.windows[key] = sw
	}

	r.pruneWindow(sw, windowStart)

	total := r.countInWindow(sw, windowStart, now)
	return total < r.max, nil
}

func (r *RPM) Record(ctx context.Context, key string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()

	// Track last access
	r.lastAccess[key] = now

	sw, exists := r.windows[key]
	if !exists {
		sw = &slidingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
			startTime:  now,
		}
		r.windows[key] = sw
	}

	sw.counts = append(sw.counts, delta)
	sw.timestamps = append(sw.timestamps, now)
	return r.store.IncrementRateLimit(ctx, key, domain.LimitTypeRPM, delta)
}

func (r *RPM) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().UTC()
	windowStart := now.Add(-r.windowSz)
	windowEnd := now

	sw, exists := r.windows[key]
	if !exists {
		return &domain.LimitState{
			Type:        domain.LimitTypeRPM,
			Current:     0,
			Max:         r.max,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
		}, nil
	}

	total := r.countInWindow(sw, windowStart, now)
	return &domain.LimitState{
		Type:        domain.LimitTypeRPM,
		Current:     total,
		Max:         r.max,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, nil
}

func (r *RPM) Reset(ctx context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.windows, key)
	return r.store.ResetRateLimit(ctx, key, domain.LimitTypeRPM)
}

func (r *RPM) LimitType() domain.LimitType {
	return domain.LimitTypeRPM
}

func (r *RPM) countInWindow(sw *slidingWindow, windowStart, now time.Time) int {
	total := 0
	for i, ts := range sw.timestamps {
		if ts.After(windowStart) && ts.Before(now.Add(time.Nanosecond)) {
			total += sw.counts[i]
		}
	}
	return total
}

// pruneWindow 原地删除窗口起始时间之前的过期记录，避免切片无限增长
func (r *RPM) pruneWindow(sw *slidingWindow, windowStart time.Time) {
	validIdx := 0
	for i, ts := range sw.timestamps {
		if ts.After(windowStart) {
			sw.counts[validIdx] = sw.counts[i]
			sw.timestamps[validIdx] = ts
			validIdx++
		}
	}
	sw.counts = sw.counts[:validIdx]
	sw.timestamps = sw.timestamps[:validIdx]
}

// LoadState loads persisted state from database into memory
// For RPM (sliding window), we can only restore the count, not the timestamps
// This is an approximation - the actual window will converge after a few minutes
func (r *RPM) LoadState(ctx context.Context, key string, state *domain.LimitState) error {
	if state == nil || state.Current <= 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()

	// Only load state if it's from the current window
	if state.WindowStart.After(now.Add(-r.windowSz)) {
		sw := &slidingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
			startTime:  state.WindowStart,
		}

		// Distribute the count across the window as a single entry
		// This is an approximation for sliding window
		sw.counts = append(sw.counts, state.Current)
		sw.timestamps = append(sw.timestamps, state.WindowStart.Add(r.windowSz/2))
		r.windows[key] = sw
		r.lastAccess[key] = now
	}

	return nil
}

// CleanupStale removes entries that haven't been accessed for more than maxAge
func (r *RPM) CleanupStale(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-maxAge)
	removed := 0

	for key, lastAccess := range r.lastAccess {
		if lastAccess.Before(cutoff) {
			delete(r.windows, key)
			delete(r.lastAccess, key)
			removed++
		}
	}

	return removed
}
