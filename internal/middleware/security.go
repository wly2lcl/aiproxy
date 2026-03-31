package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/config"
)

func SecurityHeaders(cfg *config.SecurityHeadersConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		if cfg.XFrameOptions != "" {
			c.Header("X-Frame-Options", cfg.XFrameOptions)
		}
		if cfg.XContentTypeOptions != "" {
			c.Header("X-Content-Type-Options", cfg.XContentTypeOptions)
		}
		if cfg.XXSSProtection != "" {
			c.Header("X-XSS-Protection", cfg.XXSSProtection)
		}
		if cfg.ContentSecurityPolicy != "" {
			c.Header("Content-Security-Policy", cfg.ContentSecurityPolicy)
		}
		if cfg.StrictTransportSecurity != "" {
			c.Header("Strict-Transport-Security", cfg.StrictTransportSecurity)
		}
		if cfg.ReferrerPolicy != "" {
			c.Header("Referrer-Policy", cfg.ReferrerPolicy)
		}

		c.Next()
	}
}

func CORS(cfg *config.CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowed := false
		for _, o := range cfg.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if !allowed {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)

		if len(cfg.AllowedMethods) > 0 {
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
		}

		if len(cfg.AllowedHeaders) > 0 {
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
		}

		if len(cfg.ExposedHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", "))
		}

		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if cfg.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
