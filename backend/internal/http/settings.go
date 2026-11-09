package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/auth"
)

type securitySettings struct {
	Username            string `json:"username"`
	SessionLifetimeDays int    `json:"session_lifetime_days"`
}

func (h *authenticationHandler) security(c *gin.Context) {
	administrator, settings, err := h.service.SecuritySettings(c.Request.Context())
	if err != nil {
		h.writeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, securitySettings{administrator.Username, settings.SessionLifetimeDays})
}
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
