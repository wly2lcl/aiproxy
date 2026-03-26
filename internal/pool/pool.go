package pool

import (
	"sync"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

type AccountState struct {
	Account             *domain.Account
	ConsecutiveFailures int
	LastUsedAt          time.Time
}

type Pool struct {
	mu       sync.RWMutex
	accounts map[string]*AccountState
	order    []string
}

func NewPool(accounts []*domain.Account) *Pool {
	p := &Pool{
		accounts: make(map[string]*AccountState),
		order:    make([]string, 0),
	}
	for _, acc := range accounts {
		p.accounts[acc.ID] = &AccountState{
			Account:             acc,
			ConsecutiveFailures: 0,
			LastUsedAt:          time.Time{},
		}
		p.order = append(p.order, acc.ID)
	}
	return p
}

func (p *Pool) Add(account *domain.Account) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.accounts[account.ID]; !exists {
		p.order = append(p.order, account.ID)
	}
	p.accounts[account.ID] = &AccountState{
		Account:             account,
		ConsecutiveFailures: 0,
		LastUsedAt:          time.Time{},
	}
}

func (p *Pool) Remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.accounts[id]; exists {
		delete(p.accounts, id)
		for i, accID := range p.order {
			if accID == id {
				p.order = append(p.order[:i], p.order[i+1:]...)
				break
			}
		}
	}
}

func (p *Pool) Get(id string) (*domain.Account, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if state, exists := p.accounts[id]; exists {
		return state.Account, nil
	}
	return nil, domain.NewDomainError(domain.ErrCodeAccountNotFound, "account not found")
}

func (p *Pool) List() []*domain.Account {
	p.mu.RLock()
	defer p.mu.RUnlock()

	accounts := make([]*domain.Account, 0, len(p.accounts))
	for _, id := range p.order {
		if state, exists := p.accounts[id]; exists {
			accounts = append(accounts, state.Account)
		}
	}
	return accounts
}

func (p *Pool) SetEnabled(id string, enabled bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, exists := p.accounts[id]; exists {
		state.Account.IsEnabled = enabled
		return nil
	}
	return domain.NewDomainError(domain.ErrCodeAccountNotFound, "account not found")
}

func (p *Pool) UpdateWeight(id string, weight int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, exists := p.accounts[id]; exists {
		state.Account.Weight = weight
		return nil
	}
	return domain.NewDomainError(domain.ErrCodeAccountNotFound, "account not found")
}

func (p *Pool) RecordSuccess(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, exists := p.accounts[id]; exists {
		state.ConsecutiveFailures = 0
		state.LastUsedAt = time.Now()
	}
}

func (p *Pool) RecordFailure(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, exists := p.accounts[id]; exists {
		state.ConsecutiveFailures++
	}
}

func (p *Pool) ResetFailures(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if state, exists := p.accounts[id]; exists {
		state.ConsecutiveFailures = 0
	}
}

func (p *Pool) GetState(id string) *AccountState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if state, exists := p.accounts[id]; exists {
		return state
	}
	return nil
}

func (p *Pool) GetAvailableAccounts() []*AccountState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var available []*AccountState
	for _, id := range p.order {
		if state, exists := p.accounts[id]; exists {
			if state.Account.IsEnabled &&
				state.Account.Weight > 0 &&
				state.ConsecutiveFailures < domain.CircuitBreakerThreshold {
				available = append(available, state)
			}
		}
	}
	return available
}
