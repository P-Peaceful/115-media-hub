// settings.go：提供安全设置的 HTTP 读取与更新，将认证身份、凭据修改和会话期限提交给认证服务。

package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

// securitySettings 是安全设置的只读响应，仅包含用户名和会话期限。
type securitySettings struct {
	Username            string `json:"username"`
	SessionLifetimeDays int    `json:"session_lifetime_days"`
}

// security 读取并返回管理员用户名与会话期限，不暴露密码或其他内部状态。
func (h *authenticationHandler) security(c *gin.Context) {
	administrator, settings, err := h.service.SecuritySettings(c.Request.Context())
	if err != nil {
		h.writeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, securitySettings{administrator.Username, settings.SessionLifetimeDays})
}

// credentials 校验凭据修改请求，携带当前身份更新用户名和密码，成功后返回安全设置。
func (h *authenticationHandler) credentials(c *gin.Context) {
	var body struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeAuthBody(c, &body, []string{"username", "current_password", "new_password"}) {
		return
	}
	identity := c.MustGet("auth_identity").(domain.Identity)
	if err := h.service.UpdateCredentials(c.Request.Context(), identity, body.Username, body.CurrentPassword, body.NewPassword, c.GetString("request_id")); err != nil {
		h.writeFailure(c, err)
		return
	}
	h.security(c)
}

// sessionSettings 校验并更新会话期限，成功后返回当前安全设置。
func (h *authenticationHandler) sessionSettings(c *gin.Context) {
	var body struct {
		SessionLifetimeDays int `json:"session_lifetime_days"`
	}
	if !decodeAuthBody(c, &body, []string{"session_lifetime_days"}) {
		return
	}
	identity := c.MustGet("auth_identity").(domain.Identity)
	if err := h.service.UpdateSessionLifetime(c.Request.Context(), identity, body.SessionLifetimeDays, c.GetString("request_id")); err != nil {
		h.writeFailure(c, err)
		return
	}
	h.security(c)
}
