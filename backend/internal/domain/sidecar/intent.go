// intent.go：定义附属文件下载意图、阶段及身份约束，不持有下载地址、凭据或文件内容。

package sidecar

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("sidecar download intent not found")
	ErrConflict = errors.New("sidecar download intent ownership or state conflict")
)

// Kind 标识网盘已有的 NFO 或图片，不包含视频、字幕或刮削产物。
type Kind string

const (
	KindNFO       Kind  = "nfo"
	KindImage     Kind  = "image"
	MaxNFOBytes   int64 = 8 << 20
	MaxImageBytes int64 = 64 << 20
)

// State 只记录已经完成的文件事实；文件动作由服务在数据库事务外执行。
type State string

const (
	StateReserved  State = "reserved"
	StateStaging   State = "staging"
	StateStaged    State = "staged"
	StatePublished State = "published"
)

// Intent 为一个唯一目标保留来源和任务身份，恢复时只能操作与记录身份一致的临时文件。
// Revision 每次相邻阶段更新递增；ExpectedBytes 为 nil 时表示源未提供可信长度。
type Intent struct {
	ID, JobID                                             string
	Attempt                                               int
	LibraryID, AccountID, SourceFileID, SourceDirectoryID string
	Kind                                                  Kind
	OutputRoot, RelativePath, TargetPath                  string
	TemporaryName, TemporaryDevice, TemporaryInode        string
	State                                                 State
	ExpectedBytes                                         *int64
	CompletedBytes, Revision                              int64
	CreatedAt, UpdatedAt                                  time.Time
}

// Validate 检查持久化所需身份、规范路径、阶段和字节数；不访问磁盘或判定 STRM 下载资格。
func (i Intent) Validate() error {
	if i.ID == "" || i.JobID == "" || i.Attempt < 1 || i.LibraryID == "" || i.AccountID == "" ||
		i.SourceFileID == "" || i.SourceDirectoryID == "" || i.Revision < 1 ||
		i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() || i.UpdatedAt.Before(i.CreatedAt) {
		return ErrConflict
	}
	if !filepath.IsAbs(i.OutputRoot) || filepath.Clean(i.OutputRoot) != i.OutputRoot ||
		i.RelativePath == "" || filepath.IsAbs(i.RelativePath) || filepath.Clean(i.RelativePath) != i.RelativePath ||
		i.RelativePath == "." || i.RelativePath == ".." || strings.HasPrefix(i.RelativePath, ".."+string(filepath.Separator)) ||
		strings.ContainsAny(i.OutputRoot+i.RelativePath+i.TargetPath, "\x00\\") ||
		filepath.Join(i.OutputRoot, i.RelativePath) != i.TargetPath {
		return ErrConflict
	}
	limit := MaxNFOBytes
	switch i.Kind {
	case KindNFO:
	case KindImage:
		limit = MaxImageBytes
	default:
		return ErrConflict
	}
	if i.CompletedBytes < 0 || i.CompletedBytes > limit ||
		(i.ExpectedBytes != nil && (*i.ExpectedBytes < 0 || *i.ExpectedBytes > limit || i.CompletedBytes > *i.ExpectedBytes)) {
		return ErrConflict
	}
	if i.TemporaryName != "" && i.TemporaryName == filepath.Base(i.RelativePath) {
		return ErrConflict
	}
	switch i.State {
	case StateReserved:
		if i.TemporaryDevice != "" || i.TemporaryInode != "" || i.CompletedBytes != 0 ||
			(i.TemporaryName != "" && !validTemporaryName(i.TemporaryName)) {
			return ErrConflict
		}
	case StateStaging, StateStaged, StatePublished:
		if !validTemporaryName(i.TemporaryName) ||
			i.TemporaryDevice == "" || i.TemporaryInode == "" {
			return ErrConflict
		}
		if i.State == StateStaging && i.CompletedBytes != 0 {
			return ErrConflict
		}
		if i.State != StateStaging && i.ExpectedBytes != nil && i.CompletedBytes != *i.ExpectedBytes {
			return ErrConflict
		}
	default:
		return ErrConflict
	}
	return nil
}

// CanAdvance 验证意图只推进相邻阶段，保留来源、运行身份和已建立的临时文件身份。
func (i Intent) CanAdvance(next Intent) error {
	if i.Validate() != nil || next.Validate() != nil || next.Revision != i.Revision+1 ||
		next.UpdatedAt.Before(i.UpdatedAt) || !sameIdentity(i, next) {
		return ErrConflict
	}
	valid := (i.State == StateReserved && next.State == StateStaging) ||
		(i.State == StateStaging && next.State == StateStaged) ||
		(i.State == StateStaged && next.State == StatePublished)
	if !valid {
		return ErrConflict
	}
	if i.TemporaryName != "" && i.TemporaryName != next.TemporaryName {
		return ErrConflict
	}
	if i.State != StateReserved && (i.TemporaryName != next.TemporaryName ||
		i.TemporaryDevice != next.TemporaryDevice || i.TemporaryInode != next.TemporaryInode) {
		return ErrConflict
	}
	if i.State == StateStaged && i.CompletedBytes != next.CompletedBytes {
		return ErrConflict
	}
	return nil
}

// validTemporaryName 限制临时文件为单个叶名称，不允许路径穿越或异常分隔符。
func validTemporaryName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && !strings.ContainsAny(name, "/\\\x00")
}

// sameIdentity 比较不能被阶段推进改写的任务、来源、路径、长度和创建时间。
func sameIdentity(a, b Intent) bool {
	return a.ID == b.ID && a.JobID == b.JobID && a.Attempt == b.Attempt && a.LibraryID == b.LibraryID &&
		a.AccountID == b.AccountID && a.SourceFileID == b.SourceFileID && a.SourceDirectoryID == b.SourceDirectoryID &&
		a.Kind == b.Kind && a.OutputRoot == b.OutputRoot && a.RelativePath == b.RelativePath && a.TargetPath == b.TargetPath &&
		a.CreatedAt.Equal(b.CreatedAt) && ((a.ExpectedBytes == nil && b.ExpectedBytes == nil) ||
		(a.ExpectedBytes != nil && b.ExpectedBytes != nil && *a.ExpectedBytes == *b.ExpectedBytes))
}
