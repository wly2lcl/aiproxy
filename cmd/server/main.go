package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

type Server struct {
	config              *config.Config
	storage             storage.Storage
	registry            *provider.Registry
	router              *router.Router
	proxy               *proxy.Proxy
	streamHandler       *proxy.StreamHandler
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

	store, err := storage.NewSQLite(dbPath)
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
			p = provider.NewOpenAIProvider(pc.APIKeys[0].Key, nil, pc.Models)
		case "openrouter":
			p = provider.NewOpenRouterProvider(pc.APIKeys[0].Key, nil, pc.Models, pc.Headers)
		case "groq":
			p = provider.NewGroqProvider(pc.APIKeys[0].Key, nil, pc.Models)
		default:
			openaiProvider := provider.NewOpenAIProvider(pc.APIKeys[0].Key, nil, pc.Models)
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
			ID:         utils.GenerateUUID(),
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
		limiters = append(limiters, limiter.NewWindow5h(s.storage, *limits.Window5h))
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
	s.statsReporter = stats.NewReporter(s.statsCollector)
	slog.Info("initialized stats collector")
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

	s.streamHandler = proxy.NewStreamHandler(s.proxy)

	slog.Info("initialized proxy")
}

func (s *Server) initHTTPClient() {
	s.httpClient = &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       120 * time.Second,
			DisableCompression:    false,
			DisableKeepAlives:     false,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 5 * time.Minute,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	slog.Info("initialized HTTP client with connection pooling")
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
		Enabled:    s.config.Auth.Enabled,
		APIKeys:    make(map[string]bool),
		HeaderName: s.config.Auth.HeaderName,
		KeyPrefix:  s.config.Auth.KeyPrefix,
	}
	for _, key := range s.config.Auth.APIKeys {
		authConfig.APIKeys[key] = true
	}
	engine.Use(middleware.Auth(authConfig))

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

	adminAuth := &middleware.AuthConfig{
		Enabled:    len(s.config.Admin.APIKeys) > 0,
		APIKeys:    make(map[string]bool),
		HeaderName: "Authorization",
		KeyPrefix:  "Bearer ",
	}
	for _, key := range s.config.Admin.APIKeys {
		adminAuth.APIKeys[key] = true
	}
	engine.Use(middleware.Auth(adminAuth))

	engine.GET("/admin/accounts", s.handleAdminListAccounts)
	engine.GET("/admin/accounts/:id", s.handleAdminGetAccount)
	engine.POST("/admin/accounts", s.handleAdminCreateAccount)
	engine.PUT("/admin/accounts/:id", s.handleAdminUpdateAccount)
	engine.DELETE("/admin/accounts/:id", s.handleAdminDeleteAccount)
	engine.POST("/admin/accounts/:id/reset", s.handleAdminResetAccount)
	engine.GET("/admin/stats", s.handleAdminStats)
	engine.GET("/admin/providers", s.handleAdminListProviders)
	engine.POST("/admin/reload", s.handleAdminReload)
	engine.GET("/admin/health", s.handleHealth)

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

	s.executeRequest(c, req, prov, prov.Name(), startTime)
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
	req.Model = mappedModel

	retry := s.retries[providerName]

	var lastErr error

	maxAttempts := 1
	if retry != nil {
		maxAttempts = 3
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			delay := time.Second << time.Duration(attempt-2)
			select {
			case <-c.Request.Context().Done():
				return nil, nil, c.Request.Context().Err()
			case <-time.After(delay):
			}
			slog.Info("retrying request", "attempt", attempt, "provider", providerName, "account_id", account.ID[:8])
		}

		httpReq, err := prov.TransformRequest(req, account.APIKeyHash)
		if err != nil {
			return nil, nil, err
		}

		timeout := prov.GetTimeout(req.Stream)
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		resp, err := s.httpClient.Do(httpReq.WithContext(ctx))
		if err != nil {
			s.statsCollector.RecordError(providerName, mappedModel, "request_failed")
			s.recordAccountFailure(account.ID)
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			s.statsCollector.RecordError(providerName, mappedModel, "upstream_error")
			s.recordAccountFailure(account.ID)
			lastErr = fmt.Errorf("upstream returned status %d", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		if resp.StatusCode >= 400 {
			s.statsCollector.RecordError(providerName, mappedModel, "client_error")
			s.forwardUpstreamError(c, resp)
			return nil, nil, fmt.Errorf("client error: %d", resp.StatusCode)
		}

		s.recordAccountSuccess(account.ID)

		if req.Stream {
			s.handleStreamResponse(c, resp, account, providerName, req, startTime)
		} else {
			s.handleNonStreamResponse(c, resp, account, providerName, req, startTime)
		}

		return resp, account, nil
	}

	return nil, nil, lastErr
}

func (s *Server) selectAvailableAccount(selector *pool.WeightedRoundRobin, providerName string) (*domain.Account, error) {
	for {
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

	s.streamHandler.GetTokenExtractor().Reset()

	if err := s.streamHandler.ServeStream(c.Writer, c.Request, resp); err != nil {
		slog.Error("stream error", "error", err, "request_id", c.GetString("request_id"))
	}

	latency := time.Since(startTime)
	promptTokens, completionTokens, found := s.streamHandler.GetTokenExtractor().ExtractFromStream(nil)

	var totalTokens int
	if found {
		totalTokens = promptTokens + completionTokens
		s.statsCollector.RecordRequest(providerName, req.Model, http.StatusOK, latency, totalTokens)
		s.recordTokenUsage(account.ID, providerName, req.Model, promptTokens, completionTokens)
	} else {
		s.statsCollector.RecordRequest(providerName, req.Model, http.StatusOK, latency, 0)
	}

	slog.Info("stream completed", "provider", providerName, "model", req.Model, "tokens", totalTokens, "latency", latency)
}

func (s *Server) handleNonStreamResponse(c *gin.Context, resp *http.Response, account *domain.Account, providerName string, req *openai.ChatCompletionRequest, startTime time.Time) {
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

	slog.Info("request completed", "provider", providerName, "model", req.Model, "tokens", totalTokens, "latency", latency, "status", resp.StatusCode)
}

func (s *Server) forwardUpstreamError(c *gin.Context, resp *http.Response) {
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
	result := make([]map[string]interface{}, 0)

	for name, p := range s.accountPools {
		if providerName != "" && name != providerName {
			continue
		}
		for _, acc := range p.List() {
			result = append(result, map[string]interface{}{
				"id":          acc.ID,
				"provider_id": acc.ProviderID,
				"weight":      acc.Weight,
				"priority":    acc.Priority,
				"is_enabled":  acc.IsEnabled,
			})
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

func init() {
	flag.CommandLine.Parse([]string{})
}
