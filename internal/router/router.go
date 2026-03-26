package router

import (
	"errors"
	"strings"
	"sync"

	"github.com/wangluyao/aiproxy/internal/provider"
)

var (
	ErrProviderNotFound   = errors.New("provider not found")
	ErrNoMatchingProvider = errors.New("no provider supports the specified model")
)

type Router struct {
	mu              sync.RWMutex
	providers       map[string]provider.Provider
	providerOrder   []string
	providerModels  map[string][]string
	defaultProvider string
	mappings        map[string]string
}

func NewRouter(providers []provider.Provider) *Router {
	r := &Router{
		providers:      make(map[string]provider.Provider),
		providerModels: make(map[string][]string),
		mappings:       make(map[string]string),
	}
	for _, p := range providers {
		name := p.Name()
		r.providers[name] = p
		r.providerOrder = append(r.providerOrder, name)
	}
	if len(r.providerOrder) > 0 {
		r.defaultProvider = r.providerOrder[0]
	}
	return r
}

func NewRouterWithModels(providers []provider.Provider, models map[string][]string) *Router {
	r := NewRouter(providers)
	r.mu.Lock()
	for name, m := range models {
		r.providerModels[name] = m
	}
	r.mu.Unlock()
	return r
}

func (r *Router) Resolve(model string) (provider.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if target, ok := r.mappings[model]; ok {
		parts := strings.SplitN(target, ":", 2)
		if len(parts) == 2 {
			providerName := parts[0]
			if p, exists := r.providers[providerName]; exists {
				return p, nil
			}
		}
	}

	if strings.Contains(model, "/") {
		parts := strings.SplitN(model, "/", 2)
		providerName := parts[0]
		if p, exists := r.providers[providerName]; exists {
			return p, nil
		}
	}

	if r.defaultProvider != "" {
		if p, exists := r.providers[r.defaultProvider]; exists {
			return p, nil
		}
	}

	for _, name := range r.providerOrder {
		if p, exists := r.providers[name]; exists {
			if p.SupportsModel(model) {
				return p, nil
			}
		}
	}

	return nil, ErrNoMatchingProvider
}

func (r *Router) ResolveByHeader(model, providerHeader string) (provider.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if providerHeader != "" {
		if p, exists := r.providers[providerHeader]; exists {
			return p, nil
		}
		return nil, ErrProviderNotFound
	}

	return r.Resolve(model)
}

func (r *Router) AddMapping(alias, target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappings[alias] = target
}

func (r *Router) GetMappedModel(model string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if target, ok := r.mappings[model]; ok {
		parts := strings.SplitN(target, ":", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return target
	}
	return model
}

func (r *Router) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	modelSet := make(map[string]struct{})
	for _, models := range r.providerModels {
		for _, m := range models {
			modelSet[m] = struct{}{}
		}
	}

	models := make([]string, 0, len(modelSet))
	for m := range modelSet {
		models = append(models, m)
	}
	return models
}

func (r *Router) GetProvider(name string) (provider.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if p, exists := r.providers[name]; exists {
		return p, nil
	}
	return nil, ErrProviderNotFound
}

func (r *Router) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; !exists {
		return ErrProviderNotFound
	}
	r.defaultProvider = name
	return nil
}

func (r *Router) AddProvider(p provider.Provider, models []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	r.providers[name] = p
	r.providerModels[name] = models

	found := false
	for _, n := range r.providerOrder {
		if n == name {
			found = true
			break
		}
	}
	if !found {
		r.providerOrder = append(r.providerOrder, name)
	}

	if r.defaultProvider == "" {
		r.defaultProvider = name
	}
}
