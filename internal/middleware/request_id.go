package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/pkg/utils"
)

type RequestIDConfig struct {
	HeaderName        string
	GenerateIfMissing bool
}

func NewRequestIDConfig() *RequestIDConfig {
	return &RequestIDConfig{
		HeaderName:        "X-Request-ID",
		GenerateIfMissing: true,
	}
}

func RequestID(cfg *RequestIDConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(cfg.HeaderName)

		if requestID == "" && cfg.GenerateIfMissing {
			requestID = utils.GenerateRequestID()
		}

		c.Set("request_id", requestID)
		c.Header(cfg.HeaderName, requestID)

		c.Next()
	}
}
