// artifact.go：定义 STRM 产物归属、写入恢复意图及列表查询类型，不包含文件系统或数据库实现。

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
	PlaybackToken                                        string // 产物专属播放凭据，不作为公开管理字段输出。
	SizeBytes                                            int64
	GeneratedAt                                          time.Time
	LibraryName                                          *string `json:"-"`
}

// WriteIntent 保存待写入的地址和文件原内容，用来恢复中断的写入。
type WriteIntent struct {
	Artifact        Artifact
	PreviousContent *string // nil 表示首次写入，否则保留覆盖前的完整内容。
	Legacy          bool    // 标记缺少完整原内容合同的旧版迁移意图。
}

// ListFilter 定义登记列表的媒体库、名称、排序及页码条件。
type ListFilter struct {
	LibraryID, Query, Sort, Order string
	Page, Limit                   int
}

// ArtifactPage 保存同一查询快照的产物列表、总数和分页参数。
type ArtifactPage struct {
	Items       []Artifact
	Total       int64
	Page, Limit int
}
