package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"log/slog"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				requestID, _ := c.Get("request_id")

				slog.Error("panic recovered",
					slog.Any("error", err),
					slog.String("stack", string(stack)),
					slog.String("request_id", fmt.Sprintf("%v", requestID)),
					slog.String("path", c.Request.URL.Path),
					slog.String("method", c.Request.Method),
				)

				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
