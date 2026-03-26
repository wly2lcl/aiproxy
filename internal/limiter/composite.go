package limiter

import (
	"context"
	"sync"

	"github.com/wangluyao/aiproxy/internal/domain"
)

type CompositeLimiter struct {
	limiters []Limiter
	mu       sync.RWMutex
}

func NewCompositeLimiter(limiters ...Limiter) *CompositeLimiter {
	return &CompositeLimiter{
		limiters: limiters,
	}
}

func (c *CompositeLimiter) Allow(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, l := range c.limiters {
		allowed, err := l.Allow(ctx, key)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

func (c *CompositeLimiter) Record(ctx context.Context, key string, delta int) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, l := range c.limiters {
		if err := l.Record(ctx, key, delta); err != nil {
			return err
		}
	}
	return nil
}

func (c *CompositeLimiter) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var combined domain.LimitState
	combined.Current = 0
	combined.Max = 0

	for _, l := range c.limiters {
		state, err := l.GetState(ctx, key)
		if err != nil {
			return nil, err
		}
		if state != nil {
			if combined.Type == "" {
				combined.Type = state.Type
				combined.WindowStart = state.WindowStart
				combined.WindowEnd = state.WindowEnd
			}
			if state.Current > combined.Current {
				combined.Current = state.Current
			}
			if combined.Max == 0 || state.Max < combined.Max {
				combined.Max = state.Max
			}
		}
	}

	if combined.Type == "" {
		combined.Type = domain.LimitTypeRPM
	}

	return &combined, nil
}

func (c *CompositeLimiter) Reset(ctx context.Context, key string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, l := range c.limiters {
		if err := l.Reset(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (c *CompositeLimiter) LimitType() domain.LimitType {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.limiters) == 0 {
		return domain.LimitTypeRPM
	}
	return c.limiters[0].LimitType()
}

func (c *CompositeLimiter) GetStates(ctx context.Context, key string) (map[domain.LimitType]*domain.LimitState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	states := make(map[domain.LimitType]*domain.LimitState)
	for _, l := range c.limiters {
		state, err := l.GetState(ctx, key)
		if err != nil {
			return nil, err
		}
		states[l.LimitType()] = state
	}
	return states, nil
}

func (c *CompositeLimiter) AddLimiter(l Limiter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limiters = append(c.limiters, l)
}

func (c *CompositeLimiter) GetLimiters() []Limiter {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]Limiter, len(c.limiters))
	copy(result, c.limiters)
	return result
}
