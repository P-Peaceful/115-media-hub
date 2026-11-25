// read.go：负责校验 STRM 登记查询条件和读取详情，仅向客户端展示与登记身份匹配的本地内容。

package strm

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// List 校验筛选、排序和分页范围后读取登记列表，失败时记录脱敏诊断。
func (s *Service) List(ctx context.Context, filter ListFilter, requestID string) (ArtifactPage, error) {
	switch {
	case utf8.RuneCountInString(filter.LibraryID) > 128, utf8.RuneCountInString(filter.Query) > 256, strings.ContainsRune(filter.LibraryID+filter.Query, 0):
		return ArtifactPage{}, ErrInvalidQuery
	case filter.Sort != "" && filter.Sort != "name" && filter.Sort != "generated":
		return ArtifactPage{}, ErrInvalidQuery
	case filter.Order != "" && filter.Order != "asc" && filter.Order != "desc":
		return ArtifactPage{}, ErrInvalidQuery
	case filter.Page < 1 || filter.Page > 1000000 || filter.Limit < 1 || filter.Limit > 200:
		return ArtifactPage{}, ErrInvalidQuery
	}
	started := time.Now()
	page, err := s.repo.ListStrmArtifacts(ctx, filter)
	if err != nil {
		s.logger.WarnContext(ctx, "读取 STRM 清单失败", "operation", "list", "outcome", "failed", "error_code", "strm_registry_unavailable", "request_id", requestID, "duration_ms", time.Since(started).Milliseconds())
	}
	return page, err
}

// Detail 读取登记并核对本地文件状态，内容缺失、变化或不可读时不返回预览。
func (s *Service) Detail(ctx context.Context, id, requestID string) (Detail, error) {
	started := time.Now()
	artifact, err := s.repo.StrmArtifact(ctx, id)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			s.logger.WarnContext(ctx, "读取 STRM 登记失败", "operation", "detail", "outcome", "failed", "error_code", "strm_registry_unavailable", "request_id", requestID, "duration_ms", time.Since(started).Milliseconds())
		}
		return Detail{}, err
	}
	state, content := artifactContent(artifact)
	if state != "present" {
		s.logger.InfoContext(ctx, "检查 STRM 本地内容", "operation", "detail", "outcome", "failed", "error_code", "strm_output_"+state, "artifact_id", artifact.ID, "library_id", artifact.LibraryID, "request_id", requestID, "duration_ms", time.Since(started).Milliseconds())
	}
	return Detail{Artifact: artifact, FileState: state, Content: content}, nil
}

// artifactContent 只预览与登记播放链接相符的内容，避免把路径上后来出现的其他文件返回给客户端。
func artifactContent(artifact Artifact) (state string, preview *string) {
	if !filepath.IsLocal(artifact.RelativePath) || filepath.Join(artifact.OutputRoot, artifact.RelativePath) != artifact.TargetPath {
		return "unavailable", nil
	}
	root, canonical, err := openOutputRoot(artifact.OutputRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "unavailable", nil
	}
	defer func() {
		if err := root.Close(); err != nil {
			state, preview = "unavailable", nil
		}
	}()
	if canonical != artifact.OutputRoot {
		return "unavailable", nil
	}
	content, err := readArtifactFile(root, artifact.RelativePath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "missing", nil
	case err != nil:
		return "unavailable", nil
	case !matchesArtifactContent(artifact, content):
		return "modified", nil
	default:
		return "present", &content
	}
}
