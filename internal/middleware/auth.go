package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/storage"
	"github.com/wangluyao/aiproxy/pkg/utils"
)

type APIKeyValidator interface {
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*storage.APIKey, error)
	UpdateAPIKeyUsage(ctx context.Context, keyHash string) error
}

type AuthConfig struct {
	Enabled    bool
	APIKeys    map[string]bool
	HeaderName string
	KeyPrefix  string
	Storage    APIKeyValidator
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

		if cfg.APIKeys[key] {
			c.Set("api_key", key)
			c.Next()
			return
		}

		if cfg.Storage != nil {
			keyHash := utils.HashAPIKey(key)
			apiKey, err := cfg.Storage.GetAPIKeyByHash(c.Request.Context(), keyHash)
			if err == nil && apiKey != nil && apiKey.IsEnabled {
				c.Set("api_key", key)
				c.Set("api_key_id", apiKey.ID)
				c.Set("api_key_name", apiKey.Name)
				c.Set("api_key_hash", keyHash)
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		c.Abort()
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
