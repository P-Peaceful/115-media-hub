// scan.go：获取正式同步的可信递归快照，严格核对根锚点、文件分页和目录祖先链；不可信临时结果整体丢弃。

package provider115

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	adapter "github.com/wenzhengcheng/115-media-hub/backend/internal/adapter/provider115"
)

// ErrFastUnavailable 表示快速能力不可用或快照不完整，只允许调用方一次标准回退。
var ErrFastUnavailable = errors.New("115 fast snapshot unavailable")

// TreeDirectory 保存已验证的子树目录及完整路径，供 STRM 共用路径和排除规则。
type TreeDirectory struct {
	ID, ParentID string
	Path         []FilePath
}

// TreeFile 保存源文件及其父目录稳定 ID，不向外暴露 Cookie。
type TreeFile struct {
	DirectoryID string
	Item        FileItem
}

// TreeSnapshot 仅在所有分页、目录链和结束锚点均核对成功后返回。
type TreeSnapshot struct {
	AccountID   string
	Directories []TreeDirectory
	Files       []TreeFile
}

// RapidScan 获取一次完整递归快照；取消、限流、授权及临时网络失败不会伪装为可降级错误。
func (s *Service) RapidScan(ctx context.Context, root, requestID string) (TreeSnapshot, error) {
	if !s.fastScan || root == "0" {
		return TreeSnapshot{}, ErrFastUnavailable
	}
	repo, ok := s.repo.(credentialRepository)
	if !ok {
		return TreeSnapshot{}, ErrUpstream
	}
	cookie, err := repo.ProviderCredential(ctx)
	if err != nil {
		return TreeSnapshot{}, err
	}
	if cookie == "" {
		return TreeSnapshot{}, ErrUnconfigured
	}
	anchor, err := s.ScanFiles(ctx, FileQuery{DirectoryID: root, Limit: 1, Sort: "name", Order: "asc", Type: "all"}, requestID)
	if err != nil {
		return TreeSnapshot{}, err
	}
	data, err := s.fastRequest(ctx, s.categoryURL+"?"+url.Values{"cid": {root}}.Encode(), cookie, adapter.Metadata, 1<<20)
	if err != nil {
		return TreeSnapshot{}, fastRequestError(err)
	}
	var meta struct {
		FolderCount json.RawMessage `json:"folder_count"`
		State       *upstreamState  `json:"state"`
		FileID      json.RawMessage `json:"file_id"`
		Category    json.RawMessage `json:"file_category"`
		PickCode    string          `json:"pick_code"`
		Paths       []struct {
			ID   json.RawMessage `json:"file_id"`
			Name string          `json:"file_name"`
		} `json:"paths"`
	}
	if json.Unmarshal(data, &meta) != nil || (meta.State != nil && !bool(*meta.State)) || (len(meta.FileID) > 0 && rawID(meta.FileID) != root) || parseInteger(meta.Category) == nil || *parseInteger(meta.Category) != 0 || meta.PickCode == "" {
		return TreeSnapshot{}, ErrFastUnavailable
	}
	path := make([]FilePath, 0, len(meta.Paths))
	for _, p := range meta.Paths {
		path = append(path, FilePath{ID: rawID(p.ID), Name: p.Name})
	}
	if !reflect.DeepEqual(path, anchor.Path) {
		return TreeSnapshot{}, ErrFastUnavailable
	}
	seenPath := make(map[string]bool)
	if len(anchor.Path) > 256 {
		return TreeSnapshot{}, ErrFastUnavailable
	}
	for _, part := range anchor.Path {
		if !validSourceID(part.ID) || !safeSourceName(part.Name) || seenPath[part.ID] {
			return TreeSnapshot{}, ErrFastUnavailable
		}
		seenPath[part.ID] = true
	}
	snapshot := TreeSnapshot{AccountID: anchor.AccountID, Directories: []TreeDirectory{{ID: root, Path: anchor.Path}}}
	nodes := map[string]treeNode{root: {name: anchor.Path[len(anchor.Path)-1].Name}}
	for page := 1; page <= 1000; page++ {
		address := s.directoriesURL + "?" + url.Values{"pickcode": {meta.PickCode}, "page": {strconv.Itoa(page)}, "per_page": {"5000"}}.Encode()
		data, err = s.fastRequest(ctx, address, cookie, adapter.Directories, 8<<20)
		if err != nil {
			return TreeSnapshot{}, fastRequestError(err)
		}
		var response struct {
			State upstreamState `json:"state"`
			Data  struct {
				List []struct {
					ID     json.RawMessage `json:"fid"`
					Name   string          `json:"fn"`
					Parent json.RawMessage `json:"pid"`
				} `json:"list"`
				HasNext *bool `json:"has_next_page"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &response) != nil || !bool(response.State) || response.Data.HasNext == nil || len(response.Data.List) > 5000 || len(nodes)+len(response.Data.List) > 1000000 || (*response.Data.HasNext && len(response.Data.List) != 5000) {
			return TreeSnapshot{}, ErrFastUnavailable
		}
		for _, entry := range response.Data.List {
			id, parent := rawID(entry.ID), rawID(entry.Parent)
			if !validSourceID(id) || !validSourceID(parent) || !safeSourceName(entry.Name) {
				return TreeSnapshot{}, ErrFastUnavailable
			}
			if id == root {
				if entry.Name != nodes[root].name || len(anchor.Path) < 2 || parent != anchor.Path[len(anchor.Path)-2].ID {
					return TreeSnapshot{}, ErrFastUnavailable
				}
				continue
			}
			if _, duplicate := nodes[id]; duplicate {
				return TreeSnapshot{}, ErrFastUnavailable
			}
			nodes[id] = treeNode{name: entry.Name, parent: parent}
		}
		if !*response.Data.HasNext {
			break
		}
		if page == 1000 {
			return TreeSnapshot{}, ErrFastUnavailable
		}
	}
	if len(meta.FolderCount) > 0 {
		count := parseInteger(meta.FolderCount)
		if count == nil || *count != int64(len(nodes)-1) {
			return TreeSnapshot{}, ErrFastUnavailable
		}
	}
	paths := map[string][]FilePath{root: anchor.Path}
	for id, node := range nodes {
		if id == root {
			continue
		}
		p, ok := resolveTreePath(id, root, nodes, paths)
		if !ok {
			return TreeSnapshot{}, ErrFastUnavailable
		}
		snapshot.Directories = append(snapshot.Directories, TreeDirectory{ID: id, ParentID: node.parent, Path: p})
	}
	seen := make(map[string]bool)
	total := int64(-1)
	for offset, pages := 0, 0; pages < 10000; pages++ {
		values := url.Values{"aid": {"1"}, "cid": {root}, "show_dir": {"0"}, "limit": {"1150"}, "offset": {strconv.Itoa(offset)}, "o": {"file_name"}, "asc": {"1"}, "format": {"json"}}
		data, err = s.fastRequest(ctx, s.filesURL+"?"+values.Encode(), cookie, adapter.FastFiles, 4<<20)
		if err != nil {
			return TreeSnapshot{}, fastRequestError(err)
		}
		var response struct {
			State upstreamState   `json:"state"`
			CID   json.RawMessage `json:"cid"`
			Count *int64          `json:"count"`
			Path  []struct {
				ID   json.RawMessage `json:"cid"`
				Name string          `json:"name"`
			} `json:"path"`
			Data []struct {
				ID     json.RawMessage `json:"fid"`
				Parent json.RawMessage `json:"cid"`
				Name   string          `json:"n"`
				Code   string          `json:"pc"`
				Size   json.RawMessage `json:"s"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &response) != nil || !bool(response.State) || rawID(response.CID) != root || response.Count == nil || *response.Count < 0 || *response.Count > 1000000 || len(response.Data) > 1150 {
			return TreeSnapshot{}, ErrFastUnavailable
		}
		currentPath := make([]FilePath, 0, len(response.Path))
		for _, p := range response.Path {
			currentPath = append(currentPath, FilePath{ID: rawID(p.ID), Name: p.Name})
		}
		if !reflect.DeepEqual(currentPath, anchor.Path) {
			return TreeSnapshot{}, ErrFastUnavailable
		}
		if total < 0 {
			total = *response.Count
		}
		if total != *response.Count || int64(offset+len(response.Data)) > total || (int64(offset+len(response.Data)) < total && len(response.Data) != 1150) {
			return TreeSnapshot{}, ErrFastUnavailable
		}
		for _, entry := range response.Data {
			id, parent := rawID(entry.ID), rawID(entry.Parent)
			if !validSourceID(id) || !safeSourceName(entry.Name) || seen[id] || paths[parent] == nil {
				return TreeSnapshot{}, ErrFastUnavailable
			}
			seen[id] = true
			item := FileItem{ID: id, Name: entry.Name, FileType: classifyFile(entry.Name), PlaybackCode: entry.Code}
			if size, ok := parseBytes(entry.Size); ok {
				item.SizeBytes = &size
			}
			snapshot.Files = append(snapshot.Files, TreeFile{DirectoryID: parent, Item: item})
		}
		offset += len(response.Data)
		if int64(offset) == total {
			break
		}
		if pages == 9999 {
			return TreeSnapshot{}, ErrFastUnavailable
		}
	}
	end, err := s.ScanFiles(ctx, FileQuery{DirectoryID: root, Limit: 1, Sort: "name", Order: "asc", Type: "all"}, requestID)
	if err != nil {
		return TreeSnapshot{}, err
	}
	if !reflect.DeepEqual(end.Path, anchor.Path) || end.AccountID != anchor.AccountID || end.Total != anchor.Total {
		return TreeSnapshot{}, ErrFastUnavailable
	}
	if err = s.checkCredential(cookie)(ctx); err != nil {
		return TreeSnapshot{}, err
	}
	return snapshot, nil
}

// treeNode 保存不可信目录表节点，只有解析到锚点之后才能被采用。
type treeNode struct{ name, parent string }

// resolveTreePath 有界回溯并缓存目录路径，拒绝环、断链及子树之外的父节点。
func resolveTreePath(id, root string, nodes map[string]treeNode, cache map[string][]FilePath) ([]FilePath, bool) {
	if p := cache[id]; p != nil {
		if len(p) > 256 {
			return nil, false
		}
		return p, true
	}
	seen := map[string]bool{}
	var chain []string
	current := id
	for cache[current] == nil {
		if seen[current] || len(chain) >= 256 {
			return nil, false
		}
		seen[current] = true
		node, ok := nodes[current]
		if !ok || current == root {
			return nil, false
		}
		chain = append(chain, current)
		current = node.parent
	}
	// 缓存祖先也计入完整深度，逐层命中缓存不能绕过祖先链上限。
	if len(cache[current])+len(chain) > 256 {
		return nil, false
	}
	path := append([]FilePath(nil), cache[current]...)
	for i := len(chain) - 1; i >= 0; i-- {
		node := nodes[chain[i]]
		path = append(path, FilePath{ID: chain[i], Name: node.name})
		cache[chain[i]] = append([]FilePath(nil), path...)
	}
	return cache[id], true
}

// rawID 只允许十进制非负稳定 ID，避免 JSON 数字经过浮点数。
func rawID(raw json.RawMessage) string {
	if id := parseID(raw); id != nil && validSourceID(*id) {
		return *id
	}
	return ""
}

// validSourceID 限制协议身份为最多 32 位十进制数字。
func validSourceID(id string) bool {
	return len(id) > 0 && len(id) <= 32 && strings.Trim(id, "0123456789") == ""
}

// safeSourceName 拒绝可能越过目录边界的名称，不归一化上游名称。
func safeSourceName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && utf8.ValidString(name) && !strings.ContainsAny(name, "/\\") && strings.IndexFunc(name, unicode.IsControl) == -1
}

// fastRequest 将快速页纳入同一账号后台配额，响应和尝试均有界。
func (s *Service) fastRequest(ctx context.Context, address, cookie string, operation adapter.Operation, maxBytes int64) ([]byte, error) {
	data, err := s.doRequest(ctx, adapter.Request{Scope: accountScope(cookie), Priority: adapter.Background, Operation: operation, Method: http.MethodGet, URL: address, Header: requestHeaders(cookie, "Mozilla/5.0 115Browser/36.0.0"), Check: s.checkCredential(cookie), MaxBytes: maxBytes, Inspect: s.inspectBusiness(cookie)})
	return data, err
}

// fastRequestError 只把协议不支持或不可信响应归为可回退，限流和授权保留原错误身份。
func fastRequestError(err error) error {
	if errors.Is(err, ErrUpstream) || errors.Is(err, ErrCredentialChanged) || errors.Is(err, ErrInvalidAuthorization) || errors.Is(err, errPlaybackThrottled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrRequestBudgetExhausted) {
		return err
	}
	var e *adapter.Error
	if errors.As(err, &e) && (e.Kind == adapter.Protocol || e.StatusCode == 404 || e.StatusCode == 405 || e.StatusCode == 501) {
		return errors.Join(ErrFastUnavailable, err)
	}
	return mapRequestError(err, ErrDirectoryNotFound)
}
