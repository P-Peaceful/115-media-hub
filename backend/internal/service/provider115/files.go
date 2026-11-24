package provider115

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ErrUnconfigured = errors.New("115 account not configured")
var ErrInvalidAuthorization = errors.New("115 authorization invalid")
var ErrDirectoryNotFound = errors.New("115 directory not found")
var ErrInvalidQuery = errors.New("invalid directory query")

type FileQuery struct {
	DirectoryID string
	Limit       int
	Cursor      string
	Page        int64
	Sort        string
	Order       string
	Type        string
}
type FilePath struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type FileItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	IsDirectory  bool       `json:"is_directory"`
	SizeBytes    *string    `json:"size_bytes"`
	ModifiedAt   *time.Time `json:"modified_at"`
	FileType     string     `json:"file_type"`
	PlaybackCode string     `json:"-"`
}
type FilePage struct {
	DirectoryID string     `json:"directory_id"`
	Path        []FilePath `json:"path"`
	Items       []FileItem `json:"items"`
	NextCursor  *string    `json:"next_cursor"`
	Total       int64      `json:"total"`
	Page        *int64     `json:"page"`
	PageSize    int        `json:"page_size"`
	TotalPages  int64      `json:"total_pages"`
	AccountID   string     `json:"-"`
}
type fileCursor struct {
	DirectoryID string `json:"d"`
	Offset      int    `json:"o"`
	Limit       int    `json:"l"`
	Sort        string `json:"s"`
	Order       string `json:"r"`
	Type        string `json:"t"`
}

// Files returns a provider page. Offset pagination reflects the live directory;
// concurrent changes can move entries between pages, so refresh restarts browsing.
func (s *Service) Files(ctx context.Context, q FileQuery, requestID string) (result FilePage, resultErr error) {
	start := time.Now()
	defer func() { s.log(ctx, "list_files", requestID, start, resultErr) }()
	sorts := map[string]string{"name": "file_name", "size": "file_size", "modified_at": "user_utime"}
	types := map[string]string{"all": "0", "document": "1", "image": "2", "audio": "3", "video": "4"}
	if q.DirectoryID == "" || len(q.DirectoryID) > 32 || strings.Trim(q.DirectoryID, "0123456789") != "" || q.Limit < 1 || q.Limit > 200 || sorts[q.Sort] == "" || types[q.Type] == "" || (q.Order != "asc" && q.Order != "desc") {
		return result, ErrInvalidQuery
	}
	const maxSafeInteger int64 = 9007199254740991
	if q.Page < 0 || q.Page > maxSafeInteger || (q.Page > 0 && q.Cursor != "") || (q.Page > 0 && q.Page-1 > maxSafeInteger/int64(q.Limit)) {
		return result, ErrInvalidQuery
	}
	offset := 0
	if q.Page > 0 {
		offset = int((q.Page - 1) * int64(q.Limit))
	}
	if q.Cursor != "" {
		if len(q.Cursor) > 512 {
			return result, ErrInvalidQuery
		}
		raw, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		var cursor fileCursor
		if err != nil || len(raw) > 512 || json.Unmarshal(raw, &cursor) != nil || cursor.Offset < 0 || int64(cursor.Offset) > maxSafeInteger || cursor.DirectoryID != q.DirectoryID || cursor.Limit != q.Limit || cursor.Sort != q.Sort || cursor.Order != q.Order || cursor.Type != q.Type {
			return result, ErrInvalidQuery
		}
		offset = cursor.Offset
	}
	repo, ok := s.repo.(credentialRepository)
	if !ok {
		return result, ErrUpstream
	}
	cookie, err := repo.ProviderCredential(ctx)
	if err != nil {
		return result, err
	}
	if cookie == "" {
		return result, ErrUnconfigured
	}
	defer func() {
		current, e := repo.ProviderCredential(ctx)
		if e != nil || current != cookie {
			result = FilePage{}
			resultErr = ErrUpstream
		}
	}()
	asc := "1"
	if q.Order == "desc" {
		asc = "0"
	}
	values := url.Values{"aid": {"1"}, "cid": {q.DirectoryID}, "offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(q.Limit)}, "cur": {"1"}, "custom_order": {"1"}, "o": {sorts[q.Sort]}, "asc": {asc}, "type": {types[q.Type]}}
	body, err := s.accountRequest(ctx, s.filesURL+"?"+values.Encode(), cookie)
	if err != nil {
		return result, ErrUpstream
	}
	var response struct {
		State upstreamState     `json:"state"`
		Error string            `json:"error"`
		CID   json.RawMessage   `json:"cid"`
		Count *int64            `json:"count"`
		Data  []json.RawMessage `json:"data"`
		Path  []struct {
			CID  json.RawMessage `json:"cid"`
			Name string          `json:"name"`
		} `json:"path"`
	}
	if json.Unmarshal(body, &response) != nil {
		return result, ErrUpstream
	}
	if !bool(response.State) {
		if strings.Contains(response.Error, "请先登录") || strings.Contains(response.Error, "未登录") {
			return result, ErrInvalidAuthorization
		}
		return result, ErrUpstream
	}
	if response.Count == nil || *response.Count < 0 || *response.Count > 9007199254740991 || len(response.Data) > q.Limit || int64(len(response.Data)) > *response.Count || (int64(offset) < *response.Count && len(response.Data) == 0) || (len(response.Data) > 0 && int64(offset)+int64(len(response.Data)) > *response.Count) || (q.Page > 0 && int64(offset)+int64(len(response.Data)) < *response.Count && len(response.Data) != q.Limit) {
		return result, ErrUpstream
	}
	id := parseID(response.CID)
	if id == nil || *id != q.DirectoryID {
		return result, ErrDirectoryNotFound
	}
	totalPages := (*response.Count / int64(q.Limit))
	if *response.Count%int64(q.Limit) != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	var currentPage *int64
	if offset%q.Limit == 0 && int64(offset/q.Limit) < maxSafeInteger {
		page := int64(offset/q.Limit) + 1
		currentPage = &page
	}
	result = FilePage{DirectoryID: q.DirectoryID, Path: []FilePath{}, Items: []FileItem{}, Total: *response.Count, Page: currentPage, PageSize: q.Limit, TotalPages: totalPages, AccountID: credentialAccountID(cookie)}
	for _, p := range response.Path {
		id := parseID(p.CID)
		if id == nil || p.Name == "" {
			return FilePage{}, ErrUpstream
		}
		result.Path = append(result.Path, FilePath{ID: *id, Name: p.Name})
	}
	if len(result.Path) == 0 || result.Path[0].ID != "0" || result.Path[len(result.Path)-1].ID != q.DirectoryID {
		return FilePage{}, ErrUpstream
	}
	for _, raw := range response.Data {
		var entry struct {
			FID      json.RawMessage `json:"fid"`
			CID      json.RawMessage `json:"cid"`
			Name     string          `json:"n"`
			Size     json.RawMessage `json:"s"`
			Modified json.RawMessage `json:"te"`
			PickCode json.RawMessage `json:"pc"`
		}
		if json.Unmarshal(raw, &entry) != nil || entry.Name == "" {
			return FilePage{}, ErrUpstream
		}
		id := parseID(entry.FID)
		directory := id == nil
		if directory {
			id = parseID(entry.CID)
		}
		if id == nil {
			return FilePage{}, ErrUpstream
		}
		item := FileItem{ID: *id, Name: entry.Name, IsDirectory: directory, ModifiedAt: parseUnixTime(entry.Modified), FileType: "file"}
		if directory {
			item.FileType = "directory"
		} else {
			if size, ok := parseBytes(entry.Size); ok {
				item.SizeBytes = &size
			}
			item.FileType = classifyFile(entry.Name)
			item.PlaybackCode = parseOptionalString(entry.PickCode)
		}
		result.Items = append(result.Items, item)
	}
	if len(response.Data) > 0 && int64(offset+len(response.Data)) < *response.Count {
		raw, _ := json.Marshal(fileCursor{q.DirectoryID, offset + len(response.Data), q.Limit, q.Sort, q.Order, q.Type})
		cursor := base64.RawURLEncoding.EncodeToString(raw)
		result.NextCursor = &cursor
	}
	return result, nil
}

// credentialAccountID 提取 UID 的账号部分，使同一用户重新扫码后已有产物仍能播放。
func credentialAccountID(cookie string) string {
	for field := range strings.SplitSeq(cookie, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(field), "=")
		if !found || name != "UID" {
			continue
		}
		id, _, _ := strings.Cut(value, "_")
		if len(id) <= 32 && strings.Trim(id, "0123456789") == "" {
			return id
		}
		return ""
	}
	return ""
}

func classifyFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".ts", ".m4v", ".webm":
		return "video"
	case ".mp3", ".flac", ".wav", ".aac", ".m4a", ".ogg":
		return "audio"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return "image"
	case ".srt", ".ass", ".ssa", ".vtt", ".sub":
		return "subtitle"
	case ".pdf", ".txt", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return "document"
	default:
		return "file"
	}
}
