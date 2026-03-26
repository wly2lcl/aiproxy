package handler

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wangluyao/aiproxy/internal/router"
)

type ModelsHandler struct {
	router *router.Router
}

func NewModelsHandler(r *router.Router) *ModelsHandler {
	return &ModelsHandler{
		router: r,
	}
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

func (h *ModelsHandler) Handle(c *gin.Context) {
	models := h.router.ListModels()
	sort.Strings(models)

	data := make([]Model, 0, len(models))
	now := time.Now().Unix()
	for _, m := range models {
		data = append(data, Model{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "openai",
		})
	}

	c.JSON(http.StatusOK, ModelsResponse{
		Object: "list",
		Data:   data,
	})
}
