package pool

import (
	"context"
	"errors"
	"sync"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/limiter"
)

var ErrNoAvailableAccount = errors.New("no available account")

type Selector interface {
	Select(ctx context.Context, limits []domain.LimitType) (*domain.Account, error)
}

type weightedAccount struct {
	state   *AccountState
	weight  int
	current int
}

type WeightedRoundRobin struct {
	pool     *Pool
	limiters map[string]*limiter.CompositeLimiter
	mu       sync.Mutex
	weights  map[string]*weightedAccount
}

func NewWeightedRoundRobin(pool *Pool, limiters map[string]*limiter.CompositeLimiter) *WeightedRoundRobin {
	return &WeightedRoundRobin{
		pool:     pool,
		limiters: limiters,
		weights:  make(map[string]*weightedAccount),
	}
}

func (w *WeightedRoundRobin) Select(ctx context.Context, limits []domain.LimitType) (*domain.Account, error) {
	available := w.pool.GetAvailableAccounts()
	if len(available) == 0 {
		return nil, ErrNoAvailableAccount
	}

	// Filter by rate limiter
	var eligible []*AccountState
	for _, state := range available {
		if w.limiters != nil {
			if limiter, ok := w.limiters[state.Account.ID]; ok && limiter != nil {
				allowed, err := limiter.Allow(ctx, state.Account.ID)
				if err != nil || !allowed {
					continue
				}
			}
		}
		eligible = append(eligible, state)
	}

	if len(eligible) == 0 {
		return nil, ErrNoAvailableAccount
	}

	// Find highest priority among eligible accounts
	maxPriority := eligible[0].Account.Priority
	for _, state := range eligible {
		if state.Account.Priority > maxPriority {
			maxPriority = state.Account.Priority
		}
	}

	// Filter to only highest priority accounts
	var highestPriority []*AccountState
	for _, state := range eligible {
		if state.Account.Priority == maxPriority {
			highestPriority = append(highestPriority, state)
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	return w.selectByWeight(highestPriority), nil
}

func (w *WeightedRoundRobin) selectByWeight(accounts []*AccountState) *domain.Account {
	totalWeight := 0
	var selected *weightedAccount

	for _, state := range accounts {
		id := state.Account.ID
		weight := state.Account.Weight
		totalWeight += weight

		if _, exists := w.weights[id]; !exists {
			w.weights[id] = &weightedAccount{
				state:   state,
				weight:  weight,
				current: 0,
			}
		} else {
			w.weights[id].state = state
			w.weights[id].weight = weight
		}
		w.weights[id].current += weight

		if selected == nil || w.weights[id].current > selected.current {
			selected = w.weights[id]
		}
	}

	if selected == nil {
		return accounts[0].Account
	}

	selected.current -= totalWeight
	return selected.state.Account
}

func (w *WeightedRoundRobin) ResetIndex() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.weights = make(map[string]*weightedAccount)
}
