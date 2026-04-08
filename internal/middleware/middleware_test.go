package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/limiter"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAuth_ValidKey(t *testing.T) {
	cfg := NewAuthConfig()
	cfg.Enabled = true
	cfg.APIKeys = map[string]bool{"test-key": true}

	router := gin.New()
	router.Use(Auth(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAuth_InvalidKey(t *testing.T) {
	cfg := NewAuthConfig()
	cfg.Enabled = true
	cfg.APIKeys = map[string]bool{"valid-key": true}

	router := gin.New()
	router.Use(Auth(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	if !strings.Contains(w.Body.String(), "invalid api key") {
		t.Errorf("expected error message about invalid api key, got %s", w.Body.String())
	}
}

func TestAuth_Disabled(t *testing.T) {
	cfg := NewAuthConfig()
	cfg.Enabled = false

	router := gin.New()
	router.Use(Auth(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAuth_MissingHeader(t *testing.T) {
	cfg := NewAuthConfig()
	cfg.Enabled = true
	cfg.APIKeys = map[string]bool{"test-key": true}

	router := gin.New()
	router.Use(Auth(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuth_InvalidFormat(t *testing.T) {
	cfg := NewAuthConfig()
	cfg.Enabled = true
	cfg.APIKeys = map[string]bool{"test-key": true}

	router := gin.New()
	router.Use(Auth(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "test-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestLogging_RequestFields(t *testing.T) {
	cfg := NewLoggingConfig()

	router := gin.New()
	router.Use(Logging(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestLogging_WithRequestBody(t *testing.T) {
	cfg := NewLoggingConfig()
	cfg.IncludeRequestBody = true

	router := gin.New()
	router.Use(Logging(cfg))
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestLogging_WithResponseBody(t *testing.T) {
	cfg := NewLoggingConfig()
	cfg.IncludeResponseBody = true

	router := gin.New()
	router.Use(Logging(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRecovery_Panic(t *testing.T) {
	router := gin.New()
	router.Use(Recovery())
	router.GET("/test", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	if !strings.Contains(w.Body.String(), "internal server error") {
		t.Errorf("expected error message, got %s", w.Body.String())
	}
}

func TestRecovery_NoPanic(t *testing.T) {
	router := gin.New()
	router.Use(Recovery())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRequestID_Incoming(t *testing.T) {
	cfg := NewRequestIDConfig()

	router := gin.New()
	router.Use(RequestID(cfg))
	router.GET("/test", func(c *gin.Context) {
		requestID, exists := c.Get("request_id")
		if !exists {
			t.Error("request_id not set in context")
		}
		if requestID != "incoming-id" {
			t.Errorf("expected request_id 'incoming-id', got '%v'", requestID)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "incoming-id")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") != "incoming-id" {
		t.Errorf("expected X-Request-ID header 'incoming-id', got '%s'", w.Header().Get("X-Request-ID"))
	}
}

func TestRequestID_Generate(t *testing.T) {
	cfg := NewRequestIDConfig()

	router := gin.New()
	router.Use(RequestID(cfg))
	router.GET("/test", func(c *gin.Context) {
		requestID, exists := c.Get("request_id")
		if !exists {
			t.Error("request_id not set in context")
		}
		idStr, ok := requestID.(string)
		if !ok {
			t.Error("request_id is not a string")
		}
		if !strings.HasPrefix(idStr, "req_") {
			t.Errorf("expected request_id to start with 'req_', got '%s'", idStr)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("expected X-Request-ID header to be set")
	}
	if !strings.HasPrefix(requestID, "req_") {
		t.Errorf("expected X-Request-ID header to start with 'req_', got '%s'", requestID)
	}
}

func TestRequestID_CustomHeader(t *testing.T) {
	cfg := NewRequestIDConfig()
	cfg.HeaderName = "X-Custom-ID"

	router := gin.New()
	router.Use(RequestID(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom-ID", "custom-id")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Header().Get("X-Custom-ID") != "custom-id" {
		t.Errorf("expected X-Custom-ID header 'custom-id', got '%s'", w.Header().Get("X-Custom-ID"))
	}
}

type mockLimiter struct {
	allow bool
	err   error
}

func (m *mockLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return m.allow, m.err
}

func (m *mockLimiter) Record(ctx context.Context, key string, delta int) error {
	return nil
}

func (m *mockLimiter) GetState(ctx context.Context, key string) (*domain.LimitState, error) {
	return &domain.LimitState{}, nil
}

func (m *mockLimiter) Reset(ctx context.Context, key string) error {
	return nil
}

func (m *mockLimiter) LimitType() domain.LimitType {
	return domain.LimitTypeRPM
}

func (m *mockLimiter) LoadState(ctx context.Context, key string, state *domain.LimitState) error {
	return nil
}

func (m *mockLimiter) CleanupStale(maxAge time.Duration) int {
	return 0
}

func TestRateLimit_Allow(t *testing.T) {
	mockLim := &mockLimiter{allow: true}
	cfg := &RateLimitConfig{
		Limiter: mockLim,
	}

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRateLimit_Exceeded(t *testing.T) {
	mockLim := &mockLimiter{allow: false}
	cfg := &RateLimitConfig{
		Limiter: mockLim,
	}

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	if !strings.Contains(w.Body.String(), "rate limit exceeded") {
		t.Errorf("expected error message about rate limit, got %s", w.Body.String())
	}
}

func TestRateLimit_NilLimiter(t *testing.T) {
	cfg := &RateLimitConfig{
		Limiter: nil,
	}

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRateLimit_CustomKeyFunc(t *testing.T) {
	mockLim := &mockLimiter{allow: true}
	cfg := &RateLimitConfig{
		Limiter: mockLim,
		KeyFunc: func(c *gin.Context) string {
			return "custom-key"
		},
	}

	router := gin.New()
	router.Use(RateLimit(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestDefaultRateLimitKeyFunc_APIKey(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("api_key", "test-api-key")

	key := DefaultRateLimitKeyFunc(c)

	if key != "test-api-key" {
		t.Errorf("expected key 'test-api-key', got '%s'", key)
	}
}

func TestDefaultRateLimitKeyFunc_ClientIP(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	c.Request = req

	key := DefaultRateLimitKeyFunc(c)

	if !strings.Contains(key, "192.168.1.1") {
		t.Errorf("expected key to contain IP address, got '%s'", key)
	}
}

func TestAuth_CustomHeaderAndPrefix(t *testing.T) {
	cfg := NewAuthConfig()
	cfg.Enabled = true
	cfg.APIKeys = map[string]bool{"test-key": true}
	cfg.HeaderName = "X-API-Key"
	cfg.KeyPrefix = "Key "

	router := gin.New()
	router.Use(Auth(cfg))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "Key test-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

var _ limiter.Limiter = (*mockLimiter)(nil)
