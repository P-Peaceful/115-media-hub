// write.go：负责单个 STRM 的归属核验、写入意图、文件发布和中断恢复，保留冲突内容供核对。

package strm

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

// generateFile 串行核验目标归属与已有内容，按配置跳过或发布；只为匹配源身份的登记保留凭据。
// 返回 true 表示完成新写入，false 且无错误表示同源可用文件被跳过。
func (s *Service) generateFile(ctx context.Context, config library.Library, origin string, root *os.Root, canonical string, source sourceFile) (_ bool, resultErr error) {
	if source.err != nil {
		return false, source.err
	}
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	target := filepath.Join(canonical, source.relative)
	if err := s.verifyPendingTarget(ctx, config.ID, target); err != nil {
		return false, err
	}
	current, err := s.repo.StrmArtifactByTarget(ctx, target)
	owned := err == nil
	if !owned && !errors.Is(err, ErrNotFound) {
		return false, errors.Join(errRegistry, err)
	}
	if owned && (current.LibraryID != config.ID || current.AccountID != source.accountID || current.SourceFileID != source.id) {
		return false, errors.Join(ErrConflict, errUnownedOutput)
	}
	directory, err := openArtifactDirectory(root, source.relative, true)
	if err != nil {
		return false, errors.Join(errOutputWrite, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	name := filepath.Base(source.relative)
	previous, err := existingContent(directory, name)
	if err != nil {
		return false, errors.Join(errOutputWrite, err)
	}
	if previous != nil {
		if !owned {
			return false, errors.Join(ErrConflict, errUnownedOutput)
		}
		if !matchesArtifactContent(current, *previous) {
			return false, errors.Join(ErrConflict, errModifiedOutput)
		}
		if config.STRMExistingFilePolicy == "skip" {
			return false, nil
		}
	}
	artifact := Artifact{
		ID: current.ID, PlaybackToken: current.PlaybackToken, LibraryID: config.ID, AccountID: source.accountID,
		SourceFileID: source.id, PlaybackCode: source.playbackCode, SourcePath: source.path, SourceName: source.name,
		OutputRoot: canonical, RelativePath: source.relative, TargetPath: target,
	}
	if !owned {
		artifact.ID, artifact.PlaybackToken = rand.Text(), rand.Text()
	}
	artifact.PlaybackURL = playbackURL(origin, artifact, config.STRMLinkFormat)
	content := artifact.PlaybackURL + "\n"
	artifact.SizeBytes = int64(len(content))
	if previous != nil && *previous == content && artifact.SourcePath == current.SourcePath && artifact.PlaybackCode == current.PlaybackCode {
		return false, nil
	}
	if len(content) > maxSTRMContent {
		return false, errOutputWrite
	}
	artifact.GeneratedAt = time.Now().UTC()
	if err := s.writeArtifact(ctx, root, directory, WriteIntent{Artifact: artifact, PreviousContent: previous}, content); err != nil {
		return false, err
	}
	return true, nil
}

// verifyPendingTarget 先恢复目标路径已有意图，拒绝其他媒体库占用或无法核实的待写入状态。
func (s *Service) verifyPendingTarget(ctx context.Context, libraryID, target string) error {
	intent, err := s.repo.StrmIntentByTarget(ctx, target)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return errors.Join(errRegistry, err)
	}
	if intent.Artifact.LibraryID != libraryID {
		return errors.Join(ErrConflict, errUnownedOutput)
	}
	_, err = s.reconcileIntent(ctx, intent)
	return err
}

// writeArtifact 先保存恢复意图，再发布并复核内容和目录身份，最后完成登记；不确定结果保留意图。
func (s *Service) writeArtifact(ctx context.Context, root, directory *os.Root, intent WriteIntent, content string) error {
	if err := s.repo.SaveStrmIntent(ctx, intent); err != nil {
		return errors.Join(errRegistry, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	artifact := intent.Artifact
	if err := verifyArtifactDirectory(root, directory, artifact.OutputRoot, artifact.RelativePath); err != nil {
		return errors.Join(ErrConflict, errOutputWrite, err)
	}
	name := filepath.Base(artifact.RelativePath)
	// 持久化意图、发布文件和登记跨越两种存储。发布结果存疑时保留意图供重跑校验。
	if err := publishArtifact(directory, name, intent.PreviousContent, content); err != nil {
		return errors.Join(errOutputWrite, err)
	}
	current, err := existingContent(directory, name)
	if err != nil {
		return errors.Join(errOutputWrite, err)
	}
	if current == nil || *current != content {
		return errors.Join(ErrConflict, errModifiedOutput)
	}
	if err := verifyArtifactDirectory(root, directory, artifact.OutputRoot, artifact.RelativePath); err != nil {
		return errors.Join(ErrConflict, errOutputWrite, err)
	}
	if err := s.repo.CompleteStrmIntent(ctx, intent); err != nil {
		return errors.Join(errRegistry, err)
	}
	return nil
}

// recoverGeneration 供任务中心恢复中断任务；只有意图核对无冲突且无错误时才允许重新排队。
func (s *Service) recoverGeneration(ctx context.Context, execution jobs.Execution) (bool, error) {
	conflicts, err := s.reconcileIntents(ctx, execution)
	if err == nil {
		var other int
		other, err = s.reconcileSidecars(ctx, execution)
		conflicts += other
	}
	return conflicts == 0 && err == nil, err
}

// reconcileIntents 在输出锁内核对媒体库全部意图，保留冲突并计数，其他存储或 I/O 错误立即终止。
func (s *Service) reconcileIntents(ctx context.Context, execution jobs.Execution) (int, error) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	intents, err := s.repo.StrmIntentsByLibrary(ctx, execution.Library.ID)
	if err != nil {
		return 0, errors.Join(errRegistry, err)
	}
	if len(intents) == 0 {
		return 0, nil
	}
	counts := generationCounts{}
	logger := s.jobLogger(execution, "reconcile_generation")
	started := time.Now()
	logger.InfoContext(ctx, "STRM write reconciliation started", "pending_files", len(intents))
	for _, intent := range intents {
		published, err := s.reconcileIntent(ctx, intent)
		switch {
		case errors.Is(err, ErrConflict):
			counts.failed++
			counts.conflicts++
		case err != nil:
			logGenerationResult(ctx, logger, started, counts, err)
			return counts.conflicts, err
		case published:
			counts.generated++
		default:
			counts.skipped++
		}
	}
	logGenerationResult(ctx, logger, started, counts, nil)
	return counts.conflicts, nil
}

// reconcileIntent 核对登记路径、目录及当前内容，补登已发布内容或释放未发布意图；歧义状态保留且返回冲突。
func (s *Service) reconcileIntent(ctx context.Context, intent WriteIntent) (_ bool, resultErr error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	artifact := intent.Artifact
	if !validArtifactPath(artifact.RelativePath) || filepath.Join(artifact.OutputRoot, artifact.RelativePath) != artifact.TargetPath {
		return false, ErrConflict
	}
	if intent.Legacy {
		registered, err := s.repo.StrmArtifactByTarget(ctx, artifact.TargetPath)
		switch {
		case err == nil:
			if registered.ID != artifact.ID || registered.LibraryID != artifact.LibraryID ||
				registered.AccountID != artifact.AccountID || registered.SourceFileID != artifact.SourceFileID ||
				registered.PlaybackToken != artifact.PlaybackToken {
				return false, errors.Join(ErrConflict, errUnownedOutput)
			}
			// 旧意图没有原始内容。有登记的目标由下次同步按归属和播放链接重新核对。
			return false, s.repo.DiscardStrmIntent(ctx, artifact.TargetPath)
		case !errors.Is(err, ErrNotFound):
			return false, errors.Join(errRegistry, err)
		}
	}
	root, canonical, err := openOutputRoot(artifact.OutputRoot)
	if err != nil {
		return false, errors.Join(ErrConflict, classifyOutputError(err))
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()
	if canonical != artifact.OutputRoot {
		return false, ErrConflict
	}
	directory, err := openArtifactDirectory(root, artifact.RelativePath, false)
	if errors.Is(err, fs.ErrNotExist) && intent.PreviousContent == nil {
		return false, s.repo.DiscardStrmIntent(ctx, artifact.TargetPath)
	}
	if err != nil {
		return false, errors.Join(ErrConflict, errOutputWrite, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	current, err := existingContent(directory, filepath.Base(artifact.RelativePath))
	if err != nil {
		return false, errors.Join(ErrConflict, errOutputWrite, err)
	}
	if intent.Legacy && current != nil {
		intent.Artifact.PlaybackURL = legacyPlaybackURL(artifact, *current)
	}
	// 与待发布内容一致时补齐登记，仍是原始内容时释放意图。其余状态保留供管理员核对。
	switch {
	case current != nil && intent.Artifact.PlaybackURL != "" && *current == intent.Artifact.PlaybackURL+"\n":
		if err := syncDirectory(directory); err != nil {
			return false, errors.Join(errOutputWrite, err)
		}
		if err := verifyArtifactDirectory(root, directory, artifact.OutputRoot, artifact.RelativePath); err != nil {
			return false, errors.Join(ErrConflict, errOutputWrite, err)
		}
		if err := s.repo.CompleteStrmIntent(ctx, intent); err != nil {
			return false, errors.Join(errRegistry, err)
		}
		return true, nil
	case current == nil && intent.PreviousContent == nil,
		current != nil && intent.PreviousContent != nil && *current == *intent.PreviousContent:
		return false, s.repo.DiscardStrmIntent(ctx, artifact.TargetPath)
	default:
		return false, errors.Join(ErrConflict, errModifiedOutput)
	}
}
