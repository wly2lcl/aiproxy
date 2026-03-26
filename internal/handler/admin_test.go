package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/pool"
	"github.com/wangluyao/aiproxy/internal/stats"
	"github.com/wangluyao/aiproxy/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAdminHandler_Auth(t *testing.T) {
	tests := []struct {
		name       string
		apiKeys    map[string]bool
		headerKey  string
		wantStatus int
	}{
		{
			name:       "no auth required when no api keys",
			apiKeys:    nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing admin key",
			apiKeys:    map[string]bool{"secret": true},
			headerKey:  "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid admin key",
			apiKeys:    map[string]bool{"secret": true},
			headerKey:  "wrong",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid admin key",
			apiKeys:    map[string]bool{"secret": true},
			headerKey:  "secret",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()

			handler := NewAdminHandler(&AdminConfig{
				Storage: db,
				APIKeys: tt.apiKeys,
			})

			router := gin.New()
			router.GET("/test", handler.Auth(), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.headerKey != "" {
				req.Header.Set("X-Admin-Key", tt.headerKey)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestAdminHandler_ListAccounts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProviderAndAccount(t, db)

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest("GET", "/admin/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var accounts []AccountResponse
	if err := json.Unmarshal(w.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(accounts) != 1 {
		t.Errorf("expected 1 account, got %d", len(accounts))
		return
	}

	if accounts[0].ID != "test-account" {
		t.Errorf("expected account id 'test-account', got %s", accounts[0].ID)
	}
	if accounts[0].Provider != "test-provider" {
		t.Errorf("expected provider 'test-provider', got %s", accounts[0].Provider)
	}
	if !accounts[0].Enabled {
		t.Error("expected account to be enabled")
	}
}

func TestAdminHandler_GetAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProviderAndAccount(t, db)

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	t.Run("existing account", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/accounts/test-account", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response["id"] != "test-account" {
			t.Errorf("expected id 'test-account', got %v", response["id"])
		}
	})

	t.Run("nonexistent account", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/accounts/nonexistent", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestAdminHandler_AddAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProvider(t, db)

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	t.Run("valid account", func(t *testing.T) {
		body := AddAccountRequest{
			ID:         "new-account",
			ProviderID: "test-provider",
			APIKey:     "sk-test-key",
			Weight:     2,
			Priority:   1,
			Enabled:    true,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/accounts", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var response AccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response.ID != "new-account" {
			t.Errorf("expected id 'new-account', got %s", response.ID)
		}
	})

	t.Run("missing provider_id", func(t *testing.T) {
		body := AddAccountRequest{
			ID:      "no-provider",
			APIKey:  "sk-test-key",
			Enabled: true,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/accounts", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("missing api_key", func(t *testing.T) {
		body := AddAccountRequest{
			ID:         "no-key",
			ProviderID: "test-provider",
			Enabled:    true,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/accounts", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("nonexistent provider", func(t *testing.T) {
		body := AddAccountRequest{
			ID:         "bad-provider",
			ProviderID: "nonexistent",
			APIKey:     "sk-test-key",
			Enabled:    true,
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/accounts", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestAdminHandler_UpdateAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProviderAndAccount(t, db)

	testPool := pool.NewPool([]*domain.Account{
		{
			ID:         "test-account",
			ProviderID: "test-provider",
			APIKeyHash: "hash123",
			Weight:     1,
			Priority:   0,
			IsEnabled:  true,
		},
	})

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
		Pool:    testPool,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	t.Run("update weight and priority", func(t *testing.T) {
		body := UpdateAccountRequest{
			Weight:   ptr(5),
			Priority: ptr(2),
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("PUT", "/admin/accounts/test-account", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var response AccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response.Weight != 5 {
			t.Errorf("expected weight 5, got %d", response.Weight)
		}
		if response.Priority != 2 {
			t.Errorf("expected priority 2, got %d", response.Priority)
		}
	})

	t.Run("disable account", func(t *testing.T) {
		body := UpdateAccountRequest{
			Enabled: ptr(false),
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("PUT", "/admin/accounts/test-account", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var response AccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response.Enabled {
			t.Error("expected account to be disabled")
		}
	})

	t.Run("nonexistent account", func(t *testing.T) {
		body := UpdateAccountRequest{
			Weight: ptr(5),
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("PUT", "/admin/accounts/nonexistent", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestAdminHandler_DeleteAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProviderAndAccount(t, db)

	testPool := pool.NewPool([]*domain.Account{
		{
			ID:         "test-account",
			ProviderID: "test-provider",
			APIKeyHash: "hash123",
			Weight:     1,
			Priority:   0,
			IsEnabled:  true,
		},
	})

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
		Pool:    testPool,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest("DELETE", "/admin/accounts/test-account", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["message"] != "account disabled" {
		t.Errorf("expected message 'account disabled', got %s", response["message"])
	}
}

func TestAdminHandler_ResetLimits(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProviderAndAccount(t, db)

	testPool := pool.NewPool([]*domain.Account{
		{
			ID:         "test-account",
			ProviderID: "test-provider",
			APIKeyHash: "hash123",
			Weight:     1,
			Priority:   0,
			IsEnabled:  true,
		},
	})

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
		Pool:    testPool,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest("POST", "/admin/accounts/test-account/reset", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["message"] != "limits reset" {
		t.Errorf("expected message 'limits reset', got %s", response["message"])
	}
}

func TestAdminHandler_GetStats(t *testing.T) {
	t.Run("with collector", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		collector := stats.NewCollector()
		collector.RecordRequest("openai", "gpt-4", 200, 10000000, 100)
		collector.RecordRequest("openai", "gpt-4", 200, 15000000, 150)
		collector.RecordRequest("anthropic", "claude-3", 500, 5000000, 50)

		handler := NewAdminHandler(&AdminConfig{
			Storage:   db,
			Collector: collector,
		})

		router := gin.New()
		handler.RegisterRoutes(router.Group(""))

		req := httptest.NewRequest("GET", "/admin/stats", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var response StatsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response.TotalRequests != 3 {
			t.Errorf("expected 3 total requests, got %d", response.TotalRequests)
		}
		if response.TotalTokens != 300 {
			t.Errorf("expected 300 total tokens, got %d", response.TotalTokens)
		}
		if response.TotalErrors != 1 {
			t.Errorf("expected 1 total error, got %d", response.TotalErrors)
		}
		if response.ByProvider["openai"] != 2 {
			t.Errorf("expected 2 requests for openai, got %d", response.ByProvider["openai"])
		}
		if response.ByModel["gpt-4"] != 2 {
			t.Errorf("expected 2 requests for gpt-4, got %d", response.ByModel["gpt-4"])
		}
	})

	t.Run("without collector", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		handler := NewAdminHandler(&AdminConfig{
			Storage: db,
		})

		router := gin.New()
		handler.RegisterRoutes(router.Group(""))

		req := httptest.NewRequest("GET", "/admin/stats", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

func TestAdminHandler_ListProviders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	setupTestProvider(t, db)

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest("GET", "/admin/providers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var providers []ProviderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &providers); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(providers))
		return
	}

	if providers[0].ID != "test-provider" {
		t.Errorf("expected provider id 'test-provider', got %s", providers[0].ID)
	}
	if providers[0].APIBase != "https://api.example.com" {
		t.Errorf("expected api base 'https://api.example.com', got %s", providers[0].APIBase)
	}
}

func TestAdminHandler_ReloadConfig(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("with config loader", func(t *testing.T) {
		reloadCalled := false
		handler := NewAdminHandler(&AdminConfig{
			Storage: db,
			ConfigLoader: func() error {
				reloadCalled = true
				return nil
			},
		})

		router := gin.New()
		handler.RegisterRoutes(router.Group(""))

		req := httptest.NewRequest("POST", "/admin/reload", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		if !reloadCalled {
			t.Error("expected config loader to be called")
		}
	})

	t.Run("without config loader", func(t *testing.T) {
		handler := NewAdminHandler(&AdminConfig{
			Storage: db,
		})

		router := gin.New()
		handler.RegisterRoutes(router.Group(""))

		req := httptest.NewRequest("POST", "/admin/reload", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestAdminHandler_Health(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		setupTestProvider(t, db)

		testPool := pool.NewPool([]*domain.Account{
			{
				ID:         "test-account",
				ProviderID: "test-provider",
				APIKeyHash: "hash123",
				Weight:     1,
				Priority:   0,
				IsEnabled:  true,
			},
		})

		collector := stats.NewCollector()

		handler := NewAdminHandler(&AdminConfig{
			Storage:   db,
			Pool:      testPool,
			Collector: collector,
		})

		router := gin.New()
		handler.RegisterRoutes(router.Group(""))

		req := httptest.NewRequest("GET", "/admin/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
		}

		var response HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response.Status != "healthy" {
			t.Errorf("expected status 'healthy', got %s", response.Status)
		}
		if response.Checks["storage"] != "healthy" {
			t.Errorf("expected storage check 'healthy', got %s", response.Checks["storage"])
		}
	})

	t.Run("no available accounts", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		testPool := pool.NewPool([]*domain.Account{})

		handler := NewAdminHandler(&AdminConfig{
			Storage: db,
			Pool:    testPool,
		})

		router := gin.New()
		handler.RegisterRoutes(router.Group(""))

		req := httptest.NewRequest("GET", "/admin/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var response HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if response.Checks["pool"] != "no available accounts" {
			t.Errorf("expected pool check 'no available accounts', got %s", response.Checks["pool"])
		}
	})
}

func TestAdminHandler_WithAuth(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := NewAdminHandler(&AdminConfig{
		Storage: db,
		APIKeys: map[string]bool{"secret-key": true},
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group(""))

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/accounts", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})

	t.Run("invalid auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/accounts", nil)
		req.Header.Set("X-Admin-Key", "wrong-key")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})

	t.Run("valid auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/accounts", nil)
		req.Header.Set("X-Admin-Key", "secret-key")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

func setupTestDB(t *testing.T) *storage.SQLite {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := storage.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}

	return db
}

func setupTestProvider(t *testing.T, db *storage.SQLite) {
	t.Helper()
	ctx := t.Context()

	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}
	if err := db.UpsertProvider(ctx, provider); err != nil {
		t.Fatalf("UpsertProvider failed: %v", err)
	}
}

func setupTestProviderAndAccount(t *testing.T, db *storage.SQLite) {
	t.Helper()
	ctx := t.Context()

	setupTestProvider(t, db)

	account := &domain.Account{
		ID:         "test-account",
		ProviderID: "test-provider",
		APIKeyHash: "hash123",
		Weight:     1,
		Priority:   0,
		IsEnabled:  true,
	}
	if err := db.UpsertAccount(ctx, account); err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}
}

func ptr[T any](v T) *T {
	return &v
}
