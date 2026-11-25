// generate.go：负责串联输出准备、写入恢复、源扫描和 STRM 生成，并向任务中心报告进度及结果。

// Package strm 生成、登记并查询应用管理的 STRM 产物。
package strm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

var (
	errPlaybackOrigin  = errors.New("请先在媒体库配置中填写播放器可以访问的 STRM 播放地址")
	errOutputWrite     = errors.New("无法读写 STRM 输出文件，请检查输出目录权限及磁盘空间")
	errUnownedOutput   = errors.New("目标路径已存在其他文件或属于其他源文件")
	errModifiedOutput  = errors.New("目标文件内容已变化，无法安全覆盖")
	errRegistry        = errors.New("无法读取或保存 STRM 产物登记")
	errGenerationItems = errors.New("所有待处理项均失败")
)

// Service 管理生成任务、本地产物和播放入口。
type Service struct {
	repo     Repository
	provider Provider
	logger   *slog.Logger

	// 不同媒体库的输出目录可能重叠。发布与恢复共用短临界区，网盘遍历在锁外完成。
	outputMu sync.Mutex
}

// New 构造 STRM 服务并绑定模块日志，发布与恢复共享同一输出互斥锁。
func New(repo Repository, provider Provider, logger *slog.Logger) *Service {
	return &Service{repo: repo, provider: provider, logger: logger.With("module", "strm")}
}

// Module 返回媒体库全量同步使用的 STRM 执行器。
func (s *Service) Module() jobs.Module {
	return jobs.Module{
		ManualOnly: true, Run: s.generate, Recover: s.recoverGeneration,
		Eligible: FullSyncEligible,
	}
}

// generationCounts 汇总生成、失败、跳过及其中的冲突数量。
type generationCounts struct {
	generated, failed, skipped, conflicts int
}

// generate 使用任务配置快照完成输出准备、意图恢复、全量扫描和文件生成；阶段终态统一记录。
func (s *Service) generate(ctx context.Context, execution jobs.Execution) (resultErr error) {
	started := time.Now()
	logger := s.jobLogger(execution, "generate")
	logger.InfoContext(ctx, "STRM generation started")
	phase := "validate_playback"
	total := int64(0)
	counts := generationCounts{}
	failures := generationFailures{byCode: make(map[string]int)}
	defer func() {
		logGenerationResult(ctx, logger.With("phase", phase, "total_items", total), started, counts, resultErr, failures.attributes()...)
	}()

	// 整轮同步使用同一配置快照；读取播放地址一次即可确保全部产物使用相同地址。
	origin, valid := library.PlaybackOrigin(execution.Library.STRMDirectDomain)
	if !valid || origin == "" {
		return reportGenerationError(ctx, execution, errPlaybackOrigin)
	}
	phase = "prepare_output"
	preparing := time.Now()
	root, canonical, err := prepareOutputRoot(ctx, execution.Library.OutputPath)
	if err != nil {
		return reportGenerationError(ctx, execution, err)
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()
	logger.InfoContext(ctx, "STRM output directory ready", "phase", phase, "duration_ms", time.Since(preparing).Milliseconds())
	phase = "reconcile_outputs"
	recoveryConflicts, err := s.reconcileIntents(ctx, execution)
	if err != nil {
		return reportGenerationError(ctx, execution, err)
	}
	if recoveryConflicts > 0 {
		message := fmt.Sprintf("发现 %d 个上次写入冲突，相关产物保留待核对", recoveryConflicts)
		if err := execution.Log(ctx, "warn", message); err != nil {
			return err
		}
	}
	phase = "scan"
	files, err := s.scanForGeneration(ctx, execution, logger.With("phase", phase), &failures)
	if err != nil {
		return reportGenerationError(ctx, execution, err)
	}
	phase = "generate"
	total = int64(len(files))
	return s.generateFiles(ctx, execution, origin, root, canonical, files, logger.With("phase", phase, "total_items", total), &counts, &failures)
}

// scanForGeneration 扫描来源并按目录上报任务进度，聚合完整性与失败统计，限制周期性日志频率。
func (s *Service) scanForGeneration(ctx context.Context, execution jobs.Execution, logger *slog.Logger, failures *generationFailures) ([]sourceFile, error) {
	started := time.Now()
	startAttrs := []any{"include_subdirectories", execution.Library.IncludeSubdirectories}
	if id := execution.Library.DirectoryID; providerID(id) {
		startAttrs = append(startAttrs, "source_directory_id", id)
	}
	logger.InfoContext(ctx, "STRM source scan started", startAttrs...)
	nextLog := started.Add(generationLogInterval)
	files, summary, err := s.scan(ctx, execution.Library, execution.Job.RequestID,
		func(path string, current scanSummary) error {
			now := time.Now()
			if !now.Before(nextLog) {
				logger.InfoContext(ctx, "STRM source scan progress", current.attributes(now.Sub(started))...)
				nextLog = now.Add(generationLogInterval)
			}
			return execution.Report(ctx, domain.Progress{CurrentFile: &path})
		},
		func(source sourceFile) {
			code, _ := generationError(source.err)
			failures.record(ctx, logger, source, code)
		},
	)
	logger.InfoContext(ctx, "STRM source scan finished", append(summary.attributes(time.Since(started)), "scan_complete", err == nil && summary.failedDirectories == 0)...)
	return files, err
}

// generateFiles 逐项生成并持久化结果、推进进度；尊重取消，所有条目均失败时返回批次错误。
func (s *Service) generateFiles(ctx context.Context, execution jobs.Execution, origin string, root *os.Root, canonical string, files []sourceFile, logger *slog.Logger, counts *generationCounts, failures *generationFailures) error {
	started := time.Now()
	logger.InfoContext(ctx, "STRM file generation started")
	total := int64(len(files))
	progress := domain.Progress{TotalItems: &total}
	zero := float64(0)
	progress.ProgressPercent = &zero
	if err := execution.Report(ctx, progress); err != nil {
		return err
	}
	nextReport := time.Now()
	nextLog := started.Add(generationLogInterval)
	for index, source := range files {
		item := domain.Item{Key: source.key, Path: source.path, Status: domain.ItemFailed}
		result, err := s.generateFile(ctx, execution.Library, origin, root, canonical, source)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch {
		case err != nil:
			counts.failed++
			if errors.Is(err, ErrConflict) {
				counts.conflicts++
			}
			code, reason := generationError(err)
			item.Reason = reason
			if source.err == nil {
				failures.record(ctx, logger, source, code)
			}
		case result:
			counts.generated++
			item.Status = domain.ItemSucceeded
		default:
			counts.skipped++
			item.Status, item.Reason = domain.ItemSkipped, "已存在同源的可用 STRM 文件"
		}
		if err := execution.Item(ctx, item); err != nil {
			return err
		}
		now := time.Now()
		if !now.Before(nextLog) {
			attrs := append(counts.attributes(), "processed_items", index+1, "duration_ms", now.Sub(started).Milliseconds())
			logger.InfoContext(ctx, "STRM file generation progress", attrs...)
			nextLog = now.Add(generationLogInterval)
		}
		if now.After(nextReport) {
			percent := float64(index+1) / float64(total) * 100
			progress.ProgressPercent, progress.CurrentFile = &percent, &source.path
			if err := execution.Report(ctx, progress); err != nil {
				return err
			}
			nextReport = time.Now().Add(250 * time.Millisecond)
		}
	}
	complete := float64(100)
	progress.ProgressPercent, progress.CurrentFile = &complete, nil
	if err := execution.Report(ctx, progress); err != nil {
		return err
	}
	if counts.failed > 0 && counts.generated+counts.skipped == 0 {
		return reportGenerationError(ctx, execution, errGenerationItems)
	}
	return nil
}

// jobLogger 绑定任务、媒体库、尝试次数及存在的请求关联标识。
func (s *Service) jobLogger(execution jobs.Execution, operation string) *slog.Logger {
	job := execution.Job
	logger := s.logger.With("operation", operation, "library_id", execution.Library.ID, "job_id", job.ID, "attempt", job.Attempt)
	if job.RequestID != "" {
		logger = logger.With("request_id", job.RequestID)
	}
	return logger
}

// reportGenerationError 将安全错误说明写入任务日志；取消时优先返回上下文错误。
func reportGenerationError(ctx context.Context, execution jobs.Execution, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_, message := generationError(err)
	if logErr := execution.Log(ctx, "error", message); logErr != nil {
		return errors.Join(err, fmt.Errorf("record STRM failure: %w", logErr))
	}
	return err
}

// generationError 把内部及上游错误归类为稳定错误码与管理员可见消息，不暴露原始错误正文。
func generationError(err error) (code, message string) {
	var outputErr *outputRootError
	switch {
	case errors.Is(err, errPlaybackOrigin):
		return "strm_playback_origin_unavailable", errPlaybackOrigin.Error()
	case errors.Is(err, errUnownedOutput):
		return "strm_ownership_conflict", errUnownedOutput.Error()
	case errors.Is(err, errModifiedOutput):
		return "strm_content_conflict", errModifiedOutput.Error()
	case errors.As(err, &outputErr):
		return "strm_output_root_" + outputErr.preview.Status, outputErr.preview.Message
	case errors.Is(err, ErrConflict):
		return "strm_output_conflict", "输出路径或文件与产物登记冲突"
	case errors.Is(err, errOutputWrite):
		return "strm_output_write_failed", errOutputWrite.Error()
	case errors.Is(err, errRegistry):
		return "strm_registry_unavailable", errRegistry.Error()
	case errors.Is(err, errAccountChanged):
		return "strm_115_account_changed", errAccountChanged.Error()
	case errors.Is(err, provider115.ErrUnconfigured):
		return "provider_not_configured", "请先绑定 115 账号"
	case errors.Is(err, provider115.ErrInvalidAuthorization):
		return "provider_auth_invalid", "115 授权已失效，请重新登录"
	case errors.Is(err, provider115.ErrDirectoryNotFound):
		return "provider_directory_not_found", "115 源目录不存在，请检查媒体库配置"
	case errors.Is(err, provider115.ErrUpstream):
		return "provider_unavailable", "115 源目录暂时无法读取"
	case errors.Is(err, errSourcePath):
		return "strm_invalid_source_path", errSourcePath.Error()
	case errors.Is(err, errSourcePagination):
		return "strm_incomplete_source_directory", errSourcePagination.Error()
	case errors.Is(err, errPlaybackCode):
		return "strm_playback_code_missing", errPlaybackCode.Error()
	case err == nil || errors.Is(err, errGenerationItems):
		return "strm_item_failures", "待处理项生成失败，请查看失败明细"
	default:
		return "strm_generation_failed", "STRM 生成失败，请重试"
	}
}
