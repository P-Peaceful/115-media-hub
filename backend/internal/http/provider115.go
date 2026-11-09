package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

type providerHandler struct{ service *provider115.Service }

func (h *providerHandler) register(router *gin.Engine) {
	router.GET("/api/v1/cloud-drive/files", h.files)
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

func (h *providerHandler) files(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "无效的分页参数")
		return
	}
	query := provider115.FileQuery{DirectoryID: c.DefaultQuery("directory_id", "0"), Limit: limit, Cursor: c.Query("cursor"), Sort: c.DefaultQuery("sort", "name"), Order: c.DefaultQuery("order", "asc"), Type: c.DefaultQuery("type", "all")}
	result, err := h.service.Files(c.Request.Context(), query, c.GetString("request_id"))
	switch {
	case errors.Is(err, provider115.ErrInvalidQuery):
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "无效的目录查询参数")
	case errors.Is(err, provider115.ErrUnconfigured):
		writeError(c, http.StatusConflict, generated.ProviderNotConfigured, "请先授权 115 网盘")
	case errors.Is(err, provider115.ErrInvalidAuthorization):
		writeError(c, http.StatusConflict, generated.ProviderAuthInvalid, "115 网盘授权已失效")
	case errors.Is(err, provider115.ErrDirectoryNotFound):
		writeError(c, http.StatusNotFound, generated.NotFound, "目录不存在")
	case err != nil:
		h.failure(c, err)
	default:
		c.JSON(http.StatusOK, result)
	}
}
