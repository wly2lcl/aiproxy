package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/pool"
	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/internal/stats"
	"github.com/wangluyao/aiproxy/internal/storage"
	"github.com/wangluyao/aiproxy/pkg/utils"
)

type AdminConfig struct {
	Storage          storage.Storage
	Collector        *stats.Collector
	Pool             *pool.Pool
	ProviderRegistry *provider.Registry
	APIKeys          map[string]bool
	ConfigPath       string
	ConfigLoader     func() error
}

type AdminHandler struct {
	storage          storage.Storage
	collector        *stats.Collector
	pool             *pool.Pool
	providerRegistry *provider.Registry
	apiKeys          map[string]bool
	configPath       string
	configLoader     func() error
}

func NewAdminHandler(cfg *AdminConfig) *AdminHandler {
	return &AdminHandler{
		storage:          cfg.Storage,
		collector:        cfg.Collector,
		pool:             cfg.Pool,
		providerRegistry: cfg.ProviderRegistry,
		apiKeys:          cfg.APIKeys,
		configPath:       cfg.ConfigPath,
		configLoader:     cfg.ConfigLoader,
	}
}

func (h *AdminHandler) Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(h.apiKeys) == 0 {
			c.Next()
			return
		}

		key := c.GetHeader("X-Admin-Key")
		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing admin key"})
			c.Abort()
			return
		}

		if !h.validateAdminKey(key) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (h *AdminHandler) validateAdminKey(key string) bool {
	for storedKey := range h.apiKeys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) == 1 {
			return true
		}
	}
	return false
}

type AccountResponse struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider"`
	Enabled    bool           `json:"enabled"`
	Weight     int            `json:"weight"`
	Priority   int            `json:"priority"`
	APIKey     string         `json:"api_key,omitempty"`
	Limits     map[string]int `json:"limits"`
	Available  bool           `json:"available"`
	LastUsedAt *time.Time     `json:"last_used_at,omitempty"`
}

type AddAccountRequest struct {
	ID         string         `json:"id"`
	ProviderID string         `json:"provider_id"`
	APIKey     string         `json:"api_key"`
	Weight     int            `json:"weight"`
	Priority   int            `json:"priority"`
	Enabled    bool           `json:"enabled"`
	Limits     map[string]int `json:"limits"`
}

type UpdateAccountRequest struct {
	Weight   *int  `json:"weight,omitempty"`
	Priority *int  `json:"priority,omitempty"`
	Enabled  *bool `json:"enabled,omitempty"`
}

type StatsResponse struct {
	TotalRequests    int64            `json:"total_requests"`
	TotalTokens      int64            `json:"total_tokens"`
	TotalErrors      int64            `json:"total_errors"`
	ByProvider       map[string]int64 `json:"by_provider"`
	ByModel          map[string]int64 `json:"by_model"`
	TokensByProvider map[string]int64 `json:"tokens_by_provider"`
	Latency          LatencyStats     `json:"latency"`
}

type LatencyStats struct {
	AvgMs float64 `json:"avg_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

type ProviderResponse struct {
	ID        string   `json:"id"`
	APIBase   string   `json:"api_base"`
	Models    []string `json:"models"`
	IsDefault bool     `json:"is_default"`
	Enabled   bool     `json:"enabled"`
}

type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}

func (h *AdminHandler) ListAccounts(c *gin.Context) {
	ctx := c.Request.Context()

	providers, err := h.storage.ListProviders(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var accounts []AccountResponse
	for _, p := range providers {
		accs, err := h.storage.ListAccounts(ctx, p.ID)
		if err != nil {
			continue
		}

		for _, acc := range accs {
			limits := make(map[string]int)
			for _, limitType := range []domain.LimitType{
				domain.LimitTypeRPM,
				domain.LimitTypeDaily,
				domain.LimitTypeWindow5h,
				domain.LimitTypeMonthly,
			} {
				state, err := h.storage.GetRateLimit(ctx, acc.ID, limitType)
				if err == nil && state != nil {
					limits[string(limitType)] = state.Current
				}
			}

			var lastUsedAt *time.Time
			available := true
			if h.pool != nil {
				if state := h.pool.GetState(acc.ID); state != nil {
					available = state.Account.IsEnabled &&
						state.ConsecutiveFailures < domain.CircuitBreakerThreshold
					if !state.LastUsedAt.IsZero() {
						lastUsedAt = &state.LastUsedAt
					}
				}
			}

			accounts = append(accounts, AccountResponse{
				ID:         acc.ID,
				Provider:   acc.ProviderID,
				Enabled:    acc.IsEnabled,
				Weight:     acc.Weight,
				Priority:   acc.Priority,
				Limits:     limits,
				Available:  available,
				LastUsedAt: lastUsedAt,
			})
		}
	}

	c.JSON(http.StatusOK, accounts)
}

func (h *AdminHandler) GetAccount(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	acc, err := h.storage.GetAccount(ctx, id)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	limits := make(map[string]int)
	for _, limitType := range []domain.LimitType{
		domain.LimitTypeRPM,
		domain.LimitTypeDaily,
		domain.LimitTypeWindow5h,
		domain.LimitTypeMonthly,
		domain.LimitTypeTokenDaily,
		domain.LimitTypeTokenMonthly,
	} {
		state, err := h.storage.GetRateLimit(ctx, acc.ID, limitType)
		if err == nil && state != nil {
			limits[string(limitType)] = state.Current
		}
	}

	var lastUsedAt *time.Time
	available := true
	consecutiveFailures := 0
	if h.pool != nil {
		if state := h.pool.GetState(acc.ID); state != nil {
			available = state.Account.IsEnabled &&
				state.ConsecutiveFailures < domain.CircuitBreakerThreshold
			consecutiveFailures = state.ConsecutiveFailures
			if !state.LastUsedAt.IsZero() {
				lastUsedAt = &state.LastUsedAt
			}
		}
	}

	response := gin.H{
		"id":                   acc.ID,
		"provider":             acc.ProviderID,
		"enabled":              acc.IsEnabled,
		"weight":               acc.Weight,
		"priority":             acc.Priority,
		"limits":               limits,
		"available":            available,
		"consecutive_failures": consecutiveFailures,
	}
	if lastUsedAt != nil {
		response["last_used_at"] = lastUsedAt
	}

	c.JSON(http.StatusOK, response)
}

func (h *AdminHandler) AddAccount(c *gin.Context) {
	var req AddAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ID == "" {
		req.ID = utils.GenerateUUID()
	}

	if req.ProviderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_id is required"})
		return
	}

	if req.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_key is required"})
		return
	}

	ctx := c.Request.Context()

	_, err := h.storage.GetProvider(ctx, req.ProviderID)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	account := &domain.Account{
		ID:         req.ID,
		ProviderID: req.ProviderID,
		APIKeyHash: utils.HashAPIKey(req.APIKey),
		Weight:     req.Weight,
		Priority:   req.Priority,
		IsEnabled:  req.Enabled,
	}

	if err := account.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.storage.UpsertAccount(ctx, account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.pool != nil {
		h.pool.Add(account)
	}

	c.JSON(http.StatusCreated, AccountResponse{
		ID:       account.ID,
		Provider: account.ProviderID,
		Enabled:  account.IsEnabled,
		Weight:   account.Weight,
		Priority: account.Priority,
		Limits:   make(map[string]int),
	})
}

func (h *AdminHandler) UpdateAccount(c *gin.Context) {
	id := c.Param("id")
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	acc, err := h.storage.GetAccount(ctx, id)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Weight != nil {
		acc.Weight = *req.Weight
	}
	if req.Priority != nil {
		acc.Priority = *req.Priority
	}
	if req.Enabled != nil {
		acc.IsEnabled = *req.Enabled
	}

	if err := h.storage.UpsertAccount(ctx, acc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.pool != nil {
		if req.Enabled != nil {
			h.pool.SetEnabled(id, *req.Enabled)
		}
		if req.Weight != nil {
			h.pool.UpdateWeight(id, *req.Weight)
		}
	}

	c.JSON(http.StatusOK, AccountResponse{
		ID:       acc.ID,
		Provider: acc.ProviderID,
		Enabled:  acc.IsEnabled,
		Weight:   acc.Weight,
		Priority: acc.Priority,
	})
}

func (h *AdminHandler) DeleteAccount(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	_, err := h.storage.GetAccount(ctx, id)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// SQLite doesn't have DeleteAccount in the interface, so we'll handle this
	// by disabling the account
	acc, _ := h.storage.GetAccount(ctx, id)
	acc.IsEnabled = false
	if err := h.storage.UpsertAccount(ctx, acc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.pool != nil {
		h.pool.SetEnabled(id, false)
	}

	c.JSON(http.StatusOK, gin.H{"message": "account disabled"})
}

func (h *AdminHandler) ResetLimits(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	_, err := h.storage.GetAccount(ctx, id)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, limitType := range []domain.LimitType{
		domain.LimitTypeRPM,
		domain.LimitTypeDaily,
		domain.LimitTypeWindow5h,
		domain.LimitTypeMonthly,
		domain.LimitTypeTokenDaily,
		domain.LimitTypeTokenMonthly,
	} {
		if err := h.storage.ResetRateLimit(ctx, id, limitType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if h.pool != nil {
		h.pool.ResetFailures(id)
	}

	c.JSON(http.StatusOK, gin.H{"message": "limits reset"})
}

func (h *AdminHandler) GetStats(c *gin.Context) {
	if h.collector == nil {
		c.JSON(http.StatusOK, StatsResponse{})
		return
	}

	snapshot := h.collector.GetSnapshot()

	var totalRequests int64
	for _, m := range snapshot.Requests {
		totalRequests += m.Count
	}

	p50, p95, p99 := snapshot.LatencyPercentiles()
	var avgMs float64
	if snapshot.LatencyCount > 0 {
		avgMs = float64(snapshot.TotalLatency.Milliseconds()) / float64(snapshot.LatencyCount)
	}

	response := StatsResponse{
		TotalRequests:    totalRequests,
		TotalTokens:      snapshot.TotalTokens,
		TotalErrors:      snapshot.TotalErrors,
		ByProvider:       snapshot.RequestsByProvider(),
		ByModel:          snapshot.RequestsByModel(),
		TokensByProvider: snapshot.TokensByProvider(),
		Latency: LatencyStats{
			AvgMs: avgMs,
			P50Ms: float64(p50.Milliseconds()),
			P95Ms: float64(p95.Milliseconds()),
			P99Ms: float64(p99.Milliseconds()),
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *AdminHandler) ListProviders(c *gin.Context) {
	ctx := c.Request.Context()

	providers, err := h.storage.ListProviders(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var response []ProviderResponse
	for _, p := range providers {
		response = append(response, ProviderResponse{
			ID:        p.ID,
			APIBase:   p.APIBase,
			Models:    p.Models,
			IsDefault: p.IsDefault,
			Enabled:   p.IsEnabled,
		})
	}

	c.JSON(http.StatusOK, response)
}

func (h *AdminHandler) ReloadConfig(c *gin.Context) {
	if h.configLoader == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config loader not configured"})
		return
	}

	if err := h.configLoader(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "configuration reloaded"})
}

func (h *AdminHandler) Health(c *gin.Context) {
	checks := make(map[string]string)
	allHealthy := true

	ctx := c.Request.Context()

	if h.storage != nil {
		_, err := h.storage.ListProviders(ctx)
		if err != nil {
			checks["storage"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["storage"] = "healthy"
		}
	}

	if h.pool != nil {
		available := h.pool.GetAvailableAccounts()
		if len(available) > 0 {
			checks["pool"] = "healthy"
		} else {
			checks["pool"] = "no available accounts"
		}
	}

	if h.collector != nil {
		checks["collector"] = "healthy"
	}

	status := "healthy"
	if !allHealthy {
		status = "unhealthy"
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	})
}

func (h *AdminHandler) RegisterRoutes(r *gin.RouterGroup) {
	admin := r.Group("/admin")
	admin.Use(h.Auth())
	{
		admin.GET("/accounts", h.ListAccounts)
		admin.GET("/accounts/:id", h.GetAccount)
		admin.POST("/accounts", h.AddAccount)
		admin.PUT("/accounts/:id", h.UpdateAccount)
		admin.DELETE("/accounts/:id", h.DeleteAccount)
		admin.POST("/accounts/:id/reset", h.ResetLimits)
		admin.GET("/stats", h.GetStats)
		admin.GET("/providers", h.ListProviders)
		admin.POST("/reload", h.ReloadConfig)
		admin.GET("/health", h.Health)
	}
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if domainErr, ok := err.(*domain.DomainError); ok {
		return domainErr.Code == domain.ErrCodeAccountNotFound ||
			domainErr.Code == domain.ErrCodeProviderNotFound
	}
	return strings.Contains(err.Error(), "not found")
}
