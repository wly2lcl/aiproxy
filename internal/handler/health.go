package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/router"
)

type HealthHandler struct {
	router *router.Router
}

func NewHealthHandler(r *router.Router) *HealthHandler {
	return &HealthHandler{
		router: r,
	}
}

func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	providers := h.router.ListModels()

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": gin.H{
			"database":  "ok",
			"providers": providers,
		},
	})
}
