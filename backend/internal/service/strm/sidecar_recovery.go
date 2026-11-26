// sidecar_recovery.go：恢复独立附件意图，只清理身份可证的暂存；不采用旧下载，也不修改正式文件。

package strm

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	sidecar "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/sidecar"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
)

// reconcileSidecars 在新扫描前核对本库旧意图，恢复不发起下载，歧义暂存保留并计为冲突。
func (s *Service) reconcileSidecars(ctx context.Context, execution jobs.Execution) (int, error) {
	repo, ok := s.repo.(sidecarRepository)
	if !ok { // 未实现附件能力的纯 STRM 仓储不会持有附件意图。
		if execution.Library.GenerateNFO || execution.Library.GeneratePosterCover {
			return 0, errRegistry
		}
		return 0, nil
	}
	intents, err := repo.SidecarIntentsByLibrary(ctx, execution.Library.ID)
	if err != nil {
		return 0, errors.Join(errRegistry, err)
	}
	if len(intents) == 0 {
		return 0, nil
	}
	started := time.Now()
	logger := s.jobLogger(execution, "reconcile_sidecars")
	logger.InfoContext(ctx, "Sidecar reconciliation started", "pending_files", len(intents))
	conflicts, skipped := 0, 0
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return conflicts, err
		}
		err = s.reconcileSidecar(ctx, repo, intent)
		if errors.Is(err, ErrConflict) {
			conflicts++
			continue
		}
		if err != nil {
			logger.WarnContext(ctx, "Sidecar reconciliation finished", "outcome", "failed", "error_code", "sidecar_recovery_failed", "duration_ms", time.Since(started).Milliseconds(), "conflicting_files", conflicts)
			return conflicts, err
		}
		skipped++
	}
	outcome := "succeeded"
	if conflicts > 0 {
		outcome = "failed"
	}
	logger.InfoContext(ctx, "Sidecar reconciliation finished", "outcome", outcome, "duration_ms", time.Since(started).Milliseconds(), "skipped_files", skipped, "conflicting_files", conflicts)
	return conflicts, nil
}

// reconcileSidecar 仅释放不存在或可证归属的暂存；已有普通正式文件始终保留，异常目标或身份冲突不清理。
func (s *Service) reconcileSidecar(ctx context.Context, repo sidecarRepository, intent sidecar.Intent) (resultErr error) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	if !validOutputPath(intent.RelativePath) || sidecarKind(intent.RelativePath) != string(intent.Kind) || filepath.Join(intent.OutputRoot, intent.RelativePath) != intent.TargetPath {
		return ErrConflict
	}
	root, canonical, err := openOutputRoot(intent.OutputRoot)
	if err != nil {
		return errors.Join(ErrConflict, err)
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()
	if canonical != intent.OutputRoot {
		return ErrConflict
	}
	directory, err := openOutputDirectory(root, intent.RelativePath, false)
	if errors.Is(err, fs.ErrNotExist) {
		if intent.TemporaryDevice != "" || intent.TemporaryInode != "" {
			return ErrConflict
		}
		return repo.DiscardSidecarIntent(ctx, intent)
	}
	if err != nil {
		return errors.Join(ErrConflict, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	if err = verifyOutputDirectory(root, directory, canonical, intent.RelativePath); err != nil {
		return errors.Join(ErrConflict, err)
	}
	if _, err = sidecarExists(directory, filepath.Base(intent.RelativePath)); err != nil {
		return err
	}
	if intent.TemporaryName == "" {
		return repo.DiscardSidecarIntent(ctx, intent)
	}
	info, err := directory.Lstat(intent.TemporaryName)
	if errors.Is(err, fs.ErrNotExist) {
		return repo.DiscardSidecarIntent(ctx, intent)
	}
	if err != nil || !info.Mode().IsRegular() {
		return errors.Join(ErrConflict, err)
	}
	if err = verifySidecarIdentity(directory, intent); err != nil {
		return errors.Join(ErrConflict, err)
	}
	// 正式文件与暂存即使是硬链接也只删除暂存名字，绝不删除或接管正式文件。
	return s.finishSidecar(ctx, repo, intent)
}
