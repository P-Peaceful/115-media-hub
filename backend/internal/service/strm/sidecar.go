// sidecar.go：负责附属文件的同目录 STRM 资格、持久化下载意图和不可覆盖发布；传输期间不占用输出锁。

package strm

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	sidecar "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/sidecar"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/jobs"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

var errSidecarQualification = errors.New("同目录没有已验证的同源有效 STRM，跳过附属文件")

const sidecarProcessingTimeout = 120 * time.Second

// sidecarRepository 是附件发布所需的独立持久化边界，不混入带播放凭据的 STRM 登记。
type sidecarRepository interface {
	ReserveSidecarIntent(context.Context, sidecar.Intent) error
	AdvanceSidecarIntent(context.Context, sidecar.Intent, sidecar.Intent) error
	SidecarIntentsByLibrary(context.Context, string) ([]sidecar.Intent, error)
	DiscardSidecarIntent(context.Context, sidecar.Intent) error
}

// sidecarProvider 只由正式执行消费；预览不调用取链或内容下载。
type sidecarProvider interface {
	DownloadSidecar(context.Context, provider115.DownloadRequest, provider115.DownloadSink) (provider115.DownloadReceipt, error)
}

// qualifiedDirectory 重新读取本轮成功处理的视频登记与实际 STRM 内容；父子目录资格不继承。
func (s *Service) qualifiedDirectory(ctx context.Context, execution jobs.Execution, root *os.Root, canonical string, source sourceFile, videos []sourceFile) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	for _, video := range videos {
		if video.directoryID != source.directoryID || video.accountID != source.accountID {
			continue
		}
		target := filepath.Join(canonical, video.relative)
		artifact, err := s.repo.StrmArtifactByTarget(ctx, target)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return false, errors.Join(errRegistry, err)
		}
		if artifact.LibraryID != execution.Library.ID || artifact.AccountID != source.accountID || artifact.SourceFileID != video.id || artifact.PlaybackCode != video.playbackCode || artifact.SourcePath != video.path || artifact.OutputRoot != canonical || artifact.RelativePath != video.relative || artifact.TargetPath != target {
			continue
		}
		directory, err := openArtifactDirectory(root, video.relative, false)
		if err != nil {
			continue
		}
		content, readErr := readArtifactLeaf(directory, filepath.Base(video.relative))
		bindingErr := verifyArtifactDirectory(root, directory, canonical, video.relative)
		closeErr := directory.Close()
		if readErr == nil && bindingErr == nil && closeErr == nil && matchesArtifactContent(artifact, content) {
			return true, nil
		}
	}
	return false, nil
}

// processSidecar 在准入与发布两处复核资格；返回的清理由调用者在任务条目成功持久化后执行。
func (s *Service) processSidecar(ctx context.Context, execution jobs.Execution, root *os.Root, canonical string, source sourceFile, videos []sourceFile) (generated bool, skipped bool, finish func(context.Context) error, resultErr error) {
	// 单个附件的总预算覆盖资格复核、意图写入、排队、取链、重试和发布准备。
	processingCtx, cancel := context.WithTimeout(ctx, sidecarProcessingTimeout)
	defer cancel()
	ctx = processingCtx
	qualified, qualificationErr := s.qualifiedDirectory(ctx, execution, root, canonical, source, videos)
	if qualificationErr != nil {
		return false, false, nil, qualificationErr
	}
	if !qualified {
		return false, true, nil, errSidecarQualification
	}
	repo, ok := s.repo.(sidecarRepository)
	if !ok {
		return false, false, nil, errRegistry
	}
	if !validOutputPath(source.relative) || sidecarKind(source.relative) != source.kind {
		return false, false, nil, ErrConflict
	}
	limit := int64(8 << 20)
	if source.kind == "image" {
		limit = 64 << 20
	}
	// 只在保留和发布的短阶段持有锁，下载写入独立临时句柄。
	s.outputMu.Lock()
	directory, err := openOutputDirectory(root, source.relative, true)
	if err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(errOutputWrite, err)
	}
	if err = verifyOutputDirectory(root, directory, canonical, source.relative); err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(ErrConflict, err, directory.Close())
	}
	exists, err := sidecarExists(directory, filepath.Base(source.relative))
	if err != nil || exists {
		s.outputMu.Unlock()
		return false, exists, nil, errors.Join(err, directory.Close())
	}
	if source.err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(source.err, directory.Close())
	}
	provider, ok := s.provider.(sidecarProvider)
	if !ok {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(provider115.ErrUpstream, directory.Close())
	}
	if source.expectedBytes != nil && *source.expectedBytes > limit {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(provider115.ErrDownloadTooLarge, directory.Close())
	}
	now := time.Now().UTC()
	intent := sidecar.Intent{ID: rand.Text(), JobID: execution.Job.ID, Attempt: execution.Job.Attempt, LibraryID: execution.Library.ID, AccountID: source.accountID, SourceFileID: source.id, SourceDirectoryID: source.directoryID, Kind: sidecar.Kind(source.kind), OutputRoot: canonical, RelativePath: source.relative, TargetPath: filepath.Join(canonical, source.relative), TemporaryName: ".sidecar-" + rand.Text() + ".tmp", State: sidecar.StateReserved, ExpectedBytes: source.expectedBytes, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if err = repo.ReserveSidecarIntent(ctx, intent); err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(ErrConflict, err, directory.Close())
	}
	file, err := directory.OpenFile(intent.TemporaryName, os.O_RDWR|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o644)
	if err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(errOutputWrite, err, directory.Close())
	}
	info, err := file.Stat()
	if err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(err, file.Close(), directory.Close())
	}
	device, inode, err := sidecarIdentity(info)
	if err != nil {
		s.outputMu.Unlock()
		return false, false, nil, errors.Join(err, file.Close(), directory.Close())
	}
	next := intent
	next.State = sidecar.StateStaging
	next.TemporaryDevice = device
	next.TemporaryInode = inode
	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	err = repo.AdvanceSidecarIntent(ctx, intent, next)
	s.outputMu.Unlock()
	if err != nil {
		return false, false, nil, errors.Join(errRegistry, err, file.Close(), directory.Close())
	}
	intent = next
	if err = syncDirectory(directory); err != nil {
		return false, false, nil, errors.Join(err, file.Close(), directory.Close())
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close(), directory.Close()) }()
	// cleanup 在条目落库后执行；捕获最终意图快照，不删除身份不符的文件。
	finish = func(cleanCtx context.Context) error { return s.finishSidecar(cleanCtx, repo, intent) }
	receipt, err := provider.DownloadSidecar(ctx, provider115.DownloadRequest{AccountID: source.accountID, SourceFileID: source.id, PlaybackCode: source.playbackCode, Kind: source.kind, ExpectedBytes: source.expectedBytes, RequestID: execution.Job.RequestID}, file)
	if err != nil {
		return false, false, finish, err
	}
	if receipt.Bytes < 0 || receipt.Bytes > limit || (source.expectedBytes != nil && receipt.Bytes != *source.expectedBytes) {
		return false, false, finish, errOutputWrite
	}
	actual, err := file.Stat()
	if err != nil || actual.Size() != receipt.Bytes || !os.SameFile(info, actual) {
		return false, false, finish, errors.Join(ErrConflict, err)
	}
	if err = file.Sync(); err != nil {
		return false, false, finish, errors.Join(errOutputWrite, err)
	}
	next = intent
	next.State = sidecar.StateStaged
	next.CompletedBytes = receipt.Bytes
	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	if err = repo.AdvanceSidecarIntent(ctx, intent, next); err != nil {
		return false, false, nil, errors.Join(errRegistry, err)
	}
	intent = next
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	if err = ctx.Err(); err != nil {
		return false, false, nil, err
	}
	if err = receipt.Check(ctx); err != nil {
		return false, false, finish, err
	}
	qualified, qualificationErr = s.qualifiedDirectory(ctx, execution, root, canonical, source, videos)
	if qualificationErr != nil {
		return false, false, finish, qualificationErr
	}
	if !qualified {
		return false, true, finish, errSidecarQualification
	}
	if err = verifyOutputDirectory(root, directory, canonical, source.relative); err != nil {
		return false, false, nil, errors.Join(ErrConflict, err)
	}
	if err = verifySidecarIdentity(directory, intent); err != nil {
		return false, false, nil, err
	}
	name := filepath.Base(source.relative)
	if err = directory.Link(intent.TemporaryName, name); err != nil {
		if errors.Is(err, fs.ErrExist) {
			exists, checkErr := sidecarExists(directory, name)
			return false, exists, finish, checkErr
		}
		return false, false, finish, errors.Join(errOutputWrite, err)
	}
	// 核验最终叶文件确实来自捕获的临时文件；歧义状态仅保留，不删除任何正式文件。
	final, err := directory.Lstat(name)
	if err != nil || !final.Mode().IsRegular() || !os.SameFile(info, final) || final.Size() != receipt.Bytes {
		return false, false, nil, errors.Join(ErrConflict, err)
	}
	if err = errors.Join(syncDirectory(directory), verifyOutputDirectory(root, directory, canonical, source.relative)); err != nil {
		return false, false, nil, errors.Join(ErrConflict, err)
	}
	next = intent
	next.State = sidecar.StatePublished
	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	if err = repo.AdvanceSidecarIntent(ctx, intent, next); err != nil {
		return false, false, nil, errors.Join(errRegistry, err)
	}
	intent = next
	return true, false, finish, nil
}

// sidecarExists 仅以 Lstat 判断普通文件是否已存在；符号链接、目录和其他叶类型均为冲突。
func sidecarExists(directory *os.Root, name string) (bool, error) {
	info, err := directory.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, ErrConflict
	}
	return true, nil
}

// sidecarIdentity 使用设备号与 inode 记录临时文件身份，拒绝无法辨识的文件信息。
func sidecarIdentity(info fs.FileInfo) (string, string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", ErrConflict
	}
	return strconv.FormatUint(uint64(stat.Dev), 10), strconv.FormatUint(stat.Ino, 10), nil
}

// verifySidecarIdentity 只接受仍为同一普通临时文件的叶项，不跟随链接。
func verifySidecarIdentity(directory *os.Root, intent sidecar.Intent) error {
	if !validPathPart(intent.TemporaryName) || intent.TemporaryDevice == "" || intent.TemporaryInode == "" {
		return ErrConflict
	}
	info, err := directory.Lstat(intent.TemporaryName)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrConflict
	}
	device, inode, err := sidecarIdentity(info)
	if err != nil || device != intent.TemporaryDevice || inode != intent.TemporaryInode {
		return errors.Join(ErrConflict, err)
	}
	return nil
}

// finishSidecar 重新打开原目录并核验临时身份，删除已证明归属的暂存后才释放持久化意图。
func (s *Service) finishSidecar(ctx context.Context, repo sidecarRepository, intent sidecar.Intent) (resultErr error) {
	root, canonical, err := openOutputRoot(intent.OutputRoot)
	if err != nil {
		return errors.Join(ErrConflict, err)
	}
	defer func() { resultErr = errors.Join(resultErr, root.Close()) }()
	if canonical != intent.OutputRoot || !validOutputPath(intent.RelativePath) || filepath.Join(canonical, intent.RelativePath) != intent.TargetPath {
		return ErrConflict
	}
	directory, err := openOutputDirectory(root, intent.RelativePath, false)
	if err != nil {
		return errors.Join(ErrConflict, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	if err = verifyOutputDirectory(root, directory, canonical, intent.RelativePath); err != nil {
		return errors.Join(ErrConflict, err)
	}
	if intent.TemporaryName != "" {
		err = verifySidecarIdentity(directory, intent)
		if !errors.Is(err, fs.ErrNotExist) {
			if err != nil {
				return errors.Join(ErrConflict, err)
			}
			if err = directory.Remove(intent.TemporaryName); err != nil {
				return err
			}
			if err = syncDirectory(directory); err != nil {
				return err
			}
		}
	}
	return repo.DiscardSidecarIntent(ctx, intent)
}
