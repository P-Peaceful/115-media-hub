package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
)

type Health struct{ Ping func(context.Context) error }

var _ generated.ServerInterface = Health{}

func (h Health) GetLiveness(c *gin.Context) {
	c.JSON(200, generated.Health{Status: generated.Ok})
}
func (h Health) GetReadiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.Ping(ctx); err != nil {
		c.JSON(503, generated.Health{Status: generated.Unavailable})
		return
	}
	c.JSON(200, generated.Health{Status: generated.Ok})
}
