// library.go：实现媒体库配置的 HTTP 增删改查、启停、分页和严格请求字段校验，更新时传递版本以检测冲突。

package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

// libraryHandler 仅负责配置传输与服务调用，媒体库约束由业务服务维护。
type libraryHandler struct{ service *library.Service }

// register 注册媒体库列表、详情、创建、更新、启停和删除接口。
func (h *libraryHandler) register(r *gin.Engine) {
	r.GET("/api/v1/libraries", h.list)
	r.POST("/api/v1/libraries", h.create)
	r.GET("/api/v1/libraries/:id", h.get)
	r.PUT("/api/v1/libraries/:id", h.update)
	r.PATCH("/api/v1/libraries/:id/enabled", h.enabled)
	r.DELETE("/api/v1/libraries/:id", h.delete)
}

// configFields 列出完整配置请求允许且必须提供的字段，不包含响应专用状态。
var configFields = []string{"name", "directory_id", "directory_path", "media_type", "output_path", "generate_strm", "strm_existing_file_policy", "generate_nfo", "generate_poster_cover", "language_preference", "scrape_source", "exclude_directory_path", "include_subdirectories", "follow_directory_changes", "file_encoding", "link_type", "strm_direct_domain", "strm_link_format"}

// decodeLibraryBody 解码限定大小的单个 JSON 配置，拒绝未知、不支持、缺失或 null 字段。
func decodeLibraryBody(c *gin.Context, target any, required []string) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(c, 400, generated.ValidationFailed, "请求必须使用 JSON 格式")
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	if err != nil {
		writeError(c, 400, generated.ValidationFailed, "请求内容无效或过大")
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(c, 400, generated.ValidationFailed, "请求内容无效")
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		writeError(c, 400, generated.ValidationFailed, "请求内容无效")
		return false
	}
	for key := range fields {
		if !slices.Contains(required, key) {
			writeError(c, 400, generated.ValidationFailed, "请求内容包含不支持的字段")
			return false
		}
	}
	for _, key := range required {
		if raw, ok := fields[key]; !ok || string(raw) == "null" {
			writeError(c, 400, generated.ValidationFailed, "请填写所有必填字段")
			return false
		}
	}
	return true
}

// failure 将媒体库校验、版本冲突和不存在错误转换为稳定响应，隐藏存储细节。
func (h *libraryHandler) failure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, library.ErrInvalid):
		writeError(c, 400, generated.ValidationFailed, "媒体库配置无效，请检查输入字段")
	case errors.Is(err, library.ErrConflict):
		writeError(c, 409, generated.Conflict, "媒体库已被修改，请刷新后重试")
	case errors.Is(err, library.ErrNotFound):
		writeError(c, 404, generated.NotFound, "媒体库不存在")
	default:
		writeError(c, 503, generated.ServiceUnavailable, "媒体库服务暂时不可用")
	}
}

// list 校验游标和分页上限，多读取一条判断后续页，并返回媒体库列表与下一游标。
func (h *libraryHandler) list(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	cursor := c.Query("cursor")
	if err != nil || limit < 1 || limit > 200 || len(cursor) > 64 || strings.ContainsAny(cursor, " \t\r\n") {
		h.failure(c, library.ErrInvalid)
		return
	}
	items, err := h.service.List(c.Request.Context(), cursor, limit+1, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	var next *string
	if len(items) > limit {
		items = items[:limit]
		id := items[len(items)-1].ID
		next = &id
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}

// get 按路径标识读取媒体库配置，交由服务校验与处理不存在情况。
func (h *libraryHandler) get(c *gin.Context) {
	item, err := h.service.Get(c.Request.Context(), c.Param("id"), c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, item)
}

// create 解码完整配置并创建媒体库，成功时返回 201 与保存结果。
func (h *libraryHandler) create(c *gin.Context) {
	var config library.Config
	if !decodeLibraryBody(c, &config, configFields) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), config, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(201, item)
}

// update 读取规范的正整数配置版本，提交完整配置以进行乐观并发更新。
func (h *libraryHandler) update(c *gin.Context) {
	revisionText := c.GetHeader("X-Library-Revision")
	revision, err := strconv.ParseInt(revisionText, 10, 64)
	if err != nil || revision < 1 || strconv.FormatInt(revision, 10) != revisionText {
		h.failure(c, library.ErrInvalid)
		return
	}
	var config library.Config
	if !decodeLibraryBody(c, &config, configFields) {
		return
	}
	item, err := h.service.Update(c.Request.Context(), c.Param("id"), config, revision, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, item)
}

// enabled 仅解码启停字段并更新媒体库启用状态。
func (h *libraryHandler) enabled(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeLibraryBody(c, &body, []string{"enabled"}) {
		return
	}
	item, err := h.service.SetEnabled(c.Request.Context(), c.Param("id"), body.Enabled, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, item)
}

// delete 携带管理员身份删除媒体库配置，成功后返回无内容响应。
func (h *libraryHandler) delete(c *gin.Context) {
	err := h.service.Delete(c.Request.Context(), c.Param("id"), providerIdentity(c).Administrator.ID, c.GetString("request_id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.Status(204)
}
