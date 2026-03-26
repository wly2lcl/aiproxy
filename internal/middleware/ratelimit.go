package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/limiter"
)

type RateLimitConfig struct {
	Limiter limiter.Limiter
	KeyFunc func(*gin.Context) string
}

func DefaultRateLimitKeyFunc(c *gin.Context) string {
	if key, exists := c.Get("api_key"); exists {
		return key.(string)
	}
	return c.ClientIP()
}

func RateLimit(cfg *RateLimitConfig) gin.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = DefaultRateLimitKeyFunc
	}

	return func(c *gin.Context) {
		if cfg.Limiter == nil {
			c.Next()
			return
		}

		key := cfg.KeyFunc(c)
		allowed, err := cfg.Limiter.Allow(context.Background(), key)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "rate limit check failed"})
			c.Abort()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
