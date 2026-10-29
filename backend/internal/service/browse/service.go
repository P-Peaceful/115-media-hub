package browse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/drive115"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/repository/sqlite/model"
	"gorm.io/gorm"
)

var ErrConnectionMissing = errors.New("115 connection not configured")
var ErrDirectoryNotFound = errors.New("115 directory not found")
var ErrInvalidRequest = errors.New("invalid directory browse request")
var ErrUnavailable = errors.New("115 directory browse unavailable")

type Entry struct {
	ID, ParentID, Name string
	IsDirectory        bool
	Size               int64
	ModifiedAt         *time.Time
	FileType           string
}
type Page struct {
	DirectoryID   string
	Offset, Limit int
	Total         int64
	Entries       []Entry
}
type Breadcrumb struct{ ID, Name string }
type Directory struct {
	ID, Name    string
	Breadcrumbs []Breadcrumb
}

type Service struct {
	repo    sqlite.ConnectionRepository
	adapter drive115.BrowserAdapter
}

func New(db *gorm.DB, adapter drive115.BrowserAdapter) *Service {
	return &Service{repo: sqlite.ConnectionRepository{DB: db}, adapter: adapter}
}

func (s *Service) Entries(ctx context.Context, directoryID string, offset, limit int, kind, query string) (page Page, err error) {
	started := time.Now()
	// 只记录关键词是否存在，不写入关键词本身、名称或上游响应内容。
	defer func() {
		fields := []any{"directory_id", directoryID, "offset", offset, "limit", limit, "type", kind, "has_query", query != "", "duration_ms", time.Since(started).Milliseconds()}
		if err != nil {
			slog.ErrorContext(ctx, "115 directory page failed", append(fields, "category", errorCategory(err))...)
			return
		}
		slog.InfoContext(ctx, "115 directory page loaded", append(fields, "returned_count", len(page.Entries), "total", page.Total)...)
	}()
	if !validDirectoryID(directoryID) || offset < 0 || limit < 1 || limit > 100 || len(query) > 120 || !validType(kind) {
		return Page{}, ErrInvalidRequest
	}
	connection, err := s.repo.Active115(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("load active 115 connection: %w", err)
	}
	credential, err := credentialFrom(connection)
	if err != nil {
		return Page{}, err
	}
	defer clearCredential(&credential)
	remote, err := s.adapter.ListDirectory(ctx, credential, directoryID, offset, limit, kind, query)
	if err != nil {
		if errors.Is(err, drive115.ErrDirectoryNotFound) {
			return Page{}, ErrDirectoryNotFound
		}
		return Page{}, fmt.Errorf("read 115 directory page: %w", err)
	}
	page = Page{DirectoryID: directoryID, Offset: offset, Limit: limit, Total: remote.Total, Entries: make([]Entry, 0, len(remote.Entries))}
	for _, item := range remote.Entries {
		var modified *time.Time
		if !item.ModifiedAt.IsZero() {
			value := item.ModifiedAt
			modified = &value
		}
		page.Entries = append(page.Entries, Entry{ID: item.ID, ParentID: item.ParentID, Name: item.Name, IsDirectory: item.IsDirectory, Size: item.Size, ModifiedAt: modified, FileType: item.FileType})
	}
	return page, nil
}

func (s *Service) Directory(ctx context.Context, directoryID string) (directory Directory, err error) {
	started := time.Now()
	defer func() {
		fields := []any{"directory_id", directoryID, "duration_ms", time.Since(started).Milliseconds()}
		if err != nil {
			slog.ErrorContext(ctx, "115 directory metadata failed", append(fields, "category", errorCategory(err))...)
			return
		}
		slog.InfoContext(ctx, "115 directory metadata loaded", append(fields, "breadcrumb_count", len(directory.Breadcrumbs))...)
	}()
	if !validDirectoryID(directoryID) {
		return Directory{}, ErrInvalidRequest
	}
	if directoryID == "0" {
		return Directory{ID: "0", Name: "我的网盘", Breadcrumbs: []Breadcrumb{{ID: "0", Name: "我的网盘"}}}, nil
	}
	connection, err := s.repo.Active115(ctx)
	if err != nil {
		return Directory{}, fmt.Errorf("load active 115 connection: %w", err)
	}
	credential, err := credentialFrom(connection)
	if err != nil {
		return Directory{}, err
	}
	defer clearCredential(&credential)
	remote, err := s.adapter.StatDirectory(ctx, credential, directoryID)
	if err != nil {
		if errors.Is(err, drive115.ErrDirectoryNotFound) {
			return Directory{}, ErrDirectoryNotFound
		}
		return Directory{}, fmt.Errorf("read 115 directory metadata: %w", err)
	}
	crumbs := make([]Breadcrumb, 0, len(remote.Breadcrumbs))
	for _, crumb := range remote.Breadcrumbs {
		crumbs = append(crumbs, Breadcrumb{ID: crumb.ID, Name: crumb.Name})
	}
	return Directory{ID: remote.ID, Name: remote.Name, Breadcrumbs: crumbs}, nil
}

func credentialFrom(connection *model.Connection) (drive115.Credential, error) {
	if connection == nil {
		return drive115.Credential{}, ErrConnectionMissing
	}
	var credential drive115.Credential
	if err := json.Unmarshal([]byte(connection.CredentialJSON), &credential); err != nil {
		return drive115.Credential{}, ErrUnavailable
	}
	if credential.UID == "" || credential.CID == "" || credential.SEID == "" || credential.KID == "" {
		return drive115.Credential{}, ErrUnavailable
	}
	return credential, nil
}

func clearCredential(credential *drive115.Credential) {
	// 凭证只在本次请求中使用，完成调用后清空内存中的字符串引用。
	credential.UID, credential.CID, credential.SEID, credential.KID = "", "", "", ""
}

func validDirectoryID(id string) bool {
	if id == "" || (len(id) > 1 && id[0] == '0') {
		return false
	}
	value, err := strconv.ParseUint(id, 10, 64)
	return err == nil && value <= uint64(^uint64(0)>>1)
}
func validType(kind string) bool {
	switch kind {
	case "all", "video", "folder", "image", "document", "audio", "archive":
		return true
	default:
		return false
	}
}
func errorCategory(err error) string {
	switch {
	case errors.Is(err, ErrConnectionMissing):
		return "connection_missing"
	case errors.Is(err, ErrDirectoryNotFound):
		return "directory_not_found"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, drive115.ErrCredentialInvalid):
		return "credential_invalid"
	case errors.Is(err, drive115.ErrUnreachable):
		return "upstream_unreachable"
	case errors.Is(err, drive115.ErrLimited):
		return "upstream_limited"
	default:
		return "internal"
	}
}
