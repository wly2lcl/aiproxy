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
	Pool      *pool.Pool
	Router    *router.Router
	Proxy     *proxy.Proxy
	Limiter   *limiter.CompositeLimiter
	Collector *stats.Collector
	Logger    interface {
		Info(msg string, fields ...interface{})
		Error(msg string, fields ...interface{})
	}
}

type ChatHandler struct {
	pool           *pool.Pool
	router         *router.Router
	proxy          *proxy.Proxy
	limiter        *limiter.CompositeLimiter
	collector      *stats.Collector
	logger         Logger
	selector       *pool.WeightedRoundRobin
	streamHandler  *proxy.StreamHandler
	tokenExtractor *proxy.TokenExtractor
}

func NewChatHandler(cfg *ChatConfig) *ChatHandler {
	if cfg == nil {
		panic("chat config is required")
	}

	var selector *pool.WeightedRoundRobin
	if cfg.Pool != nil && cfg.Limiter != nil {
		selector = pool.NewWeightedRoundRobin(cfg.Pool, cfg.Limiter)
	}

	return &ChatHandler{
		pool:          cfg.Pool,
		router:        cfg.Router,
		proxy:         cfg.Proxy,
		limiter:       cfg.Limiter,
		collector:     cfg.Collector,
		logger:        cfg.Logger,
		selector:      selector,
		streamHandler: proxy.NewStreamHandler(cfg.Proxy),
	}
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

	account, err := h.selectAccount(ctx, prov)
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

	if h.limiter != nil {
		allowed, limitErr := h.limiter.Allow(ctx, account.ID)
		if limitErr != nil {
			h.sendError(c, http.StatusInternalServerError, "rate_limit_error", limitErr.Error())
			return
		}
		if !allowed {
			h.recordRateLimitHit(account.ID)
			c.Header("Retry-After", "60")
			h.sendError(c, http.StatusTooManyRequests, domain.ErrCodeRateLimitExceeded, "rate limit exceeded")
			return
		}
	}

	bodyBytes, err := json.Marshal(&req)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "marshal_error", "failed to marshal request")
		return
	}

	upstreamReq, err := prov.TransformRequest(&req, account.APIKeyHash)
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

	err := h.streamHandler.ServeStream(c.Writer, c.Request, resp)
	if err != nil {
		h.logger.Error("stream error", "error", err)
	}

	latency := time.Since(startTime)
	promptTokens, completionTokens, found := h.extractStreamUsage()

	if found {
		totalTokens := promptTokens + completionTokens
		h.recordUsage(prov.Name(), req.Model, http.StatusOK, latency, totalTokens)
		if h.limiter != nil {
			_ = h.limiter.Record(c.Request.Context(), account.ID, totalTokens)
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "read_error", "failed to read response")
		return
	}

	c.Status(resp.StatusCode)
	c.Writer.Write(bodyBytes)

	latency := time.Since(startTime)

	var chatResp openai.ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err == nil && chatResp.Usage != nil {
		h.recordUsage(prov.Name(), req.Model, resp.StatusCode, latency, chatResp.Usage.TotalTokens)
		if h.limiter != nil {
			_ = h.limiter.Record(c.Request.Context(), account.ID, chatResp.Usage.TotalTokens)
		}
	} else {
		h.recordUsage(prov.Name(), req.Model, resp.StatusCode, latency, 0)
	}
}

func (h *ChatHandler) extractStreamUsage() (promptTokens, completionTokens int, found bool) {
	if h.streamHandler != nil {
		return h.streamHandler.GetTokenExtractor().ExtractFromStream(nil)
	}
	return 0, 0, false
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
