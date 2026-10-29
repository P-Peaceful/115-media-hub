package drive115

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

// ListDirectory 只请求一个分页。cur=1 用于把搜索与类型筛选限制在 cid
// 当前目录，避免 115 在部分筛选条件下递归遍历子目录。
func (d *Driver) ListDirectory(ctx context.Context, credential Credential, directoryID string, offset, limit int, kind, query string) (DirectoryPage, error) {
	if err := ctx.Err(); err != nil {
		return DirectoryPage{}, err
	}
	client := d.client(ctx).ImportCredential(&driver.Credential{UID: credential.UID, CID: credential.CID, SEID: credential.SEID, KID: credential.KID})
	params := url.Values{
		// 与锁定版本 SDK 的 GetFiles 默认参数保持一致。115 的接口
		// 依赖这些参数选择标准目录列表响应；排序参数名是 o，不是 order。
		"aid":              {"1"},
		"cid":              {directoryID},
		"o":                {"user_ptime"},
		"asc":              {"1"},
		"offset":           {strconv.Itoa(offset)},
		"show_dir":         {"1"},
		"limit":            {strconv.Itoa(limit)},
		"snap":             {"0"},
		"natsort":          {"0"},
		"record_open_time": {"1"},
		"format":           {"json"},
		"fc_mix":           {"0"},
		"cur":              {"1"},
	}
	if query != "" {
		params.Set("search_value", query)
	}
	if kind != "all" {
		upstreamType, ok := upstreamType(kind)
		if !ok {
			return DirectoryPage{}, ErrInvalidBrowseFilter
		}
		params.Set("type", upstreamType)
	}
	var response driver.FileListResp
	rest, err := client.NewRequest().SetQueryParamsFromValues(params).ForceContentType("application/json;charset=UTF-8").SetResult(&response).Get(driver.ApiFileList)
	if err := driver.CheckErr(err, &response, rest); err != nil {
		return DirectoryPage{}, safeBrowseError(err)
	}
	page := DirectoryPage{Entries: make([]DirectoryEntry, 0, len(response.Files)), Total: int64(response.Count)}
	for i := range response.Files {
		var file driver.File
		file.From(&response.Files[i])
		page.Entries = append(page.Entries, DirectoryEntry{
			ID: file.FileID, ParentID: file.ParentID, Name: file.Name,
			IsDirectory: file.IsDirectory, Size: max(file.Size, 0),
			ModifiedAt: file.UpdateTime.UTC(), FileType: fileType(file.Name, file.IsDirectory),
		})
	}
	return page, nil
}

// StatDirectory 读取当前目录和父级路径，用于生成面包屑。
func (d *Driver) StatDirectory(ctx context.Context, credential Credential, directoryID string) (Directory, error) {
	if directoryID == "0" {
		return Directory{ID: "0", Name: "我的网盘", Breadcrumbs: []Breadcrumb{{ID: "0", Name: "我的网盘"}}}, nil
	}
	if err := ctx.Err(); err != nil {
		return Directory{}, err
	}
	client := d.client(ctx).ImportCredential(&driver.Credential{UID: credential.UID, CID: credential.CID, SEID: credential.SEID, KID: credential.KID})
	info, err := client.Stat(directoryID)
	if err != nil {
		return Directory{}, safeBrowseError(err)
	}
	if info == nil || !info.IsDirectory {
		return Directory{}, ErrDirectoryNotFound
	}
	crumbs := make([]Breadcrumb, 0, len(info.Parents)+2)
	crumbs = append(crumbs, Breadcrumb{ID: "0", Name: "我的网盘"})
	for _, parent := range info.Parents {
		if parent == nil || parent.ID == "" || parent.ID == "0" {
			continue
		}
		// Stat 返回的父级路径按上游顺序排列；部分响应会包含根目录，需去重。
		if crumbs[len(crumbs)-1].ID != parent.ID {
			crumbs = append(crumbs, Breadcrumb{ID: parent.ID, Name: parent.Name})
		}
	}
	if crumbs[len(crumbs)-1].ID != directoryID {
		crumbs = append(crumbs, Breadcrumb{ID: directoryID, Name: info.Name})
	}
	return Directory{ID: directoryID, Name: info.Name, Breadcrumbs: crumbs}, nil
}

func upstreamType(kind string) (string, bool) {
	// 这些数字对应锁定版本 SDK 中 /files 接口的文件类型筛选值。
	switch kind {
	case "folder":
		return "1", true
	case "document":
		return "2", true
	case "image":
		return "3", true
	case "video":
		return "4", true
	case "archive":
		return "6", true
	case "audio":
		return "5", true
	default:
		return "", false
	}
}

func fileType(name string, directory bool) string {
	if directory {
		return "folder"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mkv", ".mp4", ".avi", ".mov", ".wmv", ".m4v", ".ts", ".webm":
		return "video"
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp":
		return "image"
	case ".srt", ".ass", ".ssa", ".sub", ".vtt":
		return "subtitle"
	case ".nfo", ".xml", ".json":
		return "metadata"
	default:
		return "other"
	}
}

func safeBrowseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, driver.ErrNotLogin) || errors.Is(err, driver.ErrCredentialInvalid) || errors.Is(err, driver.ErrBadCookie) {
		return &UpstreamError{Message: "115 登录凭据无效或已过期", Cause: ErrCredentialInvalid}
	}
	if errors.Is(err, driver.ErrNotExist) {
		return ErrDirectoryNotFound
	}
	if isTransportError(err) || errors.Is(err, driver.ErrUnexpected) {
		return safeNetworkError(err)
	}
	// 只有 Stat 明确返回“非目录”时才认定目录不存在。无法识别的 SDK
	// 错误可能包含上游响应正文，统一映射为脱敏的网络/服务错误，不能误报 404。
	return safeNetworkError(err)
}
