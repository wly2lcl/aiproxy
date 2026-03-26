package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthConfig struct {
	Enabled    bool
	APIKeys    map[string]bool
	HeaderName string
	KeyPrefix  string
}

func NewAuthConfig() *AuthConfig {
	return &AuthConfig{
		Enabled:    false,
		APIKeys:    make(map[string]bool),
		HeaderName: "Authorization",
		KeyPrefix:  "Bearer ",
	}
}

func Auth(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		authHeader := c.GetHeader(cfg.HeaderName)
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, cfg.KeyPrefix) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		key := strings.TrimPrefix(authHeader, cfg.KeyPrefix)
		if !cfg.APIKeys[key] {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			c.Abort()
			return
		}

		c.Set("api_key", key)
		c.Next()
	}
}
