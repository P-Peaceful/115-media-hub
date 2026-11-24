package strm

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("STRM artifact not found")
	ErrConflict     = errors.New("STRM output ownership or content conflict")
	ErrInvalidQuery = errors.New("invalid STRM query")
)

// Artifact 保存生成时的源文件身份和本地产物归属。
type Artifact struct {
	ID, LibraryID, AccountID, SourceFileID, PlaybackCode string
	SourcePath, SourceName                               string
	OutputRoot, RelativePath, TargetPath                 string
	PlaybackURL                                          string // 生成时的播放地址，旧版迁移记录可为空。
	PlaybackToken                                        string
	SizeBytes                                            int64
	GeneratedAt                                          time.Time
	LibraryName                                          *string `json:"-"`
}

// WriteIntent 保存待写入的地址和文件原内容，用来恢复中断的写入。
type WriteIntent struct {
	Artifact        Artifact
	PreviousContent *string
	Legacy          bool
}

type ListFilter struct {
	LibraryID, Query, Sort, Order string
	Page, Limit                   int
}

type ArtifactPage struct {
	Items       []Artifact
	Total       int64
	Page, Limit int
}
