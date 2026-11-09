package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

type providerHandler struct{ service *provider115.Service }

func (h *providerHandler) register(router *gin.Engine) {
	router.GET("/api/v1/settings/115", h.account)
	router.GET("/api/v1/settings/115/details", h.details)
	router.DELETE("/api/v1/settings/115", h.remove)
	router.POST("/api/v1/settings/115/qr", h.start)
	router.POST("/api/v1/settings/115/qr/:id/poll", h.poll)
	router.DELETE("/api/v1/settings/115/qr/:id", h.cancel)
}
func (h *providerHandler) failure(c *gin.Context, err error) {
	if errors.Is(err, provider115.ErrNotFound) {
		writeError(c, http.StatusNotFound, generated.NotFound, "二维码已失效，请重新获取")
		return
	}
	writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "115 服务暂时不可用，请稍后重试")
}
func providerIdentity(c *gin.Context) domain.Identity {
	return c.MustGet("auth_identity").(domain.Identity)
}
func (h *providerHandler) account(c *gin.Context) {
	account, err := h.service.Account(c.Request.Context())
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}
func (h *providerHandler) start(c *gin.Context) {
	result, err := h.service.Start(c.Request.Context(), providerIdentity(c).Session.TokenHash, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (h *providerHandler) poll(c *gin.Context) {
	identity := providerIdentity(c)
	result, err := h.service.Poll(c.Request.Context(), identity.Session.TokenHash, c.Param("id"), identity.Administrator.ID, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (h *providerHandler) cancel(c *gin.Context) {
	err := h.service.Cancel(c.Request.Context(), providerIdentity(c).Session.TokenHash, c.Param("id"), c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *providerHandler) remove(c *gin.Context) {
	identity := providerIdentity(c)
	if err := h.service.Remove(c.Request.Context(), identity.Session.TokenHash, identity.Administrator.ID, c.GetString("request_id")); err != nil {
		h.failure(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *providerHandler) details(c *gin.Context) {
	details, err := h.service.Details(c.Request.Context(), c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, details)
}
