package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/internal/router"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

type mockProvider struct {
	name string
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

func TestModelsHandler_Handle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openai := &mockProvider{name: "openai"}
	anthropic := &mockProvider{name: "anthropic"}

	models := map[string][]string{
		"openai":    {"gpt-4o", "gpt-4o-mini"},
		"anthropic": {"claude-3-opus"},
	}
	r := router.NewRouterWithModels([]provider.Provider{openai, anthropic}, models)

	h := NewModelsHandler(r)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	h.Handle(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %s", resp.Object)
	}

	if len(resp.Data) != 3 {
		t.Errorf("expected 3 models, got %d", len(resp.Data))
	}

	expectedModels := []string{"claude-3-opus", "gpt-4o", "gpt-4o-mini"}
	for i, m := range resp.Data {
		if m.ID != expectedModels[i] {
			t.Errorf("expected model %s at index %d, got %s", expectedModels[i], i, m.ID)
		}
		if m.Object != "model" {
			t.Errorf("expected object 'model', got %s", m.Object)
		}
		if m.OwnedBy != "openai" {
			t.Errorf("expected owned_by 'openai', got %s", m.OwnedBy)
		}
		if m.Created == 0 {
			t.Error("expected created timestamp to be non-zero")
		}
	}
}

func TestModelsHandler_EmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := router.NewRouter([]provider.Provider{})
	h := NewModelsHandler(r)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	h.Handle(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %s", resp.Object)
	}

	if len(resp.Data) != 0 {
		t.Errorf("expected 0 models, got %d", len(resp.Data))
	}
}
