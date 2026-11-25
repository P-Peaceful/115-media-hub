// provider115.go：暴露 115 单账号授权与目录浏览接口，将会话绑定、分页查询和上游错误转换为管理 API。

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

// providerHandler 将 115 服务的授权和只读文件能力转换为管理接口。
type providerHandler struct{ service *provider115.Service }

// register 注册 115 账号详情、二维码授权和只读目录浏览路由。
func (h *providerHandler) register(router *gin.Engine) {
	router.GET("/api/v1/cloud-drive/files", h.files)
	router.GET("/api/v1/settings/115", h.account)
	router.GET("/api/v1/settings/115/details", h.details)
	router.DELETE("/api/v1/settings/115", h.remove)
	router.POST("/api/v1/settings/115/qr", h.start)
	router.POST("/api/v1/settings/115/qr/:id/poll", h.poll)
	router.DELETE("/api/v1/settings/115/qr/:id", h.cancel)
}

// failure 将失效二维码映射为不存在，其余上游故障返回统一安全提示。
func (h *providerHandler) failure(c *gin.Context, err error) {
	if errors.Is(err, provider115.ErrNotFound) {
		writeError(c, http.StatusNotFound, generated.NotFound, "二维码已失效，请重新获取")
		return
	}
	writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "115 服务暂时不可用，请稍后重试")
}

// providerIdentity 读取管理边界已经校验并注入的管理员会话身份。
func providerIdentity(c *gin.Context) domain.Identity {
	return c.MustGet("auth_identity").(domain.Identity)
}

// account 返回已保存账号的公开状态，不返回网盘凭据。
func (h *providerHandler) account(c *gin.Context) {
	account, err := h.service.Account(c.Request.Context())
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, account)
}

// start 为当前管理员会话创建二维码授权流程并返回公开结果。
func (h *providerHandler) start(c *gin.Context) {
	result, err := h.service.Start(c.Request.Context(), providerIdentity(c).Session.TokenHash, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// poll 以当前会话和管理员身份轮询二维码，并由服务处理授权确认与凭据保存。
func (h *providerHandler) poll(c *gin.Context) {
	identity := providerIdentity(c)
	result, err := h.service.Poll(c.Request.Context(), identity.Session.TokenHash, c.Param("id"), identity.Administrator.ID, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// cancel 取消当前会话持有的指定二维码授权流程。
func (h *providerHandler) cancel(c *gin.Context) {
	err := h.service.Cancel(c.Request.Context(), providerIdentity(c).Session.TokenHash, c.Param("id"), c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// remove 携带当前管理员身份解除 115 账号绑定，由服务处理凭据删除和审计。
func (h *providerHandler) remove(c *gin.Context) {
	identity := providerIdentity(c)
	if err := h.service.Remove(c.Request.Context(), identity.Session.TokenHash, identity.Administrator.ID, c.GetString("request_id")); err != nil {
		h.failure(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// details 读取 115 账号与容量详情，将不可用状态转换为安全响应。
func (h *providerHandler) details(c *gin.Context) {
	details, err := h.service.Details(c.Request.Context(), c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(http.StatusOK, details)
}

// files 校验页码与游标互斥关系，读取目录分页并区分参数、授权、目录不存在和上游故障。
func (h *providerHandler) files(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "无效的分页参数")
		return
	}
	page := int64(0)
	if rawPage, ok := c.GetQuery("page"); ok {
		page, err = strconv.ParseInt(rawPage, 10, 64)
		_, hasCursor := c.GetQuery("cursor")
		if err != nil || page < 1 || hasCursor {
			writeError(c, http.StatusBadRequest, generated.ValidationFailed, "无效的分页参数")
			return
		}
	}
	query := provider115.FileQuery{DirectoryID: c.DefaultQuery("directory_id", "0"), Limit: limit, Cursor: c.Query("cursor"), Page: page, Sort: c.DefaultQuery("sort", "name"), Order: c.DefaultQuery("order", "asc"), Type: c.DefaultQuery("type", "all")}
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
