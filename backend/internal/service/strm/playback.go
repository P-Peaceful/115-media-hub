// playback.go：负责构造稳定产物播放入口、校验当前或旧版文件内容，并以产物凭据授权播放解析。

package strm

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

// playbackURL 按配置的文件名格式构造产物专属稳定播放入口，对各路径片段转义。
func playbackURL(baseURL string, artifact Artifact, format string) string {
	name := artifact.SourceFileID
	if format == "original_name" {
		name = artifact.SourceName
	}
	return strings.TrimRight(baseURL, "/") + "/p/" + url.PathEscape(artifact.ID) + "/" + url.PathEscape(artifact.PlaybackToken) + "/" + url.PathEscape(name)
}

// matchesArtifactContent 核对文件与登记时播放地址逐字匹配；旧登记通过规范入口身份识别。
func matchesArtifactContent(artifact Artifact, content string) bool {
	if artifact.PlaybackURL != "" {
		return content == artifact.PlaybackURL+"\n"
	}
	return legacyPlaybackURL(artifact, content) != ""
}

// legacyPlaybackURL 识别旧登记的单行规范入口，要求匹配产物身份、凭据及源文件名或 ID。
func legacyPlaybackURL(artifact Artifact, content string) string {
	line, ok := strings.CutSuffix(content, "\n")
	if !ok || artifact.ID == "" || artifact.PlaybackToken == "" {
		return ""
	}
	parsed, err := url.Parse(line)
	if err != nil {
		return ""
	}
	origin, valid := library.PlaybackOrigin(parsed.Scheme + "://" + parsed.Host)
	if !valid {
		return ""
	}
	if line == playbackURL(origin, artifact, "file_id") || line == playbackURL(origin, artifact, "original_name") {
		return line
	}
	return ""
}

// ResolvePlayback 使用产物自己的凭据授权，不依赖播放器无法携带的管理员会话。
func (s *Service) ResolvePlayback(ctx context.Context, id, token, userAgent, requestID string) (_ string, resultErr error) {
	started := time.Now()
	logger := s.logger.With("operation", "resolve_playback", "request_id", requestID)
	defer func() {
		attrs := []any{"duration_ms", time.Since(started).Milliseconds()}
		if resultErr == nil {
			logger.DebugContext(ctx, "STRM 播放地址已解析", append(attrs, "outcome", "succeeded")...)
			return
		}
		logger.InfoContext(ctx, "STRM 播放地址解析失败", append(attrs, "outcome", "failed", "error_code", playbackErrorCode(resultErr))...)
	}()

	if len(id) > 128 || len(token) > 128 || id == "" || token == "" {
		return "", ErrNotFound
	}
	artifact, err := s.repo.StrmArtifact(ctx, id)
	if err != nil {
		return "", err
	}
	// 使用恒定时间比较，并在读取到登记前避免把路径参数写入日志。
	if artifact.LibraryName == nil || artifact.PlaybackToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(artifact.PlaybackToken)) != 1 {
		return "", ErrNotFound
	}
	logger = logger.With("artifact_id", artifact.ID, "library_id", artifact.LibraryID)
	return s.provider.ResolvePlayback(ctx, artifact.AccountID, artifact.SourceFileID, artifact.PlaybackCode, userAgent, requestID)
}

// playbackErrorCode 将播放失败归类为客户端和日志使用的稳定错误码。
func playbackErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, provider115.ErrSourceNotFound):
		return "not_found"
	case errors.Is(err, provider115.ErrUnconfigured):
		return "provider_not_configured"
	case errors.Is(err, provider115.ErrInvalidAuthorization):
		return "provider_auth_invalid"
	default:
		return "strm_resolution_unavailable"
	}
}
