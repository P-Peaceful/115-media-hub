// Package library manages persistent media library configuration, without touching media files.
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
type Library struct {
	Config
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Revision  int64     `json:"revision"`
}

var ErrNotFound = errors.New("library not found")
var ErrConflict = errors.New("library changed concurrently")
var ErrInvalid = errors.New("invalid library configuration")

type Repository interface {
	ListLibraries(context.Context, string, int) ([]Library, error)
	GetLibrary(context.Context, string) (Library, error)
	CreateLibrary(context.Context, Library) error
	UpdateLibrary(context.Context, Library) error
	DeleteLibrary(context.Context, string, string, string) error
}
type Service struct {
	repo   Repository
	logger *slog.Logger
}

func New(repo Repository, logger *slog.Logger) *Service {
	return &Service{repo, logger.With("module", "library")}
}

var directoryID = regexp.MustCompile(`^[0-9]{1,32}$`)

func validText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, '\x00')
}
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func validDirectDomain(domain string) bool {
	if domain == "" {
		return true
	}
	if !validText(domain, 253) || strings.TrimSpace(domain) != domain {
		return false
	}
	parsed, err := url.Parse("//" + domain)
	if err != nil || parsed.Host != domain || parsed.User != nil || parsed.Path != "" {
		return false
	}
	if strings.HasSuffix(domain, ":") {
		return false
	}
	if port := parsed.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return false
		}
	}
	host := parsed.Hostname()
	if net.ParseIP(host) != nil {
		return true
	}
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

func Validate(c Config) error {
	if strings.TrimSpace(c.Name) == "" || !validText(c.Name, 100) ||
		!directoryID.MatchString(c.DirectoryID) || strings.TrimSpace(c.DirectoryPath) == "" ||
		!validText(c.DirectoryPath, 1024) || !filepath.IsAbs(c.OutputPath) ||
		!validText(c.OutputPath, 1024) || !oneOf(c.MediaType, "movie", "tv", "anime", "documentary", "mixed") ||
		!oneOf(c.LanguagePreference, "zh-CN") || !oneOf(c.ScrapeSource, "tmdb", "metatube", "none") ||
		!validText(c.ExcludeDirectoryPath, 1024) || !oneOf(c.FileEncoding, "UTF-8") ||
		!oneOf(c.LinkType, "direct", "proxy") || !validDirectDomain(c.STRMDirectDomain) ||
		!oneOf(c.STRMLinkFormat, "original_name", "file_id") ||
		!oneOf(c.STRMExistingFilePolicy, "skip", "overwrite") {
		return ErrInvalid
	}
	return nil
}
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
func (s *Service) List(ctx context.Context, cursor string, limit int, request string) ([]Library, error) {
	started := time.Now()
	items, err := s.repo.ListLibraries(ctx, cursor, limit)
	if err != nil {
		s.log(ctx, "list", "", request, started, err)
	}
	return items, err
}
func (s *Service) Get(ctx context.Context, id, request string) (Library, error) {
	started := time.Now()
	item, err := s.repo.GetLibrary(ctx, id)
	if err != nil {
		s.log(ctx, "get", id, request, started, err)
	}
	return item, err
}
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
func (s *Service) Delete(ctx context.Context, id, actor, req string) (err error) {
	start := time.Now()
	s.logger.InfoContext(ctx, "library deletion started", "operation", "delete", "library_id", id, "request_id", req)
	defer func() { s.log(ctx, "delete", id, req, start, err) }()
	return s.repo.DeleteLibrary(ctx, id, actor, req)
}
