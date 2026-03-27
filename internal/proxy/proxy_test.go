package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

type mockProvider struct {
	name    string
	apiBase string
	headers map[string]string
}

func (m *mockProvider) Name() string                              { return m.name }
func (m *mockProvider) APIBase() string                           { return m.apiBase }
func (m *mockProvider) SupportsModel(model string) bool           { return true }
func (m *mockProvider) GetTimeout(isStreaming bool) time.Duration { return 30 * time.Second }
func (m *mockProvider) TransformRequest(req *openai.ChatCompletionRequest, apiKey string) (*http.Request, error) {
	return nil, nil
}
func (m *mockProvider) TransformResponse(resp *http.Response) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}
func (m *mockProvider) GetHeaders(apiKey string) map[string]string {
	headers := make(map[string]string)
	for k, v := range m.headers {
		headers[k] = v
	}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	return headers
}

func TestProxy_ServeHTTP_Success(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header 'Bearer test-key', got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer backend.Close()

	proxy := NewProxy(&Config{
		Timeout:         30 * time.Second,
		StreamTimeout:   5 * time.Minute,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	})

	req := httptest.NewRequest(http.MethodPost, backend.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), "account", &domain.Account{
		ID:         "test-account",
		ProviderID: "test-provider",
		APIKeyHash: "test-key",
	})
	ctx = context.WithValue(ctx, "provider", &mockProvider{
		name:    "test-provider",
		apiBase: backend.URL,
		headers: map[string]string{"Content-Type": "application/json"},
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	proxy.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("X-Custom-Header") != "test-value" {
		t.Errorf("expected X-Custom-Header 'test-value', got %s", resp.Header.Get("X-Custom-Header"))
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	json.Unmarshal(body, &result)
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %s", result["status"])
	}
}

func TestProxy_ServeHTTP_Error(t *testing.T) {
	proxy := NewProxy(nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	ctx := context.WithValue(req.Context(), "account", (*domain.Account)(nil))
	ctx = context.WithValue(ctx, "provider", (*mockProvider)(nil))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	proxy.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	json.Unmarshal(body, &errResp)

	if errResp.Error.Type != "proxy_error" {
		t.Errorf("expected error type 'proxy_error', got %s", errResp.Error.Type)
	}
}

func TestProxy_ServeHTTP_MissingContext(t *testing.T) {
	proxy := NewProxy(nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	proxy.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestProxy_ModifyRequest_Headers(t *testing.T) {
	proxy := NewProxy(nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	account := &domain.Account{
		ID:         "test-account",
		ProviderID: "test-provider",
		APIKeyHash: "test-api-key",
	}

	prov := &mockProvider{
		name:    "test-provider",
		headers: map[string]string{"Content-Type": "application/json", "X-Custom": "custom-value"},
	}

	err := proxy.ModifyRequest(req, account, prov)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if req.Header.Get("Authorization") != "Bearer test-api-key" {
		t.Errorf("expected Authorization 'Bearer test-api-key', got %s", req.Header.Get("Authorization"))
	}

	if req.Header.Get("X-Custom") != "custom-value" {
		t.Errorf("expected X-Custom 'custom-value', got %s", req.Header.Get("X-Custom"))
	}
}

func TestProxy_ModifyRequest_NilProvider(t *testing.T) {
	proxy := NewProxy(nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	err := proxy.ModifyRequest(req, nil, nil)
	if err == nil {
		t.Error("expected error for nil provider")
	}
}



func TestTransport_ConnectionPool(t *testing.T) {
	cfg := &Config{
		Timeout:         30 * time.Second,
		StreamTimeout:   5 * time.Minute,
		MaxIdleConns:    50,
		IdleConnTimeout: 60 * time.Second,
	}

	transport := NewTransport(cfg)

	if transport.Transport.MaxIdleConns != 50 {
		t.Errorf("expected MaxIdleConns 50, got %d", transport.Transport.MaxIdleConns)
	}

	if transport.Transport.IdleConnTimeout != 60*time.Second {
		t.Errorf("expected IdleConnTimeout 60s, got %v", transport.Transport.IdleConnTimeout)
	}

	if transport.Transport.MaxIdleConnsPerHost != 5 {
		t.Errorf("expected MaxIdleConnsPerHost 5, got %d", transport.Transport.MaxIdleConnsPerHost)
	}

	transport.CloseIdleConnections()
}

func TestTransport_ResponseHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Header", "response-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()

	transport := NewTransport(nil)

	req := httptest.NewRequest(http.MethodGet, backend.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Response-Header") != "response-value" {
		t.Errorf("expected X-Response-Header 'response-value', got %s", resp.Header.Get("X-Response-Header"))
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", resp.Header.Get("Content-Type"))
	}
}



func TestTransport_GetConfig(t *testing.T) {
	cfg := &Config{
		Timeout:         45 * time.Second,
		StreamTimeout:   3 * time.Minute,
		MaxIdleConns:    75,
		IdleConnTimeout: 80 * time.Second,
	}

	transport := NewTransport(cfg)

	returnedCfg := transport.GetConfig()
	if returnedCfg.Timeout != 45*time.Second {
		t.Errorf("expected Timeout 45s, got %v", returnedCfg.Timeout)
	}

	if returnedCfg.StreamTimeout != 3*time.Minute {
		t.Errorf("expected StreamTimeout 3m, got %v", returnedCfg.StreamTimeout)
	}

	if returnedCfg.MaxIdleConns != 75 {
		t.Errorf("expected MaxIdleConns 75, got %d", returnedCfg.MaxIdleConns)
	}
}

func TestProxy_HandleError_DomainError(t *testing.T) {
	proxy := NewProxy(nil)

	w := httptest.NewRecorder()
	domainErr := domain.NewDomainError(domain.ErrCodeRateLimitExceeded, "rate limit exceeded")

	proxy.HandleError(w, domainErr)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	json.Unmarshal(body, &errResp)

	if errResp.Error.Code != domain.ErrCodeRateLimitExceeded {
		t.Errorf("expected error code '%s', got %s", domain.ErrCodeRateLimitExceeded, errResp.Error.Code)
	}

	if errResp.Error.Type != "domain_error" {
		t.Errorf("expected error type 'domain_error', got %s", errResp.Error.Type)
	}
}

func TestProxy_GetTransport(t *testing.T) {
	proxy := NewProxy(nil)

	transport := proxy.GetTransport()
	if transport == nil {
		t.Error("expected transport to not be nil")
	}
}

func TestNewProxy_DefaultConfig(t *testing.T) {
	proxy := NewProxy(nil)

	if proxy.config.Timeout != 30*time.Second {
		t.Errorf("expected default Timeout 30s, got %v", proxy.config.Timeout)
	}

	if proxy.config.StreamTimeout != 5*time.Minute {
		t.Errorf("expected default StreamTimeout 5m, got %v", proxy.config.StreamTimeout)
	}

	if proxy.config.MaxIdleConns != 100 {
		t.Errorf("expected default MaxIdleConns 100, got %d", proxy.config.MaxIdleConns)
	}
}

func TestNewProxy_CustomConfig(t *testing.T) {
	cfg := &Config{
		Timeout:         60 * time.Second,
		StreamTimeout:   10 * time.Minute,
		MaxIdleConns:    200,
		IdleConnTimeout: 120 * time.Second,
	}

	proxy := NewProxy(cfg)

	if proxy.config.Timeout != 60*time.Second {
		t.Errorf("expected Timeout 60s, got %v", proxy.config.Timeout)
	}

	if proxy.config.StreamTimeout != 10*time.Minute {
		t.Errorf("expected StreamTimeout 10m, got %v", proxy.config.StreamTimeout)
	}

	if proxy.config.MaxIdleConns != 200 {
		t.Errorf("expected MaxIdleConns 200, got %d", proxy.config.MaxIdleConns)
	}
}
