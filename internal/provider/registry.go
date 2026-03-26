package provider

import (
	"errors"
	"sync"

	"github.com/wangluyao/aiproxy/internal/domain"
)

var (
	ErrProviderNotFound   = errors.New("provider not found")
	ErrProviderExists     = errors.New("provider already exists")
	ErrNoDefaultProvider  = errors.New("no default provider set")
	ErrNoMatchingProvider = errors.New("no provider supports the specified model")
)

type Registry struct {
	mu              sync.RWMutex
	providers       map[string]Provider
	defaultProvider string
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := provider.Name()
	if _, exists := r.providers[name]; exists {
		return ErrProviderExists
	}

	r.providers[name] = provider

	if len(r.providers) == 1 {
		r.defaultProvider = name
	}

	return nil
}

func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, ErrProviderNotFound
	}

	return provider, nil
}

func (r *Registry) GetByModel(model string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, provider := range r.providers {
		if provider.SupportsModel(model) {
			return provider, nil
		}
	}

	return nil, ErrNoMatchingProvider
}

func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}

	return providers
}

func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; !exists {
		return ErrProviderNotFound
	}

	r.defaultProvider = name
	return nil
}

func (r *Registry) GetDefault() Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultProvider == "" {
		return nil
	}

	return r.providers[r.defaultProvider]
}

func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; !exists {
		return ErrProviderNotFound
	}

	delete(r.providers, name)

	if r.defaultProvider == name {
		r.defaultProvider = ""
		for name := range r.providers {
			r.defaultProvider = name
			break
		}
	}

	return nil
}

func NewRegistryFromConfig(configs []domain.Provider, apiKeys map[string][]string) (*Registry, error) {
	registry := NewRegistry()

	for _, cfg := range configs {
		if !cfg.IsEnabled {
			continue
		}

		keys, ok := apiKeys[cfg.ID]
		if !ok || len(keys) == 0 {
			continue
		}

		var provider Provider
		switch cfg.ID {
		case "openai":
			provider = NewOpenAIProvider(keys[0], nil, cfg.Models)
		case "openrouter":
			provider = NewOpenRouterProvider(keys[0], nil, cfg.Models, cfg.Headers)
		case "groq":
			provider = NewGroqProvider(keys[0], nil, cfg.Models)
		default:
			p := NewOpenAIProvider(keys[0], nil, cfg.Models)
			p.name = cfg.ID
			p.apiBase = cfg.APIBase
			provider = p
		}

		if err := registry.Register(provider); err != nil {
			return nil, err
		}

		if cfg.IsDefault {
			registry.SetDefault(cfg.ID)
		}
	}

	return registry, nil
}
