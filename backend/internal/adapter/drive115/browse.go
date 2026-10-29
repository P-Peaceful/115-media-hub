package drive115

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type pathNode struct {
	CID  wireInteger `json:"cid"`
	PID  wireInteger `json:"pid"`
	AID  wireInteger `json:"aid"`
	Name string      `json:"name"`
}
type listing struct {
	CID   wireInteger  `json:"cid"`
	AID   wireInteger  `json:"aid"`
	Path  []pathNode   `json:"path"`
	Count *wireInteger `json:"count"`
	Data  []fileEntry  `json:"data"`
}
type fileEntry struct {
	FID  wireInteger `json:"fid"`
	CID  wireInteger `json:"cid"`
	PID  wireInteger `json:"pid"`
	F    wireInteger `json:"f"`
	Name string      `json:"n"`
	Size byteSize    `json:"s"`
	Time jsonText    `json:"t"`
}

// jsonText supports timestamps represented as text or integers.
type jsonText string

func (v *jsonText) UnmarshalJSON(data []byte) error {
	var s string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
	} else {
		var n wireInteger
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		s = string(n)
	}
	*v = jsonText(s)
	return nil
}
func listParams(cid string, offset, limit int) url.Values {
	return url.Values{
		"aid": {"1"}, "cid": {cid}, "o": {"user_ptime"}, "asc": {"1"}, "offset": {strconv.Itoa(offset)}, "show_dir": {"1"}, "limit": {strconv.Itoa(limit)}, "snap": {"0"}, "natsort": {"0"}, "record_open_time": {"1"}, "format": {"json"}, "fc_mix": {"0"}, "cur": {"1"},
	}
}
func validateListing(r listing, cid string, requirePath bool) error {
	if r.AID != "" && r.AID != "1" {
		return ErrDirectoryNotFound
	}
	if r.CID != "" && string(r.CID) != cid {
		return ErrDirectoryNotFound
	}
	if len(r.Path) > 0 {
		last := r.Path[len(r.Path)-1]
		if string(last.CID) != cid || last.AID != "" && last.AID != "1" {
			return ErrDirectoryNotFound
		}
	}
	if requirePath && len(r.Path) == 0 {
		return protocolError()
	}
	if r.CID == "" && len(r.Path) == 0 && cid != "0" {
		return protocolError()
	}
	return nil
}
func (d *Driver) StatDirectory(ctx context.Context, cr Credential, cid string) (Directory, error) {
	if err := ctx.Err(); err != nil {
		return Directory{}, err
	}
	if cid == "0" {
		return Directory{ID: "0", Name: "我的网盘", Breadcrumbs: []Breadcrumb{{ID: "0", Name: "我的网盘"}}}, nil
	}
	params := listParams(cid, 0, 1)
	params.Set("hide_data", "1")
	params.Set("nf", "1")
	params.Set("count_folders", "1")
	var r listing
	if err := d.getJSON(ctx, filesAPI, params, cr, &r); err != nil {
		return Directory{}, err
	}
	if err := validateListing(r, cid, true); err != nil {
		return Directory{}, err
	}
	crumbs := []Breadcrumb{{ID: "0", Name: "我的网盘"}}
	seen := map[string]bool{"0": true}
	for _, node := range r.Path {
		id := string(node.CID)
		if id == "0" {
			continue
		}
		if id == "" || strings.HasPrefix(id, "-") || node.Name == "" || seen[id] || node.AID != "" && node.AID != "1" {
			return Directory{}, protocolError()
		}
		seen[id] = true
		crumbs = append(crumbs, Breadcrumb{ID: id, Name: node.Name})
	}
	return Directory{ID: cid, Name: crumbs[len(crumbs)-1].Name, Breadcrumbs: crumbs}, nil
}
func (d *Driver) ListDirectory(ctx context.Context, cr Credential, cid string, offset, limit int, kind, query string) (DirectoryPage, error) {
	if offset < 0 || limit <= 0 || limit > 1150 {
		return DirectoryPage{}, ErrInvalidBrowseFilter
	}
	params := listParams(cid, offset, limit)
	if query != "" {
		params.Set("search_value", query)
	}
	if kind != "all" {
		value, ok := upstreamType(kind)
		if !ok {
			return DirectoryPage{}, ErrInvalidBrowseFilter
		}
		params.Set("type", value)
	}
	var r listing
	if err := d.getJSON(ctx, filesAPI, params, cr, &r); err != nil {
		return DirectoryPage{}, err
	}
	if err := validateListing(r, cid, false); err != nil {
		return DirectoryPage{}, err
	}
	if r.Count == nil || r.Data == nil {
		return DirectoryPage{}, protocolError()
	}
	total, err := r.Count.int64()
	if err != nil || total < 0 {
		return DirectoryPage{}, protocolError()
	}
	page := DirectoryPage{Total: total, Entries: make([]DirectoryEntry, 0, len(r.Data))}
	for _, entry := range r.Data {
		directory := entry.F == "0" || entry.F == "" && entry.FID == ""
		id, parent := string(entry.FID), string(entry.CID)
		if directory {
			id = string(entry.CID)
			parent = string(entry.PID)
		}
		if id == "" || id == "0" || strings.HasPrefix(id, "-") || entry.Name == "" || parent != cid || entry.F != "" && entry.F != "0" && entry.F != "1" {
			return DirectoryPage{}, protocolError()
		}
		modified := time.Time{}
		if seconds, err := strconv.ParseInt(string(entry.Time), 10, 64); err == nil {
			modified = time.Unix(seconds, 0).UTC()
		} else if value, err := time.ParseInLocation("2006-01-02 15:04", string(entry.Time), time.FixedZone("UTC+8", 8*3600)); err == nil {
			modified = value.UTC()
		}
		page.Entries = append(page.Entries, DirectoryEntry{ID: id, ParentID: parent, Name: entry.Name, IsDirectory: directory, Size: int64(entry.Size), ModifiedAt: modified, FileType: fileType(entry.Name, directory)})
	}
	return page, nil
}
func upstreamType(kind string) (string, bool) {
	// These values are the existing /files type filter contract.
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
