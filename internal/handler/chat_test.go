package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/limiter"
	"github.com/wangluyao/aiproxy/internal/pool"
	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/internal/proxy"
	"github.com/wangluyao/aiproxy/internal/router"
	"github.com/wangluyao/aiproxy/internal/stats"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

type testProvider struct {
	name    string
	apiBase string
	models  []string
	headers map[string]string
}

func (m *testProvider) Name() string                              { return m.name }
func (m *testProvider) APIBase() string                           { return m.apiBase }
func (m *testProvider) SupportsModel(model string) bool           { return true }
func (m *testProvider) GetTimeout(isStreaming bool) time.Duration { return 30 * time.Second }
func (m *testProvider) TransformRequest(req *openai.ChatCompletionRequest, apiKey string) (*http.Request, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, m.apiBase+"/v1/chat/completions", bytes.NewReader(body))
	return httpReq, nil
}
func (m *testProvider) TransformResponse(resp *http.Response) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}
func (m *testProvider) GetHeaders(apiKey string) map[string]string {
	headers := make(map[string]string)
	for k, v := range m.headers {
		headers[k] = v
	}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	return headers
}

type mockLogger struct {
	mu     sync.RWMutex
	infos  []string
	errors []string
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		infos:  make([]string, 0),
		errors: make([]string, 0),
	}
}

func (l *mockLogger) Info(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, msg)
}

func (l *mockLogger) Error(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, msg)
}

type mockLimiter struct {
	mu          sync.RWMutex
	allowed     map[string]bool
	allowErrors map[string]error
	recordCalls map[string]int
}

func newMockLimiter() *mockLimiter {
	return &mockLimiter{
		allowed:     make(map[string]bool),
		allowErrors: make(map[string]error),
		recordCalls: make(map[string]int),
	}
}

func (m *mockLimiter) setAllowed(key string, allowed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowed[key] = allowed
}

func (m *mockLimiter) Allow(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err, ok := m.allowErrors[key]; ok {
		return false, err
	}
	if allowed, ok := m.allowed[key]; ok {
		return allowed, nil
	}
	return true, nil
}

func (m *mockLimiter) Record(ctx context.Context, key string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCalls[key]++
	return nil
}

func (m *mockLimiter) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	return &domain.LimitState{Type: domain.LimitTypeRPM, Current: 0, Max: 100}, nil
}

func (m *mockLimiter) Reset(ctx context.Context, key string) error {
	return nil
}

func (m *mockLimiter) LimitType() domain.LimitType {
	return domain.LimitTypeRPM
}

func createTestAccount(id string, weight int, enabled bool) *domain.Account {
	return &domain.Account{
		ID:         id,
		ProviderID: "test-provider",
		APIKeyHash: "test-key-" + id,
		Weight:     weight,
		Priority:   0,
		IsEnabled:  enabled,
	}
}

func setupTestHandler(backend *httptest.Server, accounts []*domain.Account, ml *mockLimiter) (*ChatHandler, *gin.Engine) {
	gin.SetMode(gin.TestMode)

	p := pool.NewPool(accounts)

	prov := &testProvider{
		name:    "test-provider",
		apiBase: backend.URL,
		headers: map[string]string{"Content-Type": "application/json"},
	}

	r := router.NewRouter([]provider.Provider{prov})

	var limiters map[string]*limiter.CompositeLimiter
	if ml != nil {
		limiters = make(map[string]*limiter.CompositeLimiter)
		for _, acc := range accounts {
			limiters[acc.ID] = limiter.NewCompositeLimiter(ml)
		}
	}

	proxyCfg := &proxy.Config{
		Timeout:         30 * time.Second,
		StreamTimeout:   5 * time.Minute,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	px := proxy.NewProxy(proxyCfg)

	collector := stats.NewCollector()
	logger := newMockLogger()

	cfg := &ChatConfig{
		Pool:      p,
		Router:    r,
		Proxy:     px,
		Limiters:  limiters,
		Collector: collector,
		Logger:    logger,
	}

	handler, err := NewChatHandler(cfg)
	if err != nil {
		panic("failed to create handler: " + err.Error())
	}

	engine := gin.New()
	engine.POST("/v1/chat/completions", handler.Handle)

	return handler, engine
}

func TestChatHandler_Handle_Success(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage: &openai.Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()
	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var chatResp openai.ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if chatResp.ID != "chatcmpl-123" {
		t.Errorf("expected ID chatcmpl-123, got %s", chatResp.ID)
	}

	if chatResp.Usage == nil || chatResp.Usage.TotalTokens != 15 {
		t.Errorf("expected usage with 15 tokens, got %v", chatResp.Usage)
	}
}

func TestChatHandler_Handle_Streaming(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\n",
			"data: [DONE]\n\n",
		}

		for _, chunk := range chunks {
			w.Write([]byte(chunk))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()
	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", cc)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("expected stream body to be non-empty")
	}
}

func TestChatHandler_Handle_RateLimited(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	accounts := []*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	}
	ml := newMockLimiter()
	ml.setAllowed("acc1", false)
	ml.setAllowed("acc2", false)

	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 (all accounts rate-limited), got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}

	if errResp.Error.Code != domain.ErrCodeNoAvailableAccount {
		t.Errorf("expected error code %s, got %s", domain.ErrCodeNoAvailableAccount, errResp.Error.Code)
	}
}

func TestChatHandler_Handle_NoAccount(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	accounts := []*domain.Account{}
	ml := newMockLimiter()

	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}

	if errResp.Error.Code != domain.ErrCodeNoAvailableAccount {
		t.Errorf("expected error code %s, got %s", domain.ErrCodeNoAvailableAccount, errResp.Error.Code)
	}
}

func TestChatHandler_Handle_InvalidRequest(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()

	_, engine := setupTestHandler(backend, accounts, ml)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}

	if errResp.Error.Code != "invalid_request" {
		t.Errorf("expected error code invalid_request, got %s", errResp.Error.Code)
	}
}

func TestChatHandler_Handle_UpstreamError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errResp := openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "model not found",
				Type:    "invalid_request_error",
				Code:    "model_not_found",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(errResp)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()

	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}

	if errResp.Error.Code != "model_not_found" {
		t.Errorf("expected error code model_not_found, got %s", errResp.Error.Code)
	}
}

func TestChatHandler_RecordUsage(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage: &openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()
	handler, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	snapshot := handler.collector.GetSnapshot()
	if snapshot.TotalTokens != 150 {
		t.Errorf("expected total tokens 150, got %d", snapshot.TotalTokens)
	}

	tokensByModel := snapshot.TokensByModel()
	if tokensByModel["gpt-4"] != 150 {
		t.Errorf("expected 150 tokens for gpt-4, got %d", tokensByModel["gpt-4"])
	}
}

func TestChatHandler_Handle_MissingModel(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()

	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp openai.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}

	if errResp.Error.Code != "invalid_request" {
		t.Errorf("expected error code invalid_request, got %s", errResp.Error.Code)
	}
}

func TestChatHandler_Handle_DisabledAccount(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	accounts := []*domain.Account{
		createTestAccount("acc1", 1, false),
	}
	ml := newMockLimiter()

	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 for disabled account, got %d", resp.StatusCode)
	}
}

func TestChatHandler_Handle_MultipleAccounts(t *testing.T) {
	callCount := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage: &openai.Usage{TotalTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	accounts := []*domain.Account{
		createTestAccount("acc1", 1, true),
		createTestAccount("acc2", 1, true),
	}
	ml := newMockLimiter()

	_, engine := setupTestHandler(backend, accounts, ml)

	for i := 0; i < 10; i++ {
		reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d failed", i)
		}
	}

	if callCount != 10 {
		t.Errorf("expected 10 calls, got %d", callCount)
	}
}

func TestNewChatHandler_NilConfig(t *testing.T) {
	handler, err := NewChatHandler(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
	if handler != nil {
		t.Error("expected nil handler for nil config")
	}
}

func TestNewChatHandler_WithNilLogger(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()

	gin.SetMode(gin.TestMode)
	p := pool.NewPool(accounts)
	prov := &testProvider{name: "test-provider", apiBase: backend.URL}
	r := router.NewRouter([]provider.Provider{prov})
	px := proxy.NewProxy(nil)
	collector := stats.NewCollector()

	limiters := make(map[string]*limiter.CompositeLimiter)
	for _, acc := range accounts {
		limiters[acc.ID] = limiter.NewCompositeLimiter(ml)
	}

	cfg := &ChatConfig{
		Pool:      p,
		Router:    r,
		Proxy:     px,
		Limiters:  limiters,
		Collector: collector,
		Logger:    nil,
	}

	handler, err := NewChatHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	if handler == nil {
		t.Error("expected handler to be created")
	}
}

func TestChatHandler_Handle_ProviderHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{Index: 0, Message: openai.ChatMessage{Role: "assistant", Content: "Hello!"}, FinishReason: "stop"},
			},
			Usage: &openai.Usage{TotalTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer backend.Close()

	accounts := []*domain.Account{createTestAccount("acc1", 1, true)}
	ml := newMockLimiter()
	_, engine := setupTestHandler(backend, accounts, ml)

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Provider", "test-provider")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
