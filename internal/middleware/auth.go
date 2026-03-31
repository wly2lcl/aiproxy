package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/storage"
	"github.com/wangluyao/aiproxy/pkg/utils"
)

type APIKeyValidator interface {
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*storage.APIKey, error)
	UpdateAPIKeyUsage(ctx context.Context, keyHash string) error
}

type AuthConfig struct {
	Enabled              bool
	APIKeys              map[string]bool
	HeaderName           string
	KeyPrefix            string
	Storage              APIKeyValidator
	AuthFailureRateLimit int
	AuthFailureWindow    time.Duration
	AuthFailureBlockTime time.Duration
}

type authFailureTracker struct {
	mu         sync.RWMutex
	failures   map[string]*authFailureRecord
	blockList  map[string]time.Time
	lastClean  time.Time
	cleanEvery time.Duration
}

type authFailureRecord struct {
	count     int
	firstSeen time.Time
}

var globalAuthTracker = &authFailureTracker{
	failures:   make(map[string]*authFailureRecord),
	blockList:  make(map[string]time.Time),
	lastClean:  time.Now(),
	cleanEvery: 5 * time.Minute,
}

func NewAuthConfig() *AuthConfig {
	return &AuthConfig{
		Enabled:              false,
		APIKeys:              make(map[string]bool),
		HeaderName:           "Authorization",
		KeyPrefix:            "Bearer ",
		AuthFailureRateLimit: 5,
		AuthFailureWindow:    15 * time.Minute,
		AuthFailureBlockTime: 30 * time.Minute,
	}
}

func (t *authFailureTracker) RecordFailure(ip string, window time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	record, exists := t.failures[ip]
	if !exists || now.Sub(record.firstSeen) > window {
		t.failures[ip] = &authFailureRecord{count: 1, firstSeen: now}
	} else {
		record.count++
	}

	t.maybeCleanLocked(now, window)
}

func (t *authFailureTracker) ClearFailure(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, ip)
}

func (t *authFailureTracker) IsBlocked(ip string, blockTime time.Duration) bool {
	t.mu.RLock()
	blockedAt, exists := t.blockList[ip]
	if !exists {
		t.mu.RUnlock()
		return false
	}
	blocked := time.Since(blockedAt) < blockTime
	t.mu.RUnlock()

	if !blocked {
		t.mu.Lock()
		delete(t.blockList, ip)
		t.mu.Unlock()
	}

	return blocked
}

func (t *authFailureTracker) Block(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockList[ip] = time.Now()
}

func (t *authFailureTracker) GetFailureCount(ip string, window time.Duration) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	record, exists := t.failures[ip]
	if !exists || time.Since(record.firstSeen) > window {
		return 0
	}
	return record.count
}

func (t *authFailureTracker) maybeCleanLocked(now time.Time, window time.Duration) {
	if now.Sub(t.lastClean) < t.cleanEvery {
		return
	}
	t.lastClean = now

	for ip, record := range t.failures {
		if now.Sub(record.firstSeen) > window {
			delete(t.failures, ip)
		}
	}
}

func Auth(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		clientIP := c.ClientIP()

		if cfg.AuthFailureRateLimit > 0 && globalAuthTracker.IsBlocked(clientIP, cfg.AuthFailureBlockTime) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "too many authentication failures, please try again later",
			})
			c.Abort()
			return
		}

		authHeader := c.GetHeader(cfg.HeaderName)
		if authHeader == "" {
			cfg.recordFailureIfNeeded(clientIP)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, cfg.KeyPrefix) {
			cfg.recordFailureIfNeeded(clientIP)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		key := strings.TrimPrefix(authHeader, cfg.KeyPrefix)

		if cfg.APIKeys[key] {
			globalAuthTracker.ClearFailure(clientIP)
			c.Set("api_key", key)
			c.Next()
			return
		}

		if cfg.Storage != nil {
			keyHash := utils.HashAPIKey(key)
			apiKey, err := cfg.Storage.GetAPIKeyByHash(c.Request.Context(), keyHash)
			if err == nil && apiKey != nil && apiKey.IsEnabled {
				globalAuthTracker.ClearFailure(clientIP)
				c.Set("api_key", key)
				c.Set("api_key_id", apiKey.ID)
				c.Set("api_key_name", apiKey.Name)
				c.Set("api_key_hash", keyHash)
				c.Next()
				return
			}
		}

		cfg.recordFailureIfNeeded(clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		c.Abort()
	}
}

func (cfg *AuthConfig) recordFailureIfNeeded(ip string) {
	if cfg.AuthFailureRateLimit > 0 {
		globalAuthTracker.RecordFailure(ip, cfg.AuthFailureWindow)
		if globalAuthTracker.GetFailureCount(ip, cfg.AuthFailureWindow) >= cfg.AuthFailureRateLimit {
			globalAuthTracker.Block(ip)
		}
	}
}

func UpdateAPIKeyUsage(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if cfg.Storage != nil {
			if keyHash, exists := c.Get("api_key_hash"); exists {
				if hash, ok := keyHash.(string); ok && hash != "" {
					_ = cfg.Storage.UpdateAPIKeyUsage(c.Request.Context(), hash)
				}
			}
		}
	}
}
