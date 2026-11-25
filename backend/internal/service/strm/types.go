// types.go：定义 STRM 服务消费的仓储和网盘接口，以及登记查询与本地内容详情的数据边界。

package strm

import (
	"context"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/strm"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

var (
	ErrNotFound     = domain.ErrNotFound
	ErrConflict     = domain.ErrConflict
	ErrInvalidQuery = domain.ErrInvalidQuery
)

// Artifact 复用领域层的产物身份与本地归属记录。
type Artifact = domain.Artifact

// WriteIntent 复用领域层的持久化写入恢复信息。
type WriteIntent = domain.WriteIntent

// ListFilter 复用领域层的登记列表过滤和分页条件。
type ListFilter = domain.ListFilter

// ArtifactPage 复用领域层的产物登记分页结果。
type ArtifactPage = domain.ArtifactPage

// Detail 结合产物登记与本地文件状态，仅在内容匹配时提供预览。
type Detail struct {
	Artifact
	FileState string
	Content   *string
}

// Repository 提供产物归属和写入意图存储；完成意图须同时登记产物并移除意图。
type Repository interface {
	StrmArtifactByTarget(context.Context, string) (Artifact, error)
	StrmIntentByTarget(context.Context, string) (WriteIntent, error)
	SaveStrmIntent(context.Context, WriteIntent) error
	CompleteStrmIntent(context.Context, WriteIntent) error
	DiscardStrmIntent(context.Context, string) error
	StrmIntentsByLibrary(context.Context, string) ([]WriteIntent, error)
	ListStrmArtifacts(context.Context, ListFilter) (ArtifactPage, error)
	StrmArtifact(context.Context, string) (Artifact, error)
}

// Provider 提供只读目录和按源账号解析播放链接的最小上游边界。
type Provider interface {
	Files(context.Context, provider115.FileQuery, string) (provider115.FilePage, error)
	ResolvePlayback(ctx context.Context, accountID, sourceFileID, playbackCode, userAgent, requestID string) (string, error)
}
