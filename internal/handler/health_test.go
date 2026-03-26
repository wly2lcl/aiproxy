package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/internal/router"
)

func TestHealthHandler_Health(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)

	h.Health(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
}

func TestHealthHandler_Ready(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openai := &mockProvider{name: "openai"}
	anthropic := &mockProvider{name: "anthropic"}

	models := map[string][]string{
		"openai":    {"gpt-4o"},
		"anthropic": {"claude-3-opus"},
	}
	r := router.NewRouterWithModels([]provider.Provider{openai, anthropic}, models)

	h := NewHealthHandler(r)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "ready" {
		t.Errorf("expected status 'ready', got %v", resp["status"])
	}

	checks, ok := resp["checks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected checks to be a map")
	}

	if checks["database"] != "ok" {
		t.Errorf("expected database 'ok', got %v", checks["database"])
	}

	providers, ok := checks["providers"].([]interface{})
	if !ok {
		t.Fatal("expected providers to be a slice")
	}

	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}
