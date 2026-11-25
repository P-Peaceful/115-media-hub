// service.go：负责媒体库配置校验、版本冲突和增删改查，只持久化配置，不操作媒体文件。

// Package library 管理持久化媒体库配置，不直接操作媒体文件。
package library

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Config 保存媒体库来源、输出和同步选项；是否具备执行资格由具体业务模块判断。
type Config struct {
	Name                   string `json:"name"`
	DirectoryID            string `json:"directory_id"`
	DirectoryPath          string `json:"directory_path"`
	MediaType              string `json:"media_type"`
	OutputPath             string `json:"output_path"`
	GenerateSTRM           bool   `json:"generate_strm"`
	STRMExistingFilePolicy string `json:"strm_existing_file_policy"`
	GenerateNFO            bool   `json:"generate_nfo"`
	GeneratePosterCover    bool   `json:"generate_poster_cover"`
	LanguagePreference     string `json:"language_preference"`
	ScrapeSource           string `json:"scrape_source"`
	ExcludeDirectoryPath   string `json:"exclude_directory_path"`
	IncludeSubdirectories  bool   `json:"include_subdirectories"`
	FollowDirectoryChanges bool   `json:"follow_directory_changes"`
	FileEncoding           string `json:"file_encoding"`
	LinkType               string `json:"link_type"`
	STRMDirectDomain       string `json:"strm_direct_domain"`
	STRMLinkFormat         string `json:"strm_link_format"`
	Enabled                bool   `json:"enabled"`
}

// Library 为媒体库配置附加持久化身份、时间戳和乐观并发版本。
type Library struct {
	Config
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Revision  int64     `json:"revision"`
}

// ErrNotFound 表示目标媒体库不存在。
var ErrNotFound = errors.New("library not found")

// ErrConflict 表示媒体库配置已发生并发变化。
var ErrConflict = errors.New("library changed concurrently")

// ErrInvalid 表示配置字段不符合媒体库保存合同。
var ErrInvalid = errors.New("invalid library configuration")

// Repository 提供媒体库配置持久化及版本检查边界，不负责媒体文件操作。
type Repository interface {
	ListLibraries(context.Context, string, int) ([]Library, error)
	GetLibrary(context.Context, string) (Library, error)
	CreateLibrary(context.Context, Library) error
	UpdateLibrary(context.Context, Library) error
	DeleteLibrary(context.Context, string, string, string) error
}

// Service 编排媒体库配置校验与仓储调用，并记录业务结果。
type Service struct {
	repo   Repository
	logger *slog.Logger
}

// New 构造媒体库配置服务并绑定模块日志。
func New(repo Repository, logger *slog.Logger) *Service {
	return &Service{repo, logger.With("module", "library")}
}

var directoryID = regexp.MustCompile(`^[0-9]{1,32}$`)

// validText 校验文本为合法 UTF-8、长度受限且不含 NUL。
func validText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, '\x00')
}

// oneOf 判断配置值是否属于给定枚举集合。
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

// PlaybackOrigin 校验两种链接类型共用的媒体库播放地址。旧配置仅保存主机和端口，默认使用 HTTPS。
// 空地址可以保存，生成任务需要可用的播放地址。
func PlaybackOrigin(address string) (string, bool) {
	if address == "" {
		return "", true
	}
	if !validText(address, 253) || strings.TrimSpace(address) != address || strings.ContainsAny(address, "?#") {
		return "", false
	}
	explicitScheme := strings.Contains(address, "://")
	if !explicitScheme {
		address = "https://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || !oneOf(parsed.Scheme, "http", "https") || parsed.User != nil || parsed.Host == "" ||
		strings.HasSuffix(parsed.Host, ":") || (parsed.Path != "" && (!explicitScheme || parsed.EscapedPath() != "/")) {
		return "", false
	}
	if port := parsed.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", false
		}
	}
	host := parsed.Hostname()
	bracketed := strings.HasPrefix(parsed.Host, "[")
	if bracketed {
		if !strings.Contains(host, ":") || net.ParseIP(host) == nil {
			return "", false
		}
	} else if strings.Contains(host, ":") || !validPlaybackHostname(host) {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

// validPlaybackHostname 校验域名标签长度和 ASCII 字符边界，拒绝空标签及首尾连字符。
func validPlaybackHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

// Validate 校验媒体库保存字段、枚举和播放地址；不探测源目录或本地写权限。
func Validate(c Config) error {
	_, validPlaybackAddress := PlaybackOrigin(c.STRMDirectDomain)
	if strings.TrimSpace(c.Name) == "" || !validText(c.Name, 100) ||
		!directoryID.MatchString(c.DirectoryID) || strings.TrimSpace(c.DirectoryPath) == "" ||
		!validText(c.DirectoryPath, 1024) || !filepath.IsAbs(c.OutputPath) ||
		!validText(c.OutputPath, 1024) || !oneOf(c.MediaType, "movie", "tv", "anime", "documentary", "mixed") ||
		!oneOf(c.LanguagePreference, "zh-CN") || !oneOf(c.ScrapeSource, "tmdb", "metatube", "none") ||
		!validText(c.ExcludeDirectoryPath, 1024) || !oneOf(c.FileEncoding, "UTF-8") ||
		!oneOf(c.LinkType, "direct", "proxy") || !validPlaybackAddress ||
		!oneOf(c.STRMLinkFormat, "original_name", "file_id") ||
		!oneOf(c.STRMExistingFilePolicy, "skip", "overwrite") {
		return ErrInvalid
	}
	return nil
}

// log 记录配置操作的脱敏结果，按校验、冲突、缺失或存储失败分类。
func (s *Service) log(ctx context.Context, op, id, request string, started time.Time, err error) {
	level := slog.LevelInfo
	outcome := "succeeded"
	attrs := []any{"operation", op, "request_id", request}
	if id != "" {
		attrs = append(attrs, "library_id", id)
	}
	if err != nil {
		outcome = "failed"
		code := "service_unavailable"
		if errors.Is(err, ErrInvalid) {
			code = "validation_failed"
		} else if errors.Is(err, ErrConflict) {
			code = "conflict"
		} else if errors.Is(err, ErrNotFound) {
			code = "not_found"
		} else {
			level = slog.LevelError
		}
		attrs = append(attrs, "error_code", code)
	}
	attrs = append(attrs, "outcome", outcome, "duration_ms", time.Since(started).Milliseconds())
	s.logger.Log(ctx, level, "library operation completed", attrs...)
}

// List 读取游标之后的媒体库配置列表，失败时记录业务诊断。
func (s *Service) List(ctx context.Context, cursor string, limit int, request string) ([]Library, error) {
	started := time.Now()
	items, err := s.repo.ListLibraries(ctx, cursor, limit)
	if err != nil {
		s.log(ctx, "list", "", request, started, err)
	}
	return items, err
}

// Get 按 ID 读取媒体库配置，失败时记录稳定错误类别。
func (s *Service) Get(ctx context.Context, id, request string) (Library, error) {
	started := time.Now()
	item, err := s.repo.GetLibrary(ctx, id)
	if err != nil {
		s.log(ctx, "get", id, request, started, err)
	}
	return item, err
}

// Create 校验配置并以启用状态创建媒体库，生成身份及初始版本。
func (s *Service) Create(ctx context.Context, c Config, req string) (item Library, err error) {
	start := time.Now()
	defer func() { s.log(ctx, "create", item.ID, req, start, err) }()
	if err = Validate(c); err != nil {
		return
	}
	now := time.Now().UTC()
	c.Enabled = true
	item = Library{Config: c, ID: rand.Text(), CreatedAt: now, UpdatedAt: now, Revision: 1}
	err = s.repo.CreateLibrary(ctx, item)
	return
}

// Update 校验配置并核对调用方版本，保留启用状态后保存新配置；并发变化返回冲突。
func (s *Service) Update(ctx context.Context, id string, c Config, revision int64, req string) (item Library, err error) {
	start := time.Now()
	defer func() { s.log(ctx, "update", id, req, start, err) }()
	if err = Validate(c); err != nil {
		return
	}
	if item, err = s.repo.GetLibrary(ctx, id); err != nil {
		return
	}
	if item.Revision != revision {
		err = ErrConflict
		return
	}
	c.Enabled = item.Enabled
	item.Config = c
	item.UpdatedAt = time.Now().UTC()
	err = s.repo.UpdateLibrary(ctx, item)
	if err == nil {
		item.Revision++
	}
	return
}

// SetEnabled 修改启用状态并通过仓储版本约束保存，成功后返回递增版本。
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool, req string) (item Library, err error) {
	start := time.Now()
	defer func() { s.log(ctx, "set_enabled", id, req, start, err) }()
	if item, err = s.repo.GetLibrary(ctx, id); err != nil {
		return
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	err = s.repo.UpdateLibrary(ctx, item)
	if err == nil {
		item.Revision++
	}
	return
}

// Delete 删除配置并传递审计身份；媒体文件的保留规则由仓储删除合同约束。
func (s *Service) Delete(ctx context.Context, id, actor, req string) (err error) {
	start := time.Now()
	s.logger.InfoContext(ctx, "library deletion started", "operation", "delete", "library_id", id, "request_id", req)
	defer func() { s.log(ctx, "delete", id, req, start, err) }()
	return s.repo.DeleteLibrary(ctx, id, actor, req)
}
