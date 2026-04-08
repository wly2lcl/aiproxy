package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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

type ChatConfig struct {
	Pool                *pool.Pool
	Router              *router.Router
	Proxy               *proxy.Proxy
	Limiters            map[string]*limiter.CompositeLimiter
	Collector           *stats.Collector
	MaxResponseBodySize int64
	Logger              interface {
		Info(msg string, fields ...interface{})
		Error(msg string, fields ...interface{})
	}
}

type ChatHandler struct {
	pool                *pool.Pool
	router              *router.Router
	proxy               *proxy.Proxy
	limiters            map[string]*limiter.CompositeLimiter
	collector           *stats.Collector
	logger              Logger
	selector            *pool.WeightedRoundRobin
	maxResponseBodySize int64
}

var ErrChatConfigRequired = domain.NewDomainError("chat_config_required", "chat config is required")

func NewChatHandler(cfg *ChatConfig) (*ChatHandler, error) {
	if cfg == nil {
		return nil, ErrChatConfigRequired
	}

	var selector *pool.WeightedRoundRobin
	if cfg.Pool != nil && cfg.Limiters != nil {
		selector = pool.NewWeightedRoundRobin(cfg.Pool, cfg.Limiters)
	}

	maxResponseBodySize := cfg.MaxResponseBodySize
	if maxResponseBodySize <= 0 {
		maxResponseBodySize = 50 * 1024 * 1024
	}

	return &ChatHandler{
		pool:                cfg.Pool,
		router:              cfg.Router,
		proxy:               cfg.Proxy,
		limiters:            cfg.Limiters,
		collector:           cfg.Collector,
		logger:              cfg.Logger,
		selector:            selector,
		maxResponseBodySize: maxResponseBodySize,
	}, nil
}

func (h *ChatHandler) Handle(c *gin.Context) {
	startTime := time.Now()

	var req openai.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "invalid_request", "invalid request body: "+err.Error())
		return
	}

	if req.Model == "" {
		h.sendError(c, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}

	prov, err := h.router.Resolve(req.Model)
	if err != nil {
		h.sendError(c, http.StatusServiceUnavailable, "provider_not_found", "no provider available for model: "+req.Model)
		return
	}

	provHeader := c.GetHeader("X-Provider")
	if provHeader != "" {
		prov, err = h.router.ResolveByHeader(req.Model, provHeader)
		if err != nil {
			h.sendError(c, http.StatusServiceUnavailable, "provider_not_found", "provider not found: "+provHeader)
			return
		}
	}

	ctx := c.Request.Context()

	const maxAccountRetries = 3
	var account *domain.Account

	for retry := 0; retry < maxAccountRetries; retry++ {
		account, err = h.selectAccount(ctx, prov)
		if err != nil {
			if err == pool.ErrNoAvailableAccount {
				h.sendError(c, http.StatusServiceUnavailable, domain.ErrCodeNoAvailableAccount, "no available account")
				return
			}
			if domainErr, ok := err.(*domain.DomainError); ok && domainErr.Code == domain.ErrCodeRateLimitExceeded {
				c.Header("Retry-After", "60")
				h.sendError(c, http.StatusTooManyRequests, domain.ErrCodeRateLimitExceeded, "rate limit exceeded")
				return
			}
			h.sendError(c, http.StatusServiceUnavailable, "account_error", err.Error())
			return
		}

		// Check rate limit for selected account
		if h.limiters != nil {
			if limiter, ok := h.limiters[account.ID]; ok && limiter != nil {
				allowed, limitErr := limiter.Allow(ctx, account.ID)
				if limitErr != nil {
					h.sendError(c, http.StatusInternalServerError, "rate_limit_error", limitErr.Error())
					return
				}
				if !allowed {
					h.recordRateLimitHit(account.ID)
					// Try another account instead of returning error immediately
					continue
				}
			}
		}
		// Account is available, proceed
		break
	}

	if account == nil {
		c.Header("Retry-After", "60")
		h.sendError(c, http.StatusTooManyRequests, domain.ErrCodeRateLimitExceeded, "rate limit exceeded")
		return
	}

	bodyBytes, err := json.Marshal(&req)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "marshal_error", "failed to marshal request")
		return
	}

	upstreamReq, err := prov.TransformRequest(&req, account.APIKey)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "transform_error", "failed to transform request")
		return
	}

	upstreamReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	upstreamReq.ContentLength = int64(len(bodyBytes))

	ctx = context.WithValue(ctx, "account", account)
	ctx = context.WithValue(ctx, "provider", prov)
	upstreamReq = upstreamReq.WithContext(ctx)

	resp, err := h.proxy.Do(ctx, upstreamReq)
	if err != nil {
		h.pool.RecordFailure(account.ID)
		h.sendError(c, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		h.pool.RecordFailure(account.ID)
		h.forwardUpstreamError(c, resp)
		return
	}

	h.pool.RecordSuccess(account.ID)

	if req.Stream {
		h.handleStream(c, resp, account, prov, &req, startTime)
		return
	}

	h.handleNonStream(c, resp, account, prov, &req, startTime)
}

func (h *ChatHandler) selectAccount(ctx context.Context, prov provider.Provider) (*domain.Account, error) {
	if h.selector == nil {
		return nil, pool.ErrNoAvailableAccount
	}

	limits := []domain.LimitType{
		domain.LimitTypeRPM,
		domain.LimitTypeDaily,
	}

	return h.selector.Select(ctx, limits)
}

func (h *ChatHandler) handleStream(c *gin.Context, resp *http.Response, account *domain.Account, prov provider.Provider, req *openai.ChatCompletionRequest, startTime time.Time) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	streamHandler := proxy.NewStreamHandler(h.proxy)
	err := streamHandler.ServeStream(c.Writer, c.Request, resp, startTime)
	if err != nil {
		h.logger.Error("stream error", "error", err)
	}

	latency := time.Since(startTime)
	promptTokens, completionTokens, found := streamHandler.GetTokenExtractor().ExtractFromStream(nil)

	if found {
		totalTokens := promptTokens + completionTokens
		h.recordUsage(prov.Name(), req.Model, http.StatusOK, latency, totalTokens)
		if h.limiters != nil {
			if limiter, ok := h.limiters[account.ID]; ok && limiter != nil {
				_ = limiter.Record(c.Request.Context(), account.ID, totalTokens)
			}
		}
	} else {
		h.recordUsage(prov.Name(), req.Model, http.StatusOK, latency, 0)
	}
}

func (h *ChatHandler) handleNonStream(c *gin.Context, resp *http.Response, account *domain.Account, prov provider.Provider, req *openai.ChatCompletionRequest, startTime time.Time) {
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	limitedReader := io.LimitReader(resp.Body, h.maxResponseBodySize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		h.sendError(c, http.StatusBadGateway, "upstream_error", "failed to read upstream response: "+err.Error())
		return
	}

	if int64(len(bodyBytes)) > h.maxResponseBodySize {
		h.sendError(c, http.StatusBadGateway, "response_too_large", "upstream response exceeds maximum size limit")
		return
	}

	c.Status(resp.StatusCode)
	c.Writer.Write(bodyBytes)

	latency := time.Since(startTime)

	var chatResp openai.ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err == nil && chatResp.Usage != nil {
		h.recordUsage(prov.Name(), req.Model, resp.StatusCode, latency, chatResp.Usage.TotalTokens)
		if h.limiters != nil {
			if limiter, ok := h.limiters[account.ID]; ok && limiter != nil {
				_ = limiter.Record(c.Request.Context(), account.ID, chatResp.Usage.TotalTokens)
			}
		}
	} else {
		h.recordUsage(prov.Name(), req.Model, resp.StatusCode, latency, 0)
	}
}

func (h *ChatHandler) recordUsage(providerName, model string, status int, latency time.Duration, tokens int) {
	if h.collector != nil {
		h.collector.RecordRequest(providerName, model, status, latency, tokens)
	}
}

func (h *ChatHandler) recordRateLimitHit(accountID string) {
	if h.collector != nil {
		h.collector.RecordRateLimitHit(accountID, domain.LimitTypeRPM)
	}
}

func (h *ChatHandler) sendError(c *gin.Context, statusCode int, code, message string) {
	errResp := openai.ErrorResponse{
		Error: openai.ErrorDetail{
			Message: message,
			Type:    "invalid_request_error",
			Code:    code,
		},
	}

	c.JSON(statusCode, errResp)
}

func (h *ChatHandler) forwardUpstreamError(c *gin.Context, resp *http.Response) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		h.sendError(c, resp.StatusCode, "upstream_error", "failed to read upstream error")
		return
	}

	var errResp openai.ErrorResponse
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil {
		c.JSON(resp.StatusCode, errResp)
		return
	}

	h.sendError(c, resp.StatusCode, "upstream_error", string(bodyBytes))
}

type Logger interface {
	Info(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

type noopLogger struct{}

func (n *noopLogger) Info(msg string, fields ...interface{})  {}
func (n *noopLogger) Error(msg string, fields ...interface{}) {}

func NewNoopLogger() *noopLogger {
	return &noopLogger{}
}
