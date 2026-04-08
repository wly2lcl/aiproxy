package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/storage"
)

const charsPerToken = 4

type Token struct {
	store      storage.Storage
	max        int
	limitType  domain.LimitType
	mu         sync.RWMutex
	usage      map[string]*tokenUsage
	windows    map[string]time.Time
	windowSz   time.Duration
	isMonthly  bool
	lastAccess map[string]time.Time
}

type tokenUsage struct {
	promptTokens     int
	completionTokens int
	estimated        bool
}

func NewTokenDaily(store storage.Storage, max int) *Token {
	return &Token{
		store:      store,
		max:        max,
		limitType:  domain.LimitTypeTokenDaily,
		usage:      make(map[string]*tokenUsage),
		windows:    make(map[string]time.Time),
		windowSz:   24 * time.Hour,
		isMonthly:  false,
		lastAccess: make(map[string]time.Time),
	}
}

func NewTokenMonthly(store storage.Storage, max int) *Token {
	return &Token{
		store:      store,
		max:        max,
		limitType:  domain.LimitTypeTokenMonthly,
		usage:      make(map[string]*tokenUsage),
		windows:    make(map[string]time.Time),
		windowSz:   0,
		isMonthly:  true,
		lastAccess: make(map[string]time.Time),
	}
}

func (t *Token) Allow(ctx context.Context, key string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)

	// Track last access
	t.lastAccess[key] = now

	currentWindow, exists := t.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		t.usage[key] = &tokenUsage{}
		t.windows[key] = windowStart
	}

	usage := t.usage[key]
	total := usage.promptTokens + usage.completionTokens
	if total >= t.max {
		return false, nil
	}

	return true, nil
}

func (t *Token) Record(ctx context.Context, key string, delta int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)

	// Track last access
	t.lastAccess[key] = now

	currentWindow, exists := t.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		t.usage[key] = &tokenUsage{}
		t.windows[key] = windowStart
	}

	usage := t.usage[key]
	usage.completionTokens += delta
	return t.store.IncrementRateLimit(ctx, key, t.limitType, delta)
}

func (t *Token) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)
	windowEnd := t.getWindowEnd(now)

	currentWindow, exists := t.windows[key]
	total := 0
	if exists && currentWindow.Equal(windowStart) {
		usage := t.usage[key]
		total = usage.promptTokens + usage.completionTokens
	}

	return &domain.LimitState{
		Type:        t.limitType,
		Current:     total,
		Max:         t.max,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, nil
}

func (t *Token) Reset(ctx context.Context, key string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.usage, key)
	delete(t.windows, key)
	return t.store.ResetRateLimit(ctx, key, t.limitType)
}

func (t *Token) LimitType() domain.LimitType {
	return t.limitType
}

func (t *Token) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

func (t *Token) RecordActual(ctx context.Context, key string, prompt, completion int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)

	currentWindow, exists := t.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		t.usage[key] = &tokenUsage{}
		t.windows[key] = windowStart
	}

	usage := t.usage[key]
	estimatedTotal := usage.promptTokens + usage.completionTokens
	actualTotal := prompt + completion
	delta := actualTotal - estimatedTotal

	usage.promptTokens = prompt
	usage.completionTokens = completion
	usage.estimated = false

	if delta > 0 {
		return t.store.IncrementRateLimit(ctx, key, t.limitType, delta)
	}
	return nil
}

func (t *Token) EstimateAndRecord(ctx context.Context, key string, promptText, completionText string) error {
	promptTokens := t.EstimateTokens(promptText)
	completionTokens := t.EstimateTokens(completionText)

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)

	currentWindow, exists := t.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		t.usage[key] = &tokenUsage{}
		t.windows[key] = windowStart
	}

	usage := t.usage[key]
	usage.promptTokens = promptTokens
	usage.completionTokens = completionTokens
	usage.estimated = true

	total := promptTokens + completionTokens
	return t.store.IncrementRateLimit(ctx, key, t.limitType, total)
}

func (t *Token) GetUsage(ctx context.Context, key string) (prompt, completion int, estimated bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)

	currentWindow, exists := t.windows[key]
	if !exists || !currentWindow.Equal(windowStart) {
		return 0, 0, false
	}

	usage := t.usage[key]
	return usage.promptTokens, usage.completionTokens, usage.estimated
}

func (t *Token) getWindowStart(now time.Time) time.Time {
	if t.isMonthly {
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (t *Token) getWindowEnd(now time.Time) time.Time {
	if t.isMonthly {
		return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
}

// LoadState loads persisted state from database into memory
func (t *Token) LoadState(ctx context.Context, key string, state *domain.LimitState) error {
	if state == nil || state.Current <= 0 {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	windowStart := t.getWindowStart(now)

	// Only load state if it's from the current window
	if state.WindowStart.Equal(windowStart) {
		t.usage[key] = &tokenUsage{
			completionTokens: state.Current,
		}
		t.windows[key] = windowStart
		t.lastAccess[key] = now
	}

	return nil
}

// CleanupStale removes entries that haven't been accessed for more than maxAge
func (t *Token) CleanupStale(maxAge time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-maxAge)
	removed := 0

	for key, lastAccess := range t.lastAccess {
		if lastAccess.Before(cutoff) {
			delete(t.usage, key)
			delete(t.windows, key)
			delete(t.lastAccess, key)
			removed++
		}
	}

	return removed
}
