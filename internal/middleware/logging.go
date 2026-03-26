package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"
)

type LoggingConfig struct {
	Level               string
	IncludeRequestBody  bool
	IncludeResponseBody bool
}

func NewLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:               "info",
		IncludeRequestBody:  false,
		IncludeResponseBody: false,
	}
}

func Logging(cfg *LoggingConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		blw := &bodyLogWriter{
			ResponseWriter: c.Writer,
			body:           make([]byte, 0),
		}
		if cfg.IncludeResponseBody {
			c.Writer = blw
		}

		var requestBody []byte
		if cfg.IncludeRequestBody && c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		requestID, _ := c.Get("request_id")

		args := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency", latency,
		}

		if requestID != nil {
			args = append(args, "request_id", requestID)
		}

		if cfg.IncludeRequestBody && len(requestBody) > 0 {
			args = append(args, "request_body", string(requestBody))
		}

		if cfg.IncludeResponseBody {
			args = append(args, "response_body", string(blw.body))
		}

		logger := slog.Default()
		switch {
		case status >= 500:
			logger.Error("request completed", args...)
		case status >= 400:
			logger.Warn("request completed", args...)
		default:
			logger.Info("request completed", args...)
		}
	}
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body []byte
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}
