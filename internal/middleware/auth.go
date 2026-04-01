package middleware

import (
	"context"
	"crypto/subtle"
	"log/slog"
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

type SecurityStorage interface {
	BlockIP(ctx context.Context, ip, reason string) error
	UnblockIP(ctx context.Context, ip string) error
	GetBlockedIPs(ctx context.Context) ([]storage.BlockedIP, error)
	RecordAuthFailure(ctx context.Context, ip string) error
	ClearAuthFailure(ctx context.Context, ip string) error
	GetAuthFailures(ctx context.Context) ([]storage.AuthFailure, error)
}

type AuthConfig struct {
	Enabled              bool
	APIKeys              map[string]bool
	HeaderName           string
	KeyPrefix            string
	Storage              APIKeyValidator
	SecurityStore        SecurityStorage
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

func (cfg *AuthConfig) validateStaticKey(key string) bool {
	for storedKey := range cfg.APIKeys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) == 1 {
			return true
		}
	}
	return false
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

func (t *authFailureTracker) ClearBlock(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.blockList, ip)
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

func (t *authFailureTracker) LoadFromDB(ctx context.Context, store SecurityStorage, blockTime time.Duration) error {
	blockedIPs, err := store.GetBlockedIPs(ctx)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for _, ip := range blockedIPs {
		elapsed := now.Sub(ip.BlockedAt)
		if elapsed < blockTime {
			t.blockList[ip.IP] = ip.BlockedAt
		}
	}

	failures, err := store.GetAuthFailures(ctx)
	if err != nil {
		return err
	}

	for _, f := range failures {
		if now.Sub(f.FirstSeen) < blockTime {
			t.failures[f.IP] = &authFailureRecord{
				count:     f.FailureCount,
				firstSeen: f.FirstSeen,
			}
		}
	}

	slog.Info("loaded security data from database", "blocked_ips", len(t.blockList), "failures", len(t.failures))
	return nil
}

type BlockedIPInfo struct {
	IP            string    `json:"ip"`
	BlockedAt     time.Time `json:"blocked_at"`
	RemainingTime int       `json:"remaining_time_seconds"`
}

type AuthFailureInfo struct {
	IP        string    `json:"ip"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
}

func GetBlockedIPs(blockTime time.Duration) []BlockedIPInfo {
	globalAuthTracker.mu.RLock()
	defer globalAuthTracker.mu.RUnlock()

	now := time.Now()
	var result []BlockedIPInfo
	for ip, blockedAt := range globalAuthTracker.blockList {
		elapsed := now.Sub(blockedAt)
		if elapsed < blockTime {
			remaining := int((blockTime - elapsed).Seconds())
			result = append(result, BlockedIPInfo{
				IP:            ip,
				BlockedAt:     blockedAt,
				RemainingTime: remaining,
			})
		}
	}
	return result
}

func GetAuthFailures(window time.Duration) []AuthFailureInfo {
	globalAuthTracker.mu.RLock()
	defer globalAuthTracker.mu.RUnlock()

	now := time.Now()
	var result []AuthFailureInfo
	for ip, record := range globalAuthTracker.failures {
		if now.Sub(record.firstSeen) <= window {
			result = append(result, AuthFailureInfo{
				IP:        ip,
				Count:     record.count,
				FirstSeen: record.firstSeen,
			})
		}
	}
	return result
}

func UnblockIP(ip string) {
	globalAuthTracker.ClearBlock(ip)
}

func InitSecurityFromDB(ctx context.Context, store SecurityStorage, blockTime time.Duration) error {
	return globalAuthTracker.LoadFromDB(ctx, store, blockTime)
}

func Auth(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		clientIP := c.ClientIP()

		authHeader := c.GetHeader(cfg.HeaderName)
		if authHeader == "" {
			cfg.recordFailureIfNeeded(c.Request.Context(), clientIP)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, cfg.KeyPrefix) {
			cfg.recordFailureIfNeeded(c.Request.Context(), clientIP)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		key := strings.TrimPrefix(authHeader, cfg.KeyPrefix)

		// Use constant-time comparison for static API keys to prevent timing attacks
		if cfg.validateStaticKey(key) {
			globalAuthTracker.ClearBlock(clientIP)
			if cfg.SecurityStore != nil {
				cfg.SecurityStore.UnblockIP(c.Request.Context(), clientIP)
				cfg.SecurityStore.ClearAuthFailure(c.Request.Context(), clientIP)
			}
			c.Set("api_key_hash", utils.HashAPIKey(key))
			c.Next()
			return
		}

		if cfg.Storage != nil {
			keyHash := utils.HashAPIKey(key)
			apiKey, err := cfg.Storage.GetAPIKeyByHash(c.Request.Context(), keyHash)
			if err == nil && apiKey != nil && apiKey.IsEnabled {
				globalAuthTracker.ClearBlock(clientIP)
				if cfg.SecurityStore != nil {
					cfg.SecurityStore.UnblockIP(c.Request.Context(), clientIP)
					cfg.SecurityStore.ClearAuthFailure(c.Request.Context(), clientIP)
				}
				c.Set("api_key_id", apiKey.ID)
				c.Set("api_key_name", apiKey.Name)
				c.Set("api_key_hash", keyHash)
				c.Next()
				return
			}
		}

		if cfg.AuthFailureRateLimit > 0 && globalAuthTracker.IsBlocked(clientIP, cfg.AuthFailureBlockTime) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "too many authentication failures, please try again later",
			})
			c.Abort()
			return
		}

		cfg.recordFailureIfNeeded(c.Request.Context(), clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		c.Abort()
	}
}

func (cfg *AuthConfig) recordFailureIfNeeded(ctx context.Context, ip string) {
	if cfg.AuthFailureRateLimit <= 0 {
		return
	}

	globalAuthTracker.RecordFailure(ip, cfg.AuthFailureWindow)

	if cfg.SecurityStore != nil {
		cfg.SecurityStore.RecordAuthFailure(ctx, ip)
	}

	count := globalAuthTracker.GetFailureCount(ip, cfg.AuthFailureWindow)
	if count >= cfg.AuthFailureRateLimit {
		globalAuthTracker.Block(ip)
		if cfg.SecurityStore != nil {
			cfg.SecurityStore.BlockIP(ctx, ip, "exceeded auth failure limit")
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
