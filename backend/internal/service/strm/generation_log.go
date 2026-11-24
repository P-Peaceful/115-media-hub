package strm

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const (
	generationLogInterval = 10 * time.Second
	generationSampleLimit = 3
)

type generationFailures struct {
	byCode     map[string]int
	suppressed int
}

func (failures *generationFailures) record(ctx context.Context, logger *slog.Logger, source sourceFile, code string) {
	failures.byCode[code]++
	// 单次任务每种失败原因只记录前三条明细，其他失败仍纳入分类汇总。
	if failures.byCode[code] > generationSampleLimit {
		failures.suppressed++
		return
	}
	attrs := []any{"error_code", code}
	if providerID(source.directoryID) {
		attrs = append(attrs, "source_directory_id", source.directoryID)
	}
	if providerID(source.id) {
		attrs = append(attrs, "source_file_id", source.id)
	}
	logger.WarnContext(ctx, "STRM source item failed", attrs...)
}

func (failures generationFailures) attributes() []any {
	return []any{"failure_counts", failures.byCode, "suppressed_failure_logs", failures.suppressed}
}

func (summary scanSummary) attributes(elapsed time.Duration) []any {
	return []any{
		"duration_ms", elapsed.Milliseconds(),
		"completed_directories", summary.completedDirectories,
		"failed_directories", summary.failedDirectories,
		"scanned_videos", summary.scannedVideos,
		"candidate_files", summary.candidateFiles,
		"invalid_source_entries", summary.invalidSourceEntries,
	}
}

func (counts generationCounts) attributes() []any {
	return []any{
		"generated_files", counts.generated, "failed_files", counts.failed,
		"skipped_files", counts.skipped, "conflicting_files", counts.conflicts,
	}
}

func logGenerationResult(ctx context.Context, logger *slog.Logger, started time.Time, counts generationCounts, err error, extra ...any) {
	level, outcome := slog.LevelInfo, "succeeded"
	attrs := append(counts.attributes(), "duration_ms", time.Since(started).Milliseconds())
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		outcome = "canceled"
	case err == nil && counts.failed > 0 && counts.generated+counts.skipped > 0:
		level = slog.LevelWarn
		code, _ := generationError(errGenerationItems)
		attrs = append(attrs, "partial_result", true, "error_code", code)
	case err != nil || counts.failed > 0:
		level, outcome = slog.LevelWarn, "failed"
		code, _ := generationError(err)
		attrs = append(attrs, "error_code", code)
	}
	logger.Log(ctx, level, "STRM operation finished", append(append(attrs, "outcome", outcome), extra...)...)
}
