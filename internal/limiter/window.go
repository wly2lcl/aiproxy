package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

type Window5h struct {
	store    storage.Storage
	max      int
	mu       sync.RWMutex
	windows  map[string]*rollingWindow
	windowSz time.Duration
}

type rollingWindow struct {
	counts     []int
	timestamps []time.Time
}

func NewWindow5h(store storage.Storage, max int) *Window5h {
	return &Window5h{
		store:    store,
		max:      max,
		windows:  make(map[string]*rollingWindow),
		windowSz: 5 * time.Hour,
	}
}

func NewWindow5hWithDuration(store storage.Storage, max int, windowDuration time.Duration) *Window5h {
	if windowDuration <= 0 {
		windowDuration = 5 * time.Hour
	}
	return &Window5h{
		store:    store,
		max:      max,
		windows:  make(map[string]*rollingWindow),
		windowSz: windowDuration,
	}
}

func (w *Window5h) Allow(ctx context.Context, key string) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()
	windowStart := now.Add(-w.windowSz)

	rw, exists := w.windows[key]
	if !exists {
		rw = &rollingWindow{
			counts:     make([]int, 0),
			timestamps: make([]time.Time, 0),
		}
		w.windows[key] = rw
	}

	total := w.countInWindow(rw, windowStart, now)
	if total >= w.max {
		return false, nil
	}

	rw.counts = append(rw.counts, 1)
	rw.timestamps = append(rw.timestamps, now)
	return true, nil
}

func (w *Window5h) Record(ctx context.Context, key string, delta int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()

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
