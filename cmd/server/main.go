package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"

	"github.com/wangluyao/aiproxy/internal/config"
	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/limiter"
	"github.com/wangluyao/aiproxy/internal/middleware"
	"github.com/wangluyao/aiproxy/internal/pool"
	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/internal/proxy"
	"github.com/wangluyao/aiproxy/internal/resilience"
	"github.com/wangluyao/aiproxy/internal/router"
	"github.com/wangluyao/aiproxy/internal/stats"
	"github.com/wangluyao/aiproxy/internal/storage"
	"github.com/wangluyao/aiproxy/pkg/openai"
	"github.com/wangluyao/aiproxy/pkg/utils"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

//go:embed web/*
var webFS embed.FS

type Server struct {
	config              *config.Config
	storage             storage.Storage
	registry            *provider.Registry
	router              *router.Router
	proxy               *proxy.Proxy
	statsCollector      *stats.Collector
	statsReporter       *stats.Reporter
	accountPools        map[string]*pool.Pool
	selectors           map[string]*pool.WeightedRoundRobin
	limiters            map[string]*limiter.CompositeLimiter
	retries             map[string]*resilience.Retry
	circuitBreakers     map[string]*resilience.CircuitBreaker
	maxResponseBodySize int64
	httpClient          *http.Client
}

func main() {
	configPath := flag.String("config", "", "Path to config file (default: config/config.json or AIPROXY_CONFIG env)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("AIProxy %s (built %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	if *configPath == "" {
		*configPath = os.Getenv("AIPROXY_CONFIG")
		if *configPath == "" {
			*configPath = "config/config.json"
		}
	}

	s := &Server{
		accountPools:    make(map[string]*pool.Pool),
		selectors:       make(map[string]*pool.WeightedRoundRobin),
		limiters:        make(map[string]*limiter.CompositeLimiter),
		retries:         make(map[string]*resilience.Retry),
		circuitBreakers: make(map[string]*resilience.CircuitBreaker),
	}

	if err := s.run(*configPath); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func (s *Server) run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	s.config = cfg

	s.maxResponseBodySize = cfg.Server.MaxResponseBodySize
	if s.maxResponseBodySize <= 0 {
		s.maxResponseBodySize = 50 * 1024 * 1024
	}

	if err := s.initLogging(); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	slog.Info("starting AIProxy", "version", Version, "build", BuildTime)

	if err := s.initStorage(); err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer s.storage.Close()

	if err := s.initProviders(); err != nil {
		return fmt.Errorf("failed to initialize providers: %w", err)
	}

	s.initStats()

	s.initProxy()

	s.initHTTPClient()

	s.startCleanupTask()

	publicServer, err := s.setupPublicAPI()
	if err != nil {
		return fmt.Errorf("failed to setup public API: %w", err)
	}

	adminServer, err := s.setupAdminAPI()
	if err != nil {
		return fmt.Errorf("failed to setup admin API: %w", err)
	}

	errChan := make(chan error, 2)

	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		slog.Info("starting public API server", "address", addr)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("public API server error: %w", err)
		}
	}()

	go func() {
		if !cfg.Admin.Enabled {
			return
		}
		slog.Info("starting admin API server", "address", cfg.Admin.Listen)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("admin API server error: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return err
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig)
	}

	shutdownTimeout, err := time.ParseDuration(cfg.Server.GracefulShutdownTimeout)
	if err != nil {
		shutdownTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	slog.Info("shutting down servers", "timeout", shutdownTimeout)

	if err := publicServer.Shutdown(ctx); err != nil {
		slog.Error("failed to shutdown public API server", "error", err)
	}

	if cfg.Admin.Enabled {
		if err := adminServer.Shutdown(ctx); err != nil {
			slog.Error("failed to shutdown admin API server", "error", err)
		}
	}

	slog.Info("server shutdown complete")
	return nil
}

func (s *Server) initLogging() error {
	var level slog.Level
	switch s.config.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if s.config.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	slog.SetDefault(slog.New(handler))
	return nil
}

func (s *Server) initStorage() error {
	dbPath := s.config.Database.Path
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	store, err := storage.NewSQLite(&s.config.Database)
	if err != nil {
		return fmt.Errorf("failed to initialize SQLite storage: %w", err)
	}

	s.storage = store
	slog.Info("initialized database", "path", dbPath)
	return nil
}

func (s *Server) initProviders() error {
	s.registry = provider.NewRegistry()
	s.router = router.NewRouter(nil)

	providers := make([]provider.Provider, 0, len(s.config.Providers))
	providerModels := make(map[string][]string)

	for _, pc := range s.config.Providers {
		if !pc.IsEnabled {
			continue
		}

		if len(pc.APIKeys) == 0 {
			slog.Warn("provider has no API keys, skipping", "provider", pc.Name)
			continue
		}

		var p provider.Provider
		switch pc.Name {
		case "openai":
			p = provider.NewOpenAIProvider(pc.APIKeys[0].Key, nil, pc.Models, pc.Headers)
		case "openrouter":
			p = provider.NewOpenRouterProvider(pc.APIKeys[0].Key, nil, pc.Models, pc.Headers)
		case "groq":
			p = provider.NewGroqProvider(pc.APIKeys[0].Key, nil, pc.Models, pc.Headers)
		default:
			openaiProvider := provider.NewOpenAIProvider(pc.APIKeys[0].Key, nil, pc.Models, pc.Headers)
			openaiProvider.SetAPIBase(pc.APIBase)
			openaiProvider.SetName(pc.Name)
			p = openaiProvider
		}

		if timeoutSetter, ok := p.(interface {
			SetTimeout(time.Duration, time.Duration)
		}); ok {
			timeout := parseDuration(pc.Timeout, 120*time.Second)
			streamTimeout := parseDuration(pc.StreamTimeout, 600*time.Second)
			timeoutSetter.SetTimeout(timeout, streamTimeout)
			slog.Info("set provider timeout", "provider", pc.Name, "timeout", timeout, "stream_timeout", streamTimeout)
		}

		if err := s.registry.Register(p); err != nil {
			slog.Warn("failed to register provider", "provider", pc.Name, "error", err)
			continue
		}

		providers = append(providers, p)
		providerModels[pc.Name] = pc.Models

		if pc.IsDefault {
			if err := s.registry.SetDefault(pc.Name); err != nil {
				slog.Warn("failed to set default provider", "provider", pc.Name, "error", err)
			}
		}

		if err := s.initAccountPool(pc); err != nil {
			slog.Warn("failed to initialize account pool", "provider", pc.Name, "error", err)
		}
	}

	s.router = router.NewRouterWithModels(providers, providerModels)

	for mapping, target := range s.config.ModelMapping {
		s.router.AddMapping(mapping, target)
	}

	slog.Info("initialized providers", "count", len(providers))
	return nil
}

func (s *Server) initAccountPool(pc config.ProviderConfig) error {
	accounts := make([]*domain.Account, 0, len(pc.APIKeys))

	for _, keyConfig := range pc.APIKeys {
		if !keyConfig.IsEnabled {
			continue
		}

		account := &domain.Account{
			ID:         utils.GenerateAccountID(pc.Name, keyConfig.Key),
			ProviderID: pc.Name,
			APIKeyHash: keyConfig.Key,
			Weight:     keyConfig.Weight,
			Priority:   keyConfig.Priority,
			IsEnabled:  true,
		}

		if err := account.Validate(); err != nil {
			slog.Warn("invalid account configuration", "provider", pc.Name, "error", err)
			continue
		}

		accounts = append(accounts, account)

		if err := s.storage.UpsertAccount(context.Background(), account); err != nil {
			slog.Warn("failed to persist account", "provider", pc.Name, "error", err)
		}

		if keyConfig.Limits != nil {
			s.initAccountLimiter(account.ID, keyConfig.Limits)
		}

		// Initialize circuit breaker for each account
		s.circuitBreakers[account.ID] = resilience.NewCircuitBreaker(&resilience.CircuitBreakerConfig{
			FailureThreshold: pc.CircuitBreaker.Threshold,
			SuccessThreshold: pc.CircuitBreaker.HalfOpenRequests,
			RecoveryTimeout:  parseDuration(pc.CircuitBreaker.Timeout, 60*time.Second),
		})
	}

	p := pool.NewPool(accounts)
	s.accountPools[pc.Name] = p

	if compositeLimiter, ok := s.limiters[pc.Name]; ok {
		s.selectors[pc.Name] = pool.NewWeightedRoundRobin(p, compositeLimiter)
	} else {
		s.selectors[pc.Name] = pool.NewWeightedRoundRobin(p, nil)
	}

	// Initialize retry for provider
	s.retries[pc.Name] = resilience.NewRetry(&resilience.RetryConfig{
		MaxAttempts:   pc.Retry.MaxRetries,
		InitialDelay:  parseDuration(pc.Retry.InitialWait, 1*time.Second),
		MaxDelay:      parseDuration(pc.Retry.MaxWait, 30*time.Second),
		Multiplier:    pc.Retry.Multiplier,
		RetryOnStatus: []int{429, 500, 502, 503, 504},
	})

	slog.Info("initialized account pool", "provider", pc.Name, "accounts", len(accounts))
	return nil
}

func (s *Server) initAccountLimiter(accountID string, limits *domain.AccountLimits) {
	var limiters []limiter.Limiter

	if limits.RPM != nil && *limits.RPM > 0 {
		limiters = append(limiters, limiter.NewRPM(s.storage, *limits.RPM))
	}
	if limits.Daily != nil && *limits.Daily > 0 {
		limiters = append(limiters, limiter.NewDaily(s.storage, *limits.Daily))
	}
	if limits.Window5h != nil && *limits.Window5h > 0 {
		windowDuration := parseDuration(s.config.RateLimits.Window5hDuration, 5*time.Hour)
		limiters = append(limiters, limiter.NewWindow5hWithDuration(s.storage, *limits.Window5h, windowDuration))
	}
	if limits.Monthly != nil && *limits.Monthly > 0 {
		limiters = append(limiters, limiter.NewMonthly(s.storage, *limits.Monthly))
	}
	if limits.TokenDaily != nil && *limits.TokenDaily > 0 {
		limiters = append(limiters, limiter.NewTokenDaily(s.storage, *limits.TokenDaily))
	}
	if limits.TokenMonthly != nil && *limits.TokenMonthly > 0 {
		limiters = append(limiters, limiter.NewTokenMonthly(s.storage, *limits.TokenMonthly))
	}

	if len(limiters) > 0 {
		s.limiters[accountID] = limiter.NewCompositeLimiter(limiters...)
	}
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

func (s *Server) initStats() {
	s.statsCollector = stats.NewCollector()
	s.statsReporter = stats.NewReporterWithNamespace(s.statsCollector, s.config.Metrics.Namespace)
	slog.Info("initialized stats collector", "namespace", s.config.Metrics.Namespace)
}

func (s *Server) startCleanupTask() {
	cleanupInterval := parseDuration(s.config.RateLimits.CleanupInterval, time.Hour)
	if cleanupInterval <= 0 {
		cleanupInterval = time.Hour
	}

	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		slog.Info("started rate limit cleanup task", "interval", cleanupInterval)

		for range ticker.C {
			ctx := context.Background()
			if err := s.storage.CleanupExpiredRateLimits(ctx); err != nil {
				slog.Error("failed to cleanup expired rate limits", "error", err)
			}
		}
	}()
}

func (s *Server) initProxy() {
	timeout, _ := time.ParseDuration(s.config.Server.WriteTimeout)
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	s.proxy = proxy.NewProxy(&proxy.Config{
		Timeout:         timeout,
		StreamTimeout:   timeout * 2,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	})

	charsPerToken := s.config.TokenTracking.EstimationCharsPerToken
	if charsPerToken <= 0 {
		charsPerToken = 4
	}
	streamingMode := s.config.TokenTracking.StreamingMode
	if streamingMode == "" {
		streamingMode = "hybrid"
	}

	slog.Info("initialized proxy", "chars_per_token", charsPerToken, "streaming_mode", streamingMode)
}

func (s *Server) initHTTPClient() {
	cfg := s.config.HTTPTransport

	idleConnTimeout := parseDuration(cfg.IdleConnTimeout, 300*time.Second)
	responseHeaderTimeout := parseDuration(cfg.ResponseHeaderTimeout, 10*time.Minute)
	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 100
	}
	maxIdleConnsPerHost := cfg.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = 20
	}

	s.httpClient = &http.Client{
		// 不设全局 Timeout，由每次请求的 context.WithTimeout 统一控制
		// 避免 http.Client.Timeout 与 context 超时竞争导致流式响应 EOF
		Timeout: 0,
		Transport: &http.Transport{
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			MaxConnsPerHost:       maxIdleConns,
			IdleConnTimeout:       idleConnTimeout,
			DisableKeepAlives:     cfg.DisableKeepAlives,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	slog.Info("initialized HTTP client",
		"disable_keep_alives", cfg.DisableKeepAlives,
		"idle_conn_timeout", idleConnTimeout,
		"response_header_timeout", responseHeaderTimeout,
		"max_idle_conns", maxIdleConns,
		"max_idle_conns_per_host", maxIdleConnsPerHost,
	)
}

func (s *Server) setupPublicAPI() (*http.Server, error) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	engine.Use(middleware.Recovery())
	engine.Use(middleware.RequestID(&middleware.RequestIDConfig{
		HeaderName:        s.config.RequestID.HeaderName,
		GenerateIfMissing: s.config.RequestID.GenerateIfMissing,
	}))
	engine.Use(middleware.Logging(&middleware.LoggingConfig{
		Level:               s.config.Logging.Level,
		IncludeRequestBody:  s.config.Logging.IncludeRequestBody,
		IncludeResponseBody: s.config.Logging.IncludeResponseBody,
	}))

	authConfig := &middleware.AuthConfig{
		Enabled:    len(s.config.Auth.APIKeys) > 0 || s.storage != nil,
		APIKeys:    make(map[string]bool),
		HeaderName: "Authorization",
		KeyPrefix:  "Bearer ",
		Storage:    s.storage,
	}
	for _, key := range s.config.Auth.APIKeys {
		authConfig.APIKeys[key] = true
	}
	engine.Use(middleware.Auth(authConfig))
	engine.Use(middleware.UpdateAPIKeyUsage(authConfig))

	engine.POST("/v1/chat/completions", s.handleChatCompletions)
	engine.GET("/v1/models", s.handleListModels)
	engine.GET("/health", s.handleHealth)
	engine.GET("/ready", s.handleReady)

	if s.config.Metrics.Enabled && s.config.Metrics.Prometheus.Enabled {
		statsHandler := stats.NewHandler(s.statsReporter)
		engine.GET(s.config.Metrics.Prometheus.Path, gin.WrapH(http.HandlerFunc(statsHandler.ServePrometheus)))
	}

	readTimeout, _ := time.ParseDuration(s.config.Server.ReadTimeout)
	writeTimeout, _ := time.ParseDuration(s.config.Server.WriteTimeout)
	idleTimeout, _ := time.ParseDuration(s.config.Server.IdleTimeout)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Handler:      engine,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	return server, nil
}

func (s *Server) setupAdminAPI() (*http.Server, error) {
	if !s.config.Admin.Enabled {
		return &http.Server{}, nil
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	engine.Use(middleware.Recovery())
	engine.Use(middleware.RequestID(&middleware.RequestIDConfig{
		HeaderName:        "X-Request-ID",
		GenerateIfMissing: true,
	}))

	engine.GET("/", s.handleDashboard)
	engine.GET("/dashboard", s.handleDashboard)
	engine.GET("/css/:filename", s.handleCSSFiles)
	engine.GET("/js/:filename", s.handleJSFiles)

	adminAuth := &middleware.AuthConfig{
		Enabled:    len(s.config.Admin.APIKeys) > 0,
		APIKeys:    make(map[string]bool),
		HeaderName: "Authorization",
		KeyPrefix:  "Bearer ",
	}
	for _, key := range s.config.Admin.APIKeys {
		adminAuth.APIKeys[key] = true
	}

	adminGroup := engine.Group("")
	adminGroup.Use(middleware.Auth(adminAuth))
	adminGroup.GET("/admin/accounts", s.handleAdminListAccounts)
	adminGroup.GET("/admin/accounts/:id", s.handleAdminGetAccount)
	adminGroup.GET("/admin/accounts/:id/limits", s.handleAdminGetAccountLimits)
	adminGroup.GET("/admin/accounts/:id/models", s.handleAdminAccountModelStats)
	adminGroup.POST("/admin/accounts", s.handleAdminCreateAccount)
	adminGroup.PUT("/admin/accounts/:id", s.handleAdminUpdateAccount)
	adminGroup.DELETE("/admin/accounts/:id", s.handleAdminDeleteAccount)
	adminGroup.POST("/admin/accounts/:id/reset", s.handleAdminResetAccount)
	adminGroup.POST("/admin/accounts/batch", s.handleAdminBatchAccountOperation)
	adminGroup.GET("/admin/api-keys", s.handleAdminListAPIKeys)
	adminGroup.POST("/admin/api-keys", s.handleAdminCreateAPIKey)
	adminGroup.DELETE("/admin/api-keys/:id", s.handleAdminDeleteAPIKey)
	adminGroup.PUT("/admin/api-keys/:id/toggle", s.handleAdminToggleAPIKey)
	adminGroup.GET("/admin/stats", s.handleAdminStatsFromDB)
	adminGroup.GET("/admin/stats/timeseries", s.handleAdminTimeSeries)
	adminGroup.GET("/admin/stats/accounts", s.handleAdminAllAccountStats)
	adminGroup.GET("/admin/stats/models", s.handleAdminModelStats)
	adminGroup.GET("/admin/providers", s.handleAdminListProviders)
	adminGroup.GET("/admin/providers/stats", s.handleAdminProviderStats)
	adminGroup.GET("/admin/logs", s.handleAdminLogs)
	adminGroup.GET("/admin/logs/:id", s.handleAdminLogDetail)
	adminGroup.GET("/admin/version", s.handleAdminVersion)
	adminGroup.GET("/admin/model-mapping", s.handleAdminModelMapping)
	adminGroup.GET("/admin/export/:type", s.handleAdminExport)
	adminGroup.POST("/admin/reload", s.handleAdminReload)
	adminGroup.GET("/admin/health", s.handleHealth)

	server := &http.Server{
		Addr:    s.config.Admin.Listen,
		Handler: engine,
	}

	return server, nil
}

func (s *Server) handleChatCompletions(c *gin.Context) {
	startTime := time.Now()

	var req openai.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("failed to parse request body", "error", err, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: fmt.Sprintf("invalid request body: %s", err.Error()),
				Type:    "invalid_request_error",
				Code:    "invalid_request",
			},
		})
		return
	}

	// Try with fallback if enabled
	if s.config.Fallback.Enabled {
		s.handleWithFallback(c, &req, startTime)
		return
	}

	s.handleWithProvider(c, &req, "", startTime)
}

func (s *Server) handleWithFallback(c *gin.Context, req *openai.ChatCompletionRequest, startTime time.Time) {
	providers := s.config.Fallback.Providers
	if len(providers) == 0 {
		prov, err := s.router.Resolve(req.Model)
		if err != nil {
			s.sendModelNotFoundError(c, req.Model)
			return
		}
		providers = []string{prov.Name()}
	}

	var lastErr error
	for _, providerName := range providers {
		prov, err := s.router.ResolveByHeader(req.Model, providerName)
		if err != nil {
			continue
		}

		c.Set("fallback_provider", providerName)
		_, _, err = s.executeRequest(c, req, prov, providerName, startTime)
		if err == nil {
			return
		}
		lastErr = err
		slog.Warn("fallback provider failed", "provider", providerName, "error", err)
	}

	if lastErr != nil {
		c.JSON(http.StatusBadGateway, openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: lastErr.Error(),
				Type:    "api_error",
				Code:    "all_providers_failed",
			},
		})
	} else {
		s.sendModelNotFoundError(c, req.Model)
	}
}

func (s *Server) handleWithProvider(c *gin.Context, req *openai.ChatCompletionRequest, providerOverride string, startTime time.Time) {
	var prov provider.Provider
	var err error

	if providerOverride != "" {
		prov, err = s.router.ResolveByHeader(req.Model, providerOverride)
	} else {
		prov, err = s.router.Resolve(req.Model)
	}

	if err != nil {
		s.sendModelNotFoundError(c, req.Model)
		return
	}

	_, _, err = s.executeRequest(c, req, prov, prov.Name(), startTime)
	if err != nil {
		c.JSON(http.StatusBadGateway, openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: err.Error(),
				Type:    "api_error",
				Code:    "request_failed",
			},
		})
	}
}

func (s *Server) executeRequest(c *gin.Context, req *openai.ChatCompletionRequest, prov provider.Provider, providerName string, startTime time.Time) (*http.Response, *domain.Account, error) {
	selector, ok := s.selectors[providerName]
	if !ok {
		return nil, nil, fmt.Errorf("no account pool for provider: %s", providerName)
	}

	account, err := s.selectAvailableAccount(selector, providerName)
	if err != nil {
		return nil, nil, err
	}

	slog.Info("account selected", "account_id", account.ID[:8], "weight", account.Weight, "priority", account.Priority, "provider", providerName)

	mappedModel := s.router.GetMappedModel(req.Model)
	// 创建请求副本，使用 mappedModel 发送到上游
	// 不直接修改 req.Model，避免 fallback 场景下污染原始请求
	reqCopy := *req
	reqCopy.Model = mappedModel

	retry := s.retries[providerName]

	var lastErr error

	maxAttempts := 1
	if retry != nil {
		maxAttempts = retry.GetMaxAttempts()
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			// Use configured retry delay
			delay := time.Second
			if retry != nil {
				delay = retry.CalculateDelay(attempt)
			}
			select {
			case <-c.Request.Context().Done():
				return nil, nil, c.Request.Context().Err()
			case <-time.After(delay):
			}
			slog.Info("retrying request", "attempt", attempt, "delay", delay, "provider", providerName, "account_id", account.ID[:8])
		}

		httpReq, err := prov.TransformRequest(&reqCopy, account.APIKeyHash)
		if err != nil {
			return nil, nil, err
		}

		timeout := prov.GetTimeout(req.Stream)
		// 注意：在循环体内不能使用 defer cancel()，因为 defer 不会在每次迭代时执行
		// 而是在整个函数退出时才统一执行，这会导致 context 泄漏和相互干扰
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)

		resp, err := s.httpClient.Do(httpReq.WithContext(ctx))
		if err != nil {
			// 请求失败，立即释放 context 资源
			cancel()
			slog.Error("upstream request failed",
				"error", err.Error(),
				"error_type", fmt.Sprintf("%T", err),
				"provider", providerName,
				"model", mappedModel,
				"account_id", account.ID[:8],
				"timeout", timeout,
				"attempt", attempt,
			)
			s.statsCollector.RecordError(providerName, mappedModel, "request_failed")
			s.recordAccountFailure(account.ID)
			lastErr = err
			continue
		}

		// 5xx 或 429：需要重试，先释放 context 再 continue
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			cancel()
			s.statsCollector.RecordError(providerName, mappedModel, "upstream_error")
			s.recordAccountFailure(account.ID)
			lastErr = fmt.Errorf("upstream returned status %d", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		// 4xx 客户端错误：转发错误后释放 context
		if resp.StatusCode >= 400 {
			cancel()
			s.statsCollector.RecordError(providerName, mappedModel, "client_error")
			s.forwardUpstreamError(c, resp)
			return nil, nil, fmt.Errorf("client error: %d", resp.StatusCode)
		}

		s.recordAccountSuccess(account.ID)

		// 成功：响应处理完成后再释放 context（流式需要读取 resp.Body）
		if reqCopy.Stream {
			s.handleStreamResponse(c, resp, account, providerName, &reqCopy, startTime)
		} else {
			s.handleNonStreamResponse(c, resp, account, providerName, &reqCopy, startTime)
		}
		cancel()

		return resp, account, nil
	}

	return nil, nil, lastErr
}

func (s *Server) selectAvailableAccount(selector *pool.WeightedRoundRobin, providerName string) (*domain.Account, error) {
	// 计算最大尝试次数，避免所有账号熔断时无限循环导致 goroutine 泄漏
	maxTries := len(s.circuitBreakers)*2 + 1
	if maxTries < 10 {
		maxTries = 10
	}

	for i := 0; i < maxTries; i++ {
		account, err := selector.Select(context.Background(), nil)
		if err != nil {
			return nil, fmt.Errorf("no available accounts for provider: %s", providerName)
		}

		if cb, ok := s.circuitBreakers[account.ID]; ok {
			if !cb.Allow() {
				slog.Warn("account circuit breaker open, skipping", "account_id", account.ID[:8])
				continue
			}
		}

		return account, nil
	}

	return nil, fmt.Errorf("all accounts circuit open for provider: %s, no available account", providerName)
}

func (s *Server) sendModelNotFoundError(c *gin.Context, model string) {
	c.JSON(http.StatusNotFound, openai.ErrorResponse{
		Error: openai.ErrorDetail{
			Message: fmt.Sprintf("no provider supports model %s", model),
			Type:    "model_not_found",
			Code:    "model_not_found",
		},
	})
}

func (s *Server) handleStreamResponse(c *gin.Context, resp *http.Response, account *domain.Account, providerName string, req *openai.ChatCompletionRequest, startTime time.Time) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	charsPerToken := s.config.TokenTracking.EstimationCharsPerToken
	if charsPerToken <= 0 {
		charsPerToken = 4
	}
	streamingMode := s.config.TokenTracking.StreamingMode
	if streamingMode == "" {
		streamingMode = "hybrid"
	}
	streamHandler := proxy.NewStreamHandlerWithConfig(s.proxy, charsPerToken, streamingMode)

	if err := streamHandler.ServeStream(c.Writer, c.Request, resp, startTime); err != nil {
		slog.Error("stream error", "error", err, "request_id", c.GetString("request_id"))
	}

	ttft := streamHandler.GetTTFT()
	latency := time.Since(startTime)
	promptTokens, completionTokens, found := streamHandler.GetTokenExtractor().ExtractFromStream(nil)

	var totalTokens int
	if found {
		totalTokens = promptTokens + completionTokens
		s.statsCollector.RecordRequest(providerName, req.Model, http.StatusOK, latency, totalTokens)
		s.recordTokenUsage(account.ID, providerName, req.Model, promptTokens, completionTokens)
	} else {
		s.statsCollector.RecordRequest(providerName, req.Model, http.StatusOK, latency, 0)
	}

	if ttft > 0 {
		s.statsCollector.RecordTTFT(providerName, req.Model, ttft)
	}

	reqBody := ""
	respBody := ""
	if s.config.Logging.IncludeRequestBody {
		reqBodyJSON, _ := json.Marshal(req)
		reqBody = string(reqBodyJSON)
	}
	s.recordRequestLog(c.GetString("request_id"), account.ID, providerName, req.Model, http.StatusOK, totalTokens, float64(ttft.Milliseconds()), float64(latency.Milliseconds()), "", "", reqBody, respBody, req.Stream)

	slog.Info("stream completed", "provider", providerName, "model", req.Model, "tokens", totalTokens, "ttft", ttft, "latency", latency)
}

func (s *Server) handleNonStreamResponse(c *gin.Context, resp *http.Response, account *domain.Account, providerName string, req *openai.ChatCompletionRequest, startTime time.Time) {
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	limitedReader := io.LimitReader(resp.Body, s.maxResponseBodySize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		slog.Error("failed to read response body", "error", err, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadGateway, openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "failed to read upstream response: " + err.Error(),
				Type:    "upstream_error",
				Code:    "read_error",
			},
		})
		return
	}

	if int64(len(bodyBytes)) > s.maxResponseBodySize {
		slog.Error("response too large", "size", len(bodyBytes), "limit", s.maxResponseBodySize, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadGateway, openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "upstream response exceeds maximum size limit",
				Type:    "upstream_error",
				Code:    "response_too_large",
			},
		})
		return
	}

	c.Status(resp.StatusCode)
	c.Writer.Write(bodyBytes)

	ttft := time.Since(startTime)
	latency := time.Since(startTime)

	var chatResp openai.ChatCompletionResponse
	var totalTokens int
	if err := json.Unmarshal(bodyBytes, &chatResp); err == nil && chatResp.Usage != nil {
		totalTokens = chatResp.Usage.TotalTokens
		s.statsCollector.RecordRequest(providerName, req.Model, resp.StatusCode, latency, totalTokens)
		s.recordTokenUsage(account.ID, providerName, req.Model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)
	} else {
		s.statsCollector.RecordRequest(providerName, req.Model, resp.StatusCode, latency, 0)
	}

	s.statsCollector.RecordTTFT(providerName, req.Model, ttft)

	reqBody := ""
	respBody := ""
	if s.config.Logging.IncludeRequestBody {
		reqBodyJSON, _ := json.Marshal(req)
		reqBody = string(reqBodyJSON)
	}
	if s.config.Logging.IncludeResponseBody {
		respBody = string(bodyBytes)
		if len(respBody) > 10000 {
			respBody = respBody[:10000] + "...(truncated)"
		}
	}
	s.recordRequestLog(c.GetString("request_id"), account.ID, providerName, req.Model, resp.StatusCode, totalTokens, float64(ttft.Milliseconds()), float64(latency.Milliseconds()), "", "", reqBody, respBody, false)

	slog.Info("request completed", "provider", providerName, "model", req.Model, "tokens", totalTokens, "ttft", ttft, "latency", latency, "status", resp.StatusCode)
}

func (s *Server) forwardUpstreamError(c *gin.Context, resp *http.Response) {
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, 64*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		c.JSON(resp.StatusCode, openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "failed to read upstream error",
				Type:    "upstream_error",
				Code:    "read_error",
			},
		})
		return
	}

	var errResp openai.ErrorResponse
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error.Message != "" {
		c.JSON(resp.StatusCode, errResp)
		return
	}

	c.JSON(resp.StatusCode, openai.ErrorResponse{
		Error: openai.ErrorDetail{
			Message: string(bodyBytes),
			Type:    "upstream_error",
			Code:    "upstream_error",
		},
	})
}

func (s *Server) recordAccountSuccess(accountID string) {
	for _, pool := range s.accountPools {
		pool.RecordSuccess(accountID)
	}
	if cb, ok := s.circuitBreakers[accountID]; ok {
		cb.RecordSuccess()
	}
	if s.storage != nil {
		if err := s.storage.UpdateAccountLastUsed(context.Background(), accountID); err != nil {
			slog.Warn("failed to update account last used", "account_id", accountID, "error", err)
		}
	}
}

func (s *Server) recordAccountFailure(accountID string) {
	for _, pool := range s.accountPools {
		pool.RecordFailure(accountID)
	}
	if cb, ok := s.circuitBreakers[accountID]; ok {
		cb.RecordFailure()
	}
}

func (s *Server) recordTokenUsage(accountID, providerID, model string, promptTokens, completionTokens int) {
	if s.storage != nil && promptTokens > 0 {
		ctx := context.Background()
		usage := &storage.TokenUsage{
			RequestID:        utils.GenerateUUID(),
			AccountID:        accountID,
			ProviderID:       providerID,
			Model:            model,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
			IsStreaming:      false,
			IsEstimated:      false,
		}
		if err := s.storage.RecordTokenUsage(ctx, usage); err != nil {
			slog.Error("failed to record token usage", "error", err)
		}
	}
}

func (s *Server) recordRequestLog(requestID, accountID, providerID, model string, status, tokens int, ttftMs, latencyMs float64, errorType, errorMsg, requestBody, responseBody string, isStreaming bool) {
	if s.storage == nil {
		return
	}
	if requestID == "" {
		requestID = utils.GenerateUUID()
	}
	log := &storage.RequestLog{
		RequestID:    requestID,
		AccountID:    accountID,
		ProviderID:   providerID,
		Model:        model,
		Status:       status,
		Tokens:       tokens,
		TTFTMs:       ttftMs,
		LatencyMs:    latencyMs,
		ErrorType:    errorType,
		ErrorMessage: errorMsg,
		IsStreaming:  isStreaming,
		Timestamp:    time.Now(),
		RequestBody:  requestBody,
		ResponseBody: responseBody,
	}
	if err := s.storage.RecordRequestLog(context.Background(), log); err != nil {
		slog.Warn("failed to record request log", "error", err)
	}
}

func (s *Server) handleListModels(c *gin.Context) {
	models := s.router.ListModels()
	modelList := &openai.ModelList{
		Object: "list",
		Data:   make([]openai.Model, 0, len(models)),
	}

	for _, m := range models {
		modelList.Data = append(modelList.Data, openai.Model{
			ID:      m,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "system",
		})
	}

	c.JSON(http.StatusOK, modelList)
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   Version,
	})
}

func (s *Server) handleReady(c *gin.Context) {
	providers := s.registry.List()
	if len(providers) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "no providers configured",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminListAccounts(c *gin.Context) {
	providerName := c.Query("provider")
	statusFilter := c.Query("status")
	availableFilter := c.Query("available")
	result := make([]map[string]interface{}, 0)

	lastUsedMap := make(map[string]string)
	if s.storage != nil {
		if lastUsed, err := s.storage.GetAllAccountLastUsed(c.Request.Context()); err == nil {
			for id, t := range lastUsed {
				lastUsedMap[id] = t.UTC().Format(time.RFC3339)
			}
		}
	}

	for name, p := range s.accountPools {
		if providerName != "" && name != providerName {
			continue
		}
		for _, acc := range p.List() {
			state := p.GetState(acc.ID)

			isAvailable := acc.IsEnabled && state.ConsecutiveFailures < domain.CircuitBreakerThreshold
			isCircuitOpen := false
			if cb, ok := s.circuitBreakers[acc.ID]; ok {
				isCircuitOpen = !cb.Allow()
			}

			if statusFilter == "enabled" && !acc.IsEnabled {
				continue
			}
			if statusFilter == "disabled" && acc.IsEnabled {
				continue
			}
			if availableFilter == "true" && !isAvailable {
				continue
			}
			if availableFilter == "false" && isAvailable {
				continue
			}

			accountData := map[string]interface{}{
				"id":                   acc.ID,
				"provider_id":          acc.ProviderID,
				"weight":               acc.Weight,
				"priority":             acc.Priority,
				"is_enabled":           acc.IsEnabled,
				"available":            isAvailable,
				"circuit_open":         isCircuitOpen,
				"consecutive_failures": state.ConsecutiveFailures,
			}
			if lastUsed, ok := lastUsedMap[acc.ID]; ok {
				accountData["last_used_at"] = lastUsed
			}
			result = append(result, accountData)
		}
	}

	c.JSON(http.StatusOK, gin.H{"accounts": result})
}

func (s *Server) handleAdminGetAccount(c *gin.Context) {
	accountID := c.Param("id")

	for _, p := range s.accountPools {
		acc, err := p.Get(accountID)
		if err == nil {
			state := p.GetState(accountID)
			c.JSON(http.StatusOK, gin.H{
				"account": map[string]interface{}{
					"id":                   acc.ID,
					"provider_id":          acc.ProviderID,
					"weight":               acc.Weight,
					"priority":             acc.Priority,
					"is_enabled":           acc.IsEnabled,
					"consecutive_failures": state.ConsecutiveFailures,
					"last_used_at":         state.LastUsedAt,
				},
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
}

func (s *Server) handleAdminCreateAccount(c *gin.Context) {
	var req struct {
		ProviderID string `json:"provider_id"`
		APIKey     string `json:"api_key"`
		Weight     int    `json:"weight"`
		Priority   int    `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	account := &domain.Account{
		ID:         utils.GenerateUUID(),
		ProviderID: req.ProviderID,
		APIKeyHash: req.APIKey,
		Weight:     req.Weight,
		Priority:   req.Priority,
		IsEnabled:  true,
	}

	if account.Weight == 0 {
		account.Weight = 1
	}

	if err := s.storage.UpsertAccount(c.Request.Context(), account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if p, ok := s.accountPools[req.ProviderID]; ok {
		p.Add(account)
	}

	c.JSON(http.StatusCreated, gin.H{"account": account})
}

func (s *Server) handleAdminUpdateAccount(c *gin.Context) {
	accountID := c.Param("id")

	var req struct {
		Weight    *int  `json:"weight"`
		Priority  *int  `json:"priority"`
		IsEnabled *bool `json:"is_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	for providerName, p := range s.accountPools {
		acc, err := p.Get(accountID)
		if err == nil {
			if req.Weight != nil {
				acc.Weight = *req.Weight
				p.UpdateWeight(accountID, *req.Weight)
			}
			if req.Priority != nil {
				acc.Priority = *req.Priority
			}
			if req.IsEnabled != nil {
				p.SetEnabled(accountID, *req.IsEnabled)
			}

			if err := s.storage.UpsertAccount(c.Request.Context(), acc); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"account": acc, "provider": providerName})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
}

func (s *Server) handleAdminDeleteAccount(c *gin.Context) {
	accountID := c.Param("id")

	for providerName, p := range s.accountPools {
		if _, err := p.Get(accountID); err == nil {
			p.Remove(accountID)
			slog.Info("account deleted", "account_id", accountID, "provider", providerName)
			c.JSON(http.StatusOK, gin.H{"message": "account deleted"})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
}

func (s *Server) handleAdminResetAccount(c *gin.Context) {
	accountID := c.Param("id")

	for providerName, p := range s.accountPools {
		if _, err := p.Get(accountID); err == nil {
			p.ResetFailures(accountID)

			if l, ok := s.limiters[accountID]; ok {
				l.Reset(c.Request.Context(), accountID)
			}

			slog.Info("account reset", "account_id", accountID, "provider", providerName)
			c.JSON(http.StatusOK, gin.H{"message": "account reset"})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
}

func (s *Server) handleAdminStats(c *gin.Context) {
	statsHandler := stats.NewHandler(s.statsReporter)
	statsHandler.ServeJSON(c.Writer, c.Request)
}

func (s *Server) handleAdminStatsFromDB(c *gin.Context) {
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := fmt.Sscanf(h, "%d", &hours); err == nil && parsed == 1 {
			if hours > 720 {
				hours = 720
			}
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	timeSeries, err := s.storage.GetRequestTimeSeries(c.Request.Context(), since, "hour")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	modelStats, _ := s.storage.GetModelStats(c.Request.Context(), since)
	latencyData, _ := s.storage.GetLatencyData(c.Request.Context(), since)

	var totalRequests, totalTokens, totalErrors int64
	requestsByModel := make(map[string]int64)
	tokensByModel := make(map[string]int64)
	requestsByProvider := make(map[string]int64)
	tokensByProvider := make(map[string]int64)

	for _, p := range timeSeries {
		totalRequests += p.Count
		totalTokens += p.Tokens
		totalErrors += p.Errors
	}

	for _, m := range modelStats {
		requestsByModel[m.Model] = m.RequestCount
		tokensByModel[m.Model] = m.TotalTokens
		provider := "unknown"
		if parts := strings.Split(m.Model, "/"); len(parts) > 1 {
			provider = parts[0]
		}
		requestsByProvider[provider] += m.RequestCount
		tokensByProvider[provider] += m.TotalTokens
	}

	avgTTFT := 0.0
	avgLatency := 0.0
	if len(modelStats) > 0 {
		for _, m := range modelStats {
			avgTTFT += m.AvgTTFTMs
			avgLatency += m.AvgLatencyMs
		}
		avgTTFT /= float64(len(modelStats))
		avgLatency /= float64(len(modelStats))
	}

	latencyPercentiles := calculatePercentiles(latencyData, func(d *storage.LatencyData) float64 { return d.LatencyMs })
	ttftPercentiles := calculatePercentiles(latencyData, func(d *storage.LatencyData) float64 { return d.TTFTMs })

	c.JSON(http.StatusOK, gin.H{
		"total_requests":       totalRequests,
		"total_tokens":         totalTokens,
		"total_errors":         totalErrors,
		"avg_ttft_ms":          avgTTFT,
		"avg_latency_ms":       avgLatency,
		"requests_by_provider": requestsByProvider,
		"requests_by_model":    requestsByModel,
		"tokens_by_provider":   tokensByProvider,
		"tokens_by_model":      tokensByModel,
		"latency_percentiles":  latencyPercentiles,
		"ttft_percentiles":     ttftPercentiles,
		"timestamp":            time.Now().UTC().Format(time.RFC3339),
	})
}

func calculatePercentiles(data []*storage.LatencyData, getValue func(*storage.LatencyData) float64) map[string]float64 {
	if len(data) == 0 {
		return map[string]float64{"p50": 0, "p95": 0, "p99": 0}
	}

	values := make([]float64, 0, len(data))
	for _, d := range data {
		v := getValue(d)
		if v > 0 {
			values = append(values, v)
		}
	}

	if len(values) == 0 {
		return map[string]float64{"p50": 0, "p95": 0, "p99": 0}
	}

	sort.Float64s(values)
	n := len(values)

	p50 := values[n*50/100]
	p95 := values[n*95/100]
	p99 := values[n*99/100]

	return map[string]float64{"p50": p50, "p95": p95, "p99": p99}
}

func (s *Server) handleAdminListProviders(c *gin.Context) {
	providers := s.registry.List()
	result := make([]map[string]interface{}, 0, len(providers))

	for _, p := range providers {
		accountCount := 0
		if pool, ok := s.accountPools[p.Name()]; ok {
			accountCount = len(pool.List())
		}

		result = append(result, map[string]interface{}{
			"name":          p.Name(),
			"api_base":      p.APIBase(),
			"models":        s.router.ListModels(),
			"account_count": accountCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{"providers": result})
}

func (s *Server) handleAdminReload(c *gin.Context) {
	slog.Info("configuration reload requested")

	configPath := os.Getenv("AIPROXY_CONFIG")
	if configPath == "" {
		configPath = "config/config.json"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reload config: %v", err)})
		return
	}

	if err := config.Validate(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("config validation failed: %v", err)})
		return
	}

	s.config = cfg

	s.accountPools = make(map[string]*pool.Pool)
	s.selectors = make(map[string]*pool.WeightedRoundRobin)
	s.limiters = make(map[string]*limiter.CompositeLimiter)

	if err := s.initProviders(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reinitialize providers: %v", err)})
		return
	}

	slog.Info("configuration reloaded successfully")
	c.JSON(http.StatusOK, gin.H{"message": "configuration reloaded"})
}

func (s *Server) handleDashboard(c *gin.Context) {
	html, err := webFS.ReadFile("web/index.html")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load dashboard"})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", html)
}

func (s *Server) handleCSSFiles(c *gin.Context) {
	filename := c.Param("filename")
	fullPath := "web/css/" + filename

	data, err := webFS.ReadFile(fullPath)
	if err != nil {
		slog.Error("failed to read CSS file", "path", fullPath, "error", err)
		c.Status(http.StatusNotFound)
		return
	}

	c.Data(http.StatusOK, "text/css; charset=utf-8", data)
}

func (s *Server) handleJSFiles(c *gin.Context) {
	filename := c.Param("filename")
	fullPath := "web/js/" + filename

	data, err := webFS.ReadFile(fullPath)
	if err != nil {
		slog.Error("failed to read JS file", "path", fullPath, "error", err)
		c.Status(http.StatusNotFound)
		return
	}

	c.Data(http.StatusOK, "application/javascript; charset=utf-8", data)
}

func (s *Server) handleAdminGetAccountLimits(c *gin.Context) {
	accountID := c.Param("id")

	limits, err := s.storage.GetAllRateLimits(c.Request.Context(), accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"limits": limits})
}

func (s *Server) handleAdminProviderStats(c *gin.Context) {
	providers := s.registry.List()
	result := make([]map[string]interface{}, 0, len(providers))

	for _, p := range providers {
		providerName := p.Name()
		accountCount := 0
		availableCount := 0
		circuitOpenCount := 0

		if pool, ok := s.accountPools[providerName]; ok {
			for _, acc := range pool.List() {
				accountCount++
				state := pool.GetState(acc.ID)
				if acc.IsEnabled && state.ConsecutiveFailures < domain.CircuitBreakerThreshold {
					availableCount++
				}
				if cb, ok := s.circuitBreakers[acc.ID]; ok && !cb.Allow() {
					circuitOpenCount++
				}
			}
		}

		result = append(result, map[string]interface{}{
			"name":               providerName,
			"api_base":           p.APIBase(),
			"models":             s.router.ListModels(),
			"account_count":      accountCount,
			"available_accounts": availableCount,
			"circuit_open_count": circuitOpenCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{"providers": result})
}

func (s *Server) handleAdminLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := fmt.Sscanf(l, "%d", &limit); err == nil && parsed == 1 {
			if limit > 500 {
				limit = 500
			}
		}
	}

	logs, err := s.storage.GetRecentLogs(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

func (s *Server) handleAdminVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    Version,
		"build_time": BuildTime,
	})
}

func (s *Server) handleAdminModelMapping(c *gin.Context) {
	mapping := make(map[string]string)
	for k, v := range s.config.ModelMapping {
		mapping[k] = v
	}
	c.JSON(http.StatusOK, gin.H{"model_mapping": mapping})
}

func (s *Server) handleAdminTimeSeries(c *gin.Context) {
	interval := c.DefaultQuery("interval", "hour")
	if interval != "hour" && interval != "day" {
		interval = "hour"
	}

	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := fmt.Sscanf(h, "%d", &hours); err == nil && parsed == 1 {
			if hours > 720 {
				hours = 720
			}
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	points, err := s.storage.GetRequestTimeSeries(c.Request.Context(), since, interval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"timeseries": points})
}

func (s *Server) handleAdminAllAccountStats(c *gin.Context) {
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := fmt.Sscanf(h, "%d", &hours); err == nil && parsed == 1 {
			if hours > 720 {
				hours = 720
			}
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := s.storage.GetAllAccountStats(c.Request.Context(), since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"account_stats": stats})
}

func (s *Server) handleAdminModelStats(c *gin.Context) {
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := fmt.Sscanf(h, "%d", &hours); err == nil && parsed == 1 {
			if hours > 720 {
				hours = 720
			}
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := s.storage.GetModelStats(c.Request.Context(), since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"model_stats": stats})
}

func (s *Server) handleAdminBatchAccountOperation(c *gin.Context) {
	var req struct {
		Action     string   `json:"action"`
		AccountIDs []string `json:"account_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.AccountIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no accounts specified"})
		return
	}

	successCount := 0
	for _, id := range req.AccountIDs {
		switch req.Action {
		case "enable":
			for _, p := range s.accountPools {
				if acc, err := p.Get(id); err == nil {
					p.SetEnabled(id, true)
					acc.IsEnabled = true
					s.storage.UpsertAccount(c.Request.Context(), acc)
					successCount++
					break
				}
			}
		case "disable":
			for _, p := range s.accountPools {
				if acc, err := p.Get(id); err == nil {
					p.SetEnabled(id, false)
					acc.IsEnabled = false
					s.storage.UpsertAccount(c.Request.Context(), acc)
					successCount++
					break
				}
			}
		case "reset":
			for _, p := range s.accountPools {
				if _, err := p.Get(id); err == nil {
					p.ResetFailures(id)
					if l, ok := s.limiters[id]; ok {
						l.Reset(c.Request.Context(), id)
					}
					successCount++
					break
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": successCount, "total": len(req.AccountIDs)})
}

func (s *Server) handleAdminLogDetail(c *gin.Context) {
	requestID := c.Param("id")
	log, err := s.storage.GetLogByID(c.Request.Context(), requestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if log == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "log not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"log": log})
}

func (s *Server) handleAdminAccountModelStats(c *gin.Context) {
	accountID := c.Param("id")
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsed, err := fmt.Sscanf(h, "%d", &hours); err == nil && parsed == 1 {
			if hours > 720 {
				hours = 720
			}
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := s.storage.GetAccountModelStats(c.Request.Context(), accountID, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"account_id": accountID, "model_stats": stats})
}

func (s *Server) handleAdminListAPIKeys(c *gin.Context) {
	keys, err := s.storage.ListAPIKeys(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		result = append(result, map[string]interface{}{
			"id":            k.ID,
			"name":          k.Name,
			"is_enabled":    k.IsEnabled,
			"created_at":    k.CreatedAt.Format(time.RFC3339),
			"last_used_at":  formatTime(k.LastUsedAt),
			"request_count": k.RequestCount,
			"expires_at":    formatTime(k.ExpiresAt),
		})
	}

	c.JSON(http.StatusOK, gin.H{"api_keys": result})
}

func (s *Server) handleAdminCreateAPIKey(c *gin.Context) {
	var req struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	key := utils.GenerateUUID()
	keyHash := utils.HashAPIKey(key)

	id, err := s.storage.CreateAPIKey(c.Request.Context(), keyHash, req.Name, req.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"key":     key,
		"name":    req.Name,
		"message": "API key created. Save the key now - it won't be shown again.",
	})
}

func (s *Server) handleAdminDeleteAPIKey(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := s.storage.DeleteAPIKey(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key deleted"})
}

func (s *Server) handleAdminToggleAPIKey(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := s.storage.ToggleAPIKey(c.Request.Context(), id, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key updated"})
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (s *Server) handleAdminExport(c *gin.Context) {
	exportType := c.Param("type")

	switch exportType {
	case "accounts":
		accounts, err := s.storage.GetAllAccountStats(c.Request.Context(), time.Now().Add(-24*time.Hour))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=accounts.csv")
		c.Header("Content-Type", "text/csv")
		c.Writer.WriteString("account_id,request_count,error_count,total_tokens,avg_latency_ms,success_rate\n")
		for _, a := range accounts {
			c.Writer.WriteString(fmt.Sprintf("%s,%d,%d,%d,%.2f,%.2f\n", a.AccountID, a.RequestCount, a.ErrorCount, a.TotalTokens, a.AvgLatencyMs, a.SuccessRate))
		}
	case "models":
		stats, err := s.storage.GetModelStats(c.Request.Context(), time.Now().Add(-24*time.Hour))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=models.csv")
		c.Header("Content-Type", "text/csv")
		c.Writer.WriteString("model,request_count,error_count,total_tokens,avg_ttft_ms,avg_latency_ms,success_rate\n")
		for _, m := range stats {
			c.Writer.WriteString(fmt.Sprintf("%s,%d,%d,%d,%.2f,%.2f,%.2f\n", m.Model, m.RequestCount, m.ErrorCount, m.TotalTokens, m.AvgTTFTMs, m.AvgLatencyMs, m.SuccessRate))
		}
	case "stats":
		c.Header("Content-Disposition", "attachment; filename=stats.csv")
		c.Header("Content-Type", "text/csv")
		c.Writer.WriteString("timestamp,total_requests,total_tokens,total_errors\n")
		points, _ := s.storage.GetRequestTimeSeries(c.Request.Context(), time.Now().Add(-24*time.Hour), "hour")
		for _, p := range points {
			c.Writer.WriteString(fmt.Sprintf("%s,%d,%d,%d\n", p.Timestamp.Format("2006-01-02 15:04"), p.Count, p.Tokens, p.Errors))
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid export type"})
	}
}

func init() {
	flag.CommandLine.Parse([]string{})
}
