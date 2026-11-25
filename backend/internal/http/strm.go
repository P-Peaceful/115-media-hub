// strm.go：提供 STRM 清单、详情、全量同步预览与独立播放入口，隔离管理会话和播放凭据的响应边界。

package http

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/strm"
)

// fullSyncPreviewTimeout 为只读预览保留独立期限，在服务器写入超时前留出响应时间。
const fullSyncPreviewTimeout = 55 * time.Second

// strmHandler 连接 STRM 服务及媒体库配置，不在传输层实现扫描和落盘。
type strmHandler struct {
	service   *strm.Service
	libraries *library.Service
}

// register 注册管理清单和详情、可用时的媒体库预览，以及使用独立凭据的 GET/HEAD 播放入口。
func (h *strmHandler) register(router *gin.Engine) {
	router.GET("/api/v1/strm", h.list)
	router.GET("/api/v1/strm/:id", h.detail)
	if h.libraries != nil {
		router.GET("/api/v1/libraries/:id/full-sync-preview", h.previewFullSync)
	}
	router.GET("/p/:id/:token/:name", h.play)
	router.HEAD("/p/:id/:token/:name", h.play)
}

// previewFullSync 在有界请求上下文中读取媒体库并执行只读预览，保留取消语义且区分配置、授权和源目录故障。
func (h *strmHandler) previewFullSync(c *gin.Context) {
	id := c.Param("id")
	if !validTaskID(id) {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "媒体库标识无效")
		return
	}
	// 预留响应时间，并在浏览器离开预览时停止网盘扫描。
	requestContext := c.Request.Context()
	ctx, cancel := context.WithTimeout(requestContext, fullSyncPreviewTimeout)
	defer cancel()
	requestID := c.GetString("request_id")
	target, err := h.libraries.Get(ctx, id, requestID)
	if err != nil {
		switch {
		case requestContext.Err() != nil:
			return
		case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
			writeError(c, http.StatusGatewayTimeout, generated.FullSyncPreviewTimeout, "读取媒体库配置超时，请重试")
		case errors.Is(err, library.ErrNotFound):
			writeError(c, http.StatusNotFound, generated.NotFound, "媒体库不存在")
		default:
			writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "暂时无法读取媒体库配置")
		}
		return
	}
	result, err := h.service.PreviewFullSync(ctx, target, requestID)
	if requestContext.Err() != nil {
		return
	}
	switch {
	case errors.Is(err, strm.ErrFullSyncUnavailable):
		writeError(c, http.StatusConflict, generated.FullSyncUnavailable, "当前媒体库的配置无法执行全量同步")
	case errors.Is(err, provider115.ErrUnconfigured):
		writeError(c, http.StatusConflict, generated.ProviderNotConfigured, "请先绑定 115 账号")
	case errors.Is(err, provider115.ErrInvalidAuthorization):
		writeError(c, http.StatusConflict, generated.ProviderAuthInvalid, "115 授权已失效，请重新登录")
	case errors.Is(err, provider115.ErrDirectoryNotFound):
		writeError(c, http.StatusNotFound, generated.ProviderDirectoryNotFound, "115 源目录不存在，请检查媒体库配置")
	case err != nil:
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "暂时无法扫描 115 源目录，请重试")
	default:
		samples := make([]generated.FullSyncPreviewSample, 0, len(result.Samples))
		for _, sample := range result.Samples {
			samples = append(samples, generated.FullSyncPreviewSample{SourcePath: sample.SourcePath, RelativePath: sample.RelativePath})
		}
		c.JSON(http.StatusOK, generated.FullSyncPreview{
			Revision: result.Revision, ScanComplete: result.ScanComplete, ScannedVideos: result.ScannedVideos,
			CandidateCount: result.CandidateCount, FailedCount: result.FailedCount, Samples: samples,
			Output: generated.FullSyncOutputCheck{Status: generated.FullSyncOutputCheckStatus(result.Output.Status), Message: result.Output.Message},
		})
	}
}

// list 解析 STRM 分页筛选条件，读取登记清单并转换为 API 分页响应。
func (h *strmHandler) list(c *gin.Context) {
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if pageErr != nil || limitErr != nil {
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "STRM 分页参数无效")
		return
	}
	result, err := h.service.List(c.Request.Context(), strm.ListFilter{
		LibraryID: c.Query("library_id"), Query: c.Query("q"), Sort: c.Query("sort"), Order: c.Query("order"),
		Page: page, Limit: limit,
	}, c.GetString("request_id"))
	switch {
	case errors.Is(err, strm.ErrInvalidQuery):
		writeError(c, http.StatusBadRequest, generated.ValidationFailed, "STRM 查询参数无效")
	case err != nil:
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "暂时无法读取 STRM 清单")
	default:
		items := make([]generated.StrmFile, 0, len(result.Items))
		for _, artifact := range result.Items {
			items = append(items, strmFileResponse(artifact))
		}
		c.JSON(http.StatusOK, generated.StrmPage{Items: items, Total: result.Total, Page: result.Page, Limit: result.Limit})
	}
}

// strmFileResponse 从业务产物提取公开登记字段，使用相对路径文件名及 UTC 生成时间构造传输对象。
func strmFileResponse(artifact strm.Artifact) generated.StrmFile {
	return generated.StrmFile{
		Id: artifact.ID, LibraryId: artifact.LibraryID, LibraryName: artifact.LibraryName,
		Name: filepath.Base(artifact.RelativePath), RelativePath: artifact.RelativePath,
		SourceFileId: artifact.SourceFileID, SourcePath: artifact.SourcePath, SizeBytes: artifact.SizeBytes,
		GeneratedAt: artifact.GeneratedAt.UTC(),
	}
}

// detail 读取登记产物与本地文件状态，返回受服务内容校验约束的文件内容和输出位置。
func (h *strmHandler) detail(c *gin.Context) {
	result, err := h.service.Detail(c.Request.Context(), c.Param("id"), c.GetString("request_id"))
	switch {
	case errors.Is(err, strm.ErrNotFound):
		writeError(c, http.StatusNotFound, generated.NotFound, "找不到 STRM 文件登记")
	case err != nil:
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "暂时无法读取 STRM 文件详情")
	default:
		item := strmFileResponse(result.Artifact)
		c.JSON(http.StatusOK, generated.StrmDetail{
			Id: item.Id, LibraryId: item.LibraryId, LibraryName: item.LibraryName,
			Name: item.Name, RelativePath: item.RelativePath, SourceFileId: item.SourceFileId,
			SourcePath: item.SourcePath, SizeBytes: item.SizeBytes, GeneratedAt: item.GeneratedAt,
			Content: result.Content, OutputPath: result.TargetPath, FileState: generated.StrmDetailFileState(result.FileState),
		})
	}
}

// play 验证独立播放凭据并按客户端 User-Agent 解析临时直链，仅发送不可缓存的 302 响应。
func (h *strmHandler) play(c *gin.Context) {
	// 302 目标具有时效，避免中间缓存与 Referer 传播播放凭据。
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	target, err := h.service.ResolvePlayback(c.Request.Context(), c.Param("id"), c.Param("token"), c.Request.UserAgent(), c.GetString("request_id"))
	switch {
	case errors.Is(err, strm.ErrNotFound), errors.Is(err, provider115.ErrSourceNotFound):
		writeError(c, http.StatusNotFound, generated.NotFound, "找不到可播放的 STRM 文件")
	case errors.Is(err, provider115.ErrUnconfigured):
		writeError(c, http.StatusConflict, generated.ProviderNotConfigured, "请先绑定 115 账号")
	case errors.Is(err, provider115.ErrInvalidAuthorization):
		writeError(c, http.StatusConflict, generated.ProviderAuthInvalid, "115 授权已失效或与产物不匹配")
	case err != nil:
		writeError(c, http.StatusServiceUnavailable, generated.ServiceUnavailable, "暂时无法获取 115 播放地址")
	default:
		c.Header("Location", target)
		c.Status(http.StatusFound)
	}
}
