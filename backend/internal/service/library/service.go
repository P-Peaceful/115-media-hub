// Package library manages persistent media library configuration, without touching media files.
package library

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Mapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type Config struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	DirectoryID       string    `json:"directory_id"`
	DirectoryPath     string    `json:"directory_path"`
	MediaType         string    `json:"media_type"`
	Enabled           bool      `json:"enabled"`
	OutputPath        string    `json:"output_path"`
	GenerateSTRM      bool      `json:"generate_strm"`
	ScrapeMetadata    bool      `json:"scrape_metadata"`
	GenerateNFO       bool      `json:"generate_nfo"`
	PreserveStructure bool      `json:"preserve_structure"`
	StructureMode     string    `json:"structure_mode"`
	PathMappings      []Mapping `json:"path_mappings"`
	LinkType          string    `json:"link_type"`
	LinkExpiry        string    `json:"link_expiry"`
	FilenameRule      string    `json:"filename_rule"`
	STRMExtension     string    `json:"strm_extension"`
	NFOName           string    `json:"nfo_name"`
	MinFileSizeMB     int       `json:"min_file_size_mb"`
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
func Validate(c Config) error {
	if strings.TrimSpace(c.Name) == "" || !validText(c.Name, 100) || !validText(c.Description, 200) || !directoryID.MatchString(c.DirectoryID) || strings.TrimSpace(c.DirectoryPath) == "" || !validText(c.DirectoryPath, 1024) || !filepath.IsAbs(c.OutputPath) || !validText(c.OutputPath, 1024) || !oneOf(c.MediaType, "movie", "tv", "anime", "documentary", "mixed") || !oneOf(c.StructureMode, "full", "flatten") || !oneOf(c.LinkType, "direct", "proxy") || !oneOf(c.LinkExpiry, "default", "never") || c.FilenameRule != "original" || c.STRMExtension != ".strm" || c.NFOName != "same_as_video" || c.MinFileSizeMB < 0 || c.MinFileSizeMB > 1048576 || c.PathMappings == nil || len(c.PathMappings) > 100 {
		return ErrInvalid
	}
	for _, m := range c.PathMappings {
		if strings.TrimSpace(m.Source) == "" || strings.TrimSpace(m.Target) == "" || !validText(m.Source, 1024) || !validText(m.Target, 1024) {
			return ErrInvalid
		}
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
