package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/connections"
)

type Connections struct{ Service *connections.Service }

func (h Connections) Get(c *gin.Context) {
	row, err := h.Service.Current(c.Request.Context())
	if err != nil {
		connectionError(c, err)
		return
	}
	resp := generated.ConnectionSummary{Configured: row != nil}
	if row != nil {
		dto := toConnection(row)
		resp.Connection = &dto
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, resp)
}
func (h Connections) Start(c *gin.Context) {
	var body generated.CreateQRCodeSessionRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"category": "invalid_request", "message": "invalid request"})
		return
	}
	s, err := h.Service.Start(c.Request.Context(), drive115.Device(body.Device))
	if err != nil {
		connectionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(201, toSession(s))
}
func (h Connections) Poll(c *gin.Context, id string) {
	s, err := h.Service.Poll(c.Request.Context(), id)
	if err != nil {
		connectionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, toSession(s))
}
func (h Connections) QR(c *gin.Context, id string) {
	data, err := h.Service.QRImage(id)
	if err != nil {
		connectionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(200, "image/png", data)
}
func (h Connections) Complete(c *gin.Context, id string) {
	row, err := h.Service.Complete(c.Request.Context(), id)
	if err != nil {
		connectionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, toConnection(row))
}
func (h Connections) Test(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	r, err := h.Service.Test(c.Request.Context())
	if err != nil {
		connectionError(c, err)
		return
	}
	caps := make([]generated.CapabilityEvidence, 0, len(r.Capabilities))
	for _, v := range r.Capabilities {
		caps = append(caps, generated.CapabilityEvidence{Name: generated.CapabilityEvidenceName(v.Name), Status: generated.CapabilityEvidenceStatus(v.Status)})
	}
	c.JSON(200, generated.ConnectionTestResult{Status: generated.ConnectionTestResultStatus(r.Status), Capabilities: caps, CheckedAt: r.CheckedAt, Message: optionalString(r.Message), UpstreamCode: optionalString(r.UpstreamCode)})
}
func (h Connections) RefreshProfile(c *gin.Context) {
	row, err := h.Service.RefreshProfile(c.Request.Context())
	if err != nil {
		connectionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, toConnection(row))
}
func toSession(v connections.Session) generated.QRCodeSession {
	return generated.QRCodeSession{SessionId: v.SessionID, Device: generated.QRCodeDevice(v.Device), ExpiresAt: v.ExpiresAt, Status: generated.QRCodeSessionStatus(v.Status), Message: optionalString(v.Message), UpstreamCode: optionalString(v.UpstreamCode)}
}
func toConnection(v *connections.Connection) generated.Connection {
	caps := make([]generated.CapabilityEvidence, 0, len(v.Capabilities))
	for _, c := range v.Capabilities {
		caps = append(caps, generated.CapabilityEvidence{Name: generated.CapabilityEvidenceName(c.Name), Status: generated.CapabilityEvidenceStatus(c.Status)})
	}
	var retrievedAt *time.Time
	if !v.Profile.RetrievedAt.IsZero() {
		value := v.Profile.RetrievedAt
		retrievedAt = &value
	}
	profile := generated.AccountProfile{
		AccountName:         v.Profile.AccountName,
		AvatarUrl:           optionalString(v.Profile.AvatarURL),
		AuthorizationType:   authorizationType(v.Profile.AuthorizationType),
		MembershipStatus:    generated.AccountProfileMembershipStatus(v.Profile.MembershipStatus),
		MembershipLevel:     v.Profile.MembershipLevel,
		MembershipExpiresAt: v.Profile.MembershipExpiresAt,
		StorageTotalBytes:   v.Profile.StorageTotalBytes,
		StorageUsedBytes:    v.Profile.StorageUsedBytes,
		StorageFreeBytes:    v.Profile.StorageFreeBytes,
		DeviceCount:         v.Profile.DeviceCount,
		RetrievedAt:         retrievedAt,
	}
	return generated.Connection{Id: v.ID, Name: v.Name, ExternalAccountId: v.ExternalAccountID, Status: generated.ConnectionStatus(v.Status), Capabilities: caps, Profile: profile, UpdatedAt: v.UpdatedAt}
}
func connectionError(c *gin.Context, err error) {
	status := 500
	category, msg := "internal", "115 connection operation failed"
	switch {
	case errors.Is(err, connections.ErrSessionMissing):
		status = 404
		category = "not_found"
		msg = "QR session not found"
	case errors.Is(err, connections.ErrConnectionMissing):
		status = 404
		category = "not_found"
		msg = "115 connection not configured"
	case errors.Is(err, connections.ErrAccountConflict):
		status = 409
		category = "conflict"
		msg = "115 account differs from existing connection"
	case errors.Is(err, connections.ErrNotAllowed):
		status = 422
		category = "invalid_request"
		msg = "QR session is not authorized"
	case errors.Is(err, drive115.ErrInvalidDevice):
		status = 400
		category = "invalid_request"
		msg = "unsupported QR device"
	default:
		status = 503
		category = "unavailable"
		msg = "115 service unavailable"
	}
	c.Header("Cache-Control", "no-store")
	response := gin.H{"category": category, "message": msg}
	if upstream, ok := drive115.AsUpstreamError(err); ok {
		response["message"] = upstream.Message
		if upstream.Code != "" {
			response["upstreamCode"] = upstream.Code
		}
	}
	c.JSON(status, response)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func authorizationType(value string) *generated.AccountProfileAuthorizationType {
	if value == "" {
		return nil
	}
	v := generated.AccountProfileAuthorizationType(value)
	return &v
}
