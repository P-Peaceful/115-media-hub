package strm

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

const fullSyncPreviewSampleLimit = 20

// ErrFullSyncUnavailable 表示媒体库的保存配置不满足全量同步要求。
var ErrFullSyncUnavailable = errors.New("当前媒体库的配置无法执行全量同步")

// FullSyncPreview 汇总已扫描的源文件和输出目录检查结果。
type FullSyncPreview struct {
	Revision       int64
	ScanComplete   bool
	ScannedVideos  int64
	CandidateCount int64
	FailedCount    int64
	Samples        []PreviewSample
	Output         OutputPreview
}

// PreviewSample 描述一个通过源文件校验的 STRM 候选项。
type PreviewSample struct {
	SourcePath   string
	RelativePath string
}

// FullSyncEligible 判断当前配置是否可以提交 STRM 全量同步。
func FullSyncEligible(target library.Library) bool {
	origin, valid := library.PlaybackOrigin(target.STRMDirectDomain)
	return target.Enabled && target.GenerateSTRM && target.LinkType == "proxy" && valid && origin != ""
}

// PreviewFullSync 扫描已保存的媒体库配置，返回只读的预检结果。
func (s *Service) PreviewFullSync(ctx context.Context, target library.Library, requestID string) (result FullSyncPreview, resultErr error) {
	started := time.Now()
	logger := s.logger.With("operation", "full_sync_preview", "library_id", target.ID)
	if requestID != "" {
		logger = logger.With("request_id", requestID)
	}
	logger.InfoContext(ctx, "STRM full sync preview started")
	defer func() { logPreviewResult(ctx, logger, started, result, resultErr) }()

	if !FullSyncEligible(target) {
		return result, ErrFullSyncUnavailable
	}
	result.Revision = target.Revision
	result.Output = inspectOutputRoot(target.OutputPath)
	files, summary, err := s.scan(ctx, target, requestID, nil, nil)
	// 到达预览时限时保留已完整读取的目录。正式任务有独立生命周期，会重新扫描。
	if err != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, err
	}
	result.ScanComplete = err == nil
	result.ScannedVideos = int64(summary.scannedVideos)
	result.CandidateCount = int64(summary.candidateFiles)
	result.FailedCount = int64(summary.failedDirectories + summary.invalidSourceEntries)
	result.Samples = make([]PreviewSample, 0, fullSyncPreviewSampleLimit)
	for _, file := range files {
		if file.err == nil && len(result.Samples) < fullSyncPreviewSampleLimit {
			result.Samples = append(result.Samples, PreviewSample{SourcePath: file.path, RelativePath: filepath.ToSlash(file.relative)})
		}
	}
	return result, nil
}

func logPreviewResult(ctx context.Context, logger *slog.Logger, started time.Time, result FullSyncPreview, err error) {
	level, outcome, code := slog.LevelInfo, "succeeded", ""
	switch {
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case errors.Is(err, ErrFullSyncUnavailable):
		level, outcome, code = slog.LevelWarn, "failed", "full_sync_unavailable"
	case err != nil:
		code, _ = generationError(err)
		if code == "strm_generation_failed" {
			code = "full_sync_preview_unavailable"
		}
		level, outcome = slog.LevelWarn, "failed"
	case !result.ScanComplete:
		level, outcome, code = slog.LevelWarn, "failed", "full_sync_preview_timeout"
	case result.Output.Status != "ready" && result.Output.Status != "will_create":
		level, outcome, code = slog.LevelWarn, "failed", "strm_output_"+result.Output.Status
	case result.FailedCount > 0:
		level, outcome, code = slog.LevelWarn, "failed", "strm_source_scan_incomplete"
	}
	attrs := []any{
		"duration_ms", time.Since(started).Milliseconds(), "outcome", outcome, "scan_complete", result.ScanComplete,
		"scanned_videos", result.ScannedVideos, "candidate_count", result.CandidateCount, "failed_count", result.FailedCount,
		"output_status", result.Output.Status,
	}
	if code != "" {
		attrs = append(attrs, "error_code", code)
	}
	logger.Log(ctx, level, "STRM full sync preview finished", attrs...)
}
