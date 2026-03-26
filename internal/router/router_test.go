package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

type mockProvider struct {
	name   string
	models []string
}

func (m *mockProvider) Name() string                               { return m.name }
func (m *mockProvider) APIBase() string                            { return "https://api.example.com" }
func (m *mockProvider) SupportsModel(model string) bool            { return true }
func (m *mockProvider) GetHeaders(apiKey string) map[string]string { return nil }
func (m *mockProvider) GetTimeout(isStreaming bool) time.Duration  { return 30 * time.Second }
func (m *mockProvider) TransformRequest(req *openai.ChatCompletionRequest, apiKey string) (*http.Request, error) {
	return nil, nil
}
func (m *mockProvider) TransformResponse(resp *http.Response) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}

func TestRouter_ResolveByHeader(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o", "gpt-4o-mini"}}
	anthropic := &mockProvider{name: "anthropic", models: []string{"claude-3-opus", "claude-3-sonnet"}}

	models := map[string][]string{
		"openai":    {"gpt-4o", "gpt-4o-mini"},
		"anthropic": {"claude-3-opus", "claude-3-sonnet"},
	}
	r := NewRouterWithModels([]provider.Provider{openai, anthropic}, models)

	tests := []struct {
		name           string
		model          string
		providerHeader string
		wantProvider   string
		wantErr        bool
	}{
		{
			name:           "header overrides model",
			model:          "gpt-4o",
			providerHeader: "anthropic",
			wantProvider:   "anthropic",
			wantErr:        false,
		},
		{
			name:           "empty header uses model resolution",
			model:          "gpt-4o",
			providerHeader: "",
			wantProvider:   "openai",
			wantErr:        false,
		},
		{
			name:           "invalid header returns error",
			model:          "gpt-4o",
			providerHeader: "nonexistent",
			wantProvider:   "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := r.ResolveByHeader(tt.model, tt.providerHeader)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveByHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && p.Name() != tt.wantProvider {
				t.Errorf("ResolveByHeader() provider = %v, want %v", p.Name(), tt.wantProvider)
			}
		})
	}
}

func TestRouter_ResolveByPrefix(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o"}}
	anthropic := &mockProvider{name: "anthropic", models: []string{"claude-3-opus"}}

	r := NewRouter([]provider.Provider{openai, anthropic})

	tests := []struct {
		name         string
		model        string
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "openai prefix",
			model:        "openai/gpt-4o",
			wantProvider: "openai",
			wantErr:      false,
		},
		{
			name:         "anthropic prefix",
			model:        "anthropic/claude-3-opus",
			wantProvider: "anthropic",
			wantErr:      false,
		},
		{
			name:         "unknown prefix falls back to default",
			model:        "unknown/model",
			wantProvider: "openai",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := r.Resolve(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && p.Name() != tt.wantProvider {
				t.Errorf("Resolve() provider = %v, want %v", p.Name(), tt.wantProvider)
			}
		})
	}
}

func TestRouter_ResolveByMapping(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o"}}
	anthropic := &mockProvider{name: "anthropic", models: []string{"claude-3-opus"}}

	r := NewRouter([]provider.Provider{openai, anthropic})
	r.AddMapping("gpt-4", "openai:gpt-4o")
	r.AddMapping("claude", "anthropic:claude-3-opus")

	tests := []struct {
		name         string
		model        string
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "gpt-4 alias maps to openai",
			model:        "gpt-4",
			wantProvider: "openai",
			wantErr:      false,
		},
		{
			name:         "claude alias maps to anthropic",
			model:        "claude",
			wantProvider: "anthropic",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := r.Resolve(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && p.Name() != tt.wantProvider {
				t.Errorf("Resolve() provider = %v, want %v", p.Name(), tt.wantProvider)
			}
		})
	}
}

func TestRouter_ResolveDefault(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o"}}
	anthropic := &mockProvider{name: "anthropic", models: []string{"claude-3-opus"}}

	r := NewRouter([]provider.Provider{openai, anthropic})

	p, err := r.Resolve("unknown-model")
	if err != nil {
		t.Errorf("Resolve() error = %v, want nil", err)
		return
	}
	if p.Name() != "openai" {
		t.Errorf("Resolve() provider = %v, want openai (default)", p.Name())
	}
}

func TestRouter_NoProvider_Error(t *testing.T) {
	r := NewRouter([]provider.Provider{})

	_, err := r.Resolve("gpt-4o")
	if err == nil {
		t.Error("Resolve() error = nil, want ErrNoMatchingProvider")
	}
	if err != ErrNoMatchingProvider {
		t.Errorf("Resolve() error = %v, want ErrNoMatchingProvider", err)
	}
}

func TestRouter_ListModels(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o", "gpt-4o-mini"}}
	anthropic := &mockProvider{name: "anthropic", models: []string{"claude-3-opus"}}

	models := map[string][]string{
		"openai":    {"gpt-4o", "gpt-4o-mini"},
		"anthropic": {"claude-3-opus"},
	}
	r := NewRouterWithModels([]provider.Provider{openai, anthropic}, models)

	modelList := r.ListModels()
	if len(modelList) != 3 {
		t.Errorf("ListModels() returned %d models, want 3", len(modelList))
	}

	modelSet := make(map[string]bool)
	for _, m := range modelList {
		modelSet[m] = true
	}
	expected := []string{"gpt-4o", "gpt-4o-mini", "claude-3-opus"}
	for _, e := range expected {
		if !modelSet[e] {
			t.Errorf("ListModels() missing expected model %s", e)
		}
	}
}

func TestRouter_AddMapping(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o"}}
	r := NewRouter([]provider.Provider{openai})

	r.AddMapping("gpt-4", "openai:gpt-4o")

	p, err := r.Resolve("gpt-4")
	if err != nil {
		t.Errorf("Resolve() error = %v", err)
		return
	}
	if p.Name() != "openai" {
		t.Errorf("Resolve() provider = %v, want openai", p.Name())
	}
}

func TestRouter_GetProvider(t *testing.T) {
	openai := &mockProvider{name: "openai", models: []string{"gpt-4o"}}
	anthropic := &mockProvider{name: "anthropic", models: []string{"claude-3-opus"}}

	r := NewRouter([]provider.Provider{openai, anthropic})

	tests := []struct {
		name         string
		providerName string
		wantErr      bool
	}{
		{
			name:         "existing provider",
			providerName: "openai",
			wantErr:      false,
		},
		{
			name:         "another existing provider",
			providerName: "anthropic",
			wantErr:      false,
		},
		{
			name:         "nonexistent provider",
			providerName: "nonexistent",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := r.GetProvider(tt.providerName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && p.Name() != tt.providerName {
				t.Errorf("GetProvider() provider = %v, want %v", p.Name(), tt.providerName)
			}
		})
	}
}
