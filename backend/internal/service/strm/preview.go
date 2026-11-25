// preview.go：负责全量同步资格与只读预检，汇总有界抽样页面的候选数量、样例和输出目录状态。

package strm

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

// fullSyncPreviewSampleLimit 限制预览响应中的源和输出路径样例数量。
const fullSyncPreviewSampleLimit = 20

// ErrFullSyncUnavailable 表示媒体库的保存配置不满足全量同步要求。
var ErrFullSyncUnavailable = errors.New("当前媒体库的配置无法执行全量同步")

// FullSyncPreview 汇总抽样已见源文件和输出目录检查结果，不推算全库数量。
type FullSyncPreview struct {
	Revision       int64
	Mode           string
	StopReason     string
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

// PreviewFullSync 按保存的媒体库配置做有界抽样，返回只读预检；确认后的正式任务重新完整扫描。
func (s *Service) PreviewFullSync(ctx context.Context, target library.Library, requestID string) (result FullSyncPreview, resultErr error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
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
	result.Mode = "sample"
	result.Samples = make([]PreviewSample, 0, fullSyncPreviewSampleLimit)
	if err := s.previewScan(ctx, target, requestID, &result); err != nil {
		return result, err
	}
	return result, nil
}

// logPreviewResult 按扫描完成度、输出状态和失败数记录预检结果，不记录路径样例。
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
	case result.StopReason == "source_error":
		level, outcome, code = slog.LevelWarn, "failed", "full_sync_preview_"+result.StopReason
	case result.Output.Status != "ready" && result.Output.Status != "will_create":
		level, outcome, code = slog.LevelWarn, "failed", "strm_output_"+result.Output.Status
	case result.FailedCount > 0:
		level, outcome, code = slog.LevelWarn, "failed", "strm_source_scan_incomplete"
	}
	attrs := []any{
		"mode", result.Mode, "stop_reason", result.StopReason,
		"duration_ms", time.Since(started).Milliseconds(), "outcome", outcome, "scan_complete", result.ScanComplete,
		"scanned_videos", result.ScannedVideos, "candidate_count", result.CandidateCount, "failed_count", result.FailedCount,
		"output_status", result.Output.Status,
	}
	if code != "" {
		attrs = append(attrs, "error_code", code)
	}
	logger.Log(ctx, level, "STRM full sync preview finished", attrs...)
}
