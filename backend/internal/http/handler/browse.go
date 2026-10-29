package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/http/generated"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/browse"
)

type Browse struct{ Service *browse.Service }

func (h Browse) GetDirectory(c *gin.Context, directoryID string) {
	if h.Service == nil {
		browseError(c, browse.ErrUnavailable)
		return
	}
	result, err := h.Service.Directory(c.Request.Context(), directoryID)
	if err != nil {
		browseError(c, err)
		return
	}
	crumbs := make([]generated.DirectoryBreadcrumb, 0, len(result.Breadcrumbs))
	for _, item := range result.Breadcrumbs {
		crumbs = append(crumbs, generated.DirectoryBreadcrumb{DirectoryId: item.ID, Name: item.Name})
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, generated.DirectoryMetadata{DirectoryId: result.ID, Name: result.Name, Breadcrumbs: crumbs})
}

func (h Browse) GetDirectoryEntries(c *gin.Context, directoryID string, params generated.Get115DirectoryEntriesParams) {
	if h.Service == nil {
		browseError(c, browse.ErrUnavailable)
		return
	}
	offset, limit, kind, query := 0, 6, "all", ""
	if params.Offset != nil {
		offset = *params.Offset
	}
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Type != nil {
		kind = string(*params.Type)
	}
	if params.Q != nil {
		query = *params.Q
	}
	result, err := h.Service.Entries(c.Request.Context(), directoryID, offset, limit, kind, query)
	if err != nil {
		browseError(c, err)
		return
	}
	entries := make([]generated.DirectoryEntry, 0, len(result.Entries))
	for _, item := range result.Entries {
		entries = append(entries, generated.DirectoryEntry{Id: item.ID, ParentId: item.ParentID, Name: item.Name, IsDirectory: item.IsDirectory, Size: item.Size, ModifiedAt: item.ModifiedAt, FileType: generated.DirectoryEntryFileType(item.FileType)})
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, generated.DirectoryEntriesPage{DirectoryId: result.DirectoryID, Offset: result.Offset, Limit: result.Limit, Total: result.Total, Entries: entries})
}

func browseError(c *gin.Context, err error) {
	status, category, message := http.StatusServiceUnavailable, generated.ErrorResponseCategoryUnavailable, "115 网盘暂时不可用，请稍后重试"
	switch {
	case errors.Is(err, browse.ErrInvalidRequest), errors.Is(err, drive115.ErrInvalidBrowseFilter):
		status, category, message = http.StatusBadRequest, generated.ErrorResponseCategoryInvalidRequest, "目录浏览参数无效"
	case errors.Is(err, browse.ErrConnectionMissing):
		status, category, message = http.StatusNotFound, generated.ErrorResponseCategoryConnectionMissing, "尚未配置 115 网盘连接"
	case errors.Is(err, browse.ErrDirectoryNotFound):
		status, category, message = http.StatusNotFound, generated.ErrorResponseCategoryNotFound, "目录不存在或无法访问"
	case errors.Is(err, browse.ErrUnavailable), errors.Is(err, drive115.ErrCredentialInvalid):
		status, category, message = http.StatusServiceUnavailable, generated.ErrorResponseCategoryUnavailable, "115 网盘连接不可用，请重新连接"
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(status, generated.ErrorResponse{Category: category, Message: message})
}
