package sqlite

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/library"
)

func validLibraryConfig(output string) library.Config {
	return library.Config{Name: "电影", DirectoryID: "12345678901234567890", DirectoryPath: "/我的资源/电影", MediaType: "movie", Enabled: true, OutputPath: output, GenerateSTRM: true, STRMExistingFilePolicy: "skip", GenerateNFO: true, GeneratePosterCover: true, LanguagePreference: "zh-CN", ScrapeSource: "tmdb", IncludeSubdirectories: true, FollowDirectoryChanges: true, FileEncoding: "UTF-8", LinkType: "proxy", STRMLinkFormat: "original_name"}
}
func TestLibraryLifecycleAndDeleteAudit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	var logs bytes.Buffer
	service := library.New(db, slog.New(slog.NewJSONHandler(&logs, nil)))
	file := filepath.Join(dir, "keep.strm")
	if err = os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := validLibraryConfig(dir)
	created, err := service.Create(ctx, cfg, "create-request")
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Get(ctx, created.ID, "get")
	if err != nil || got.DirectoryID != cfg.DirectoryID || got.ScrapeSource != cfg.ScrapeSource || got.STRMExistingFilePolicy != "skip" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	got.STRMExistingFilePolicy = "overwrite"
	got.GenerateSTRM = false
	if _, err = service.Update(ctx, got.ID, got.Config, got.Revision, "update-request"); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, created.ID, "get-updated")
	if err != nil || got.STRMExistingFilePolicy != "overwrite" || got.GenerateSTRM {
		t.Fatalf("policy update round trip: %+v %v", got, err)
	}
	content, readErr := os.ReadFile(file)
	if readErr != nil || string(content) != "keep" {
		t.Fatalf("configuration update changed STRM: %q %v", content, readErr)
	}
	got, err = service.SetEnabled(ctx, got.ID, false, "pause-request")
	if err != nil || got.Enabled {
		t.Fatalf("pause: %+v %v", got, err)
	}
	items, err := service.List(ctx, "", 10, "list")
	if err != nil || len(items) != 1 {
		t.Fatalf("list: %+v %v", items, err)
	}
	if err = db.orm.Exec("CREATE TRIGGER reject_library_audit BEFORE INSERT ON audit_records BEGIN SELECT RAISE(FAIL, 'audit unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(ctx, got.ID, "actor", "delete-fail"); err == nil {
		t.Fatal("expected audit failure")
	}
	if _, err = service.Get(ctx, got.ID, "get"); err != nil {
		t.Fatalf("delete was not rolled back: %v", err)
	}
	if err = db.orm.Exec("DROP TRIGGER reject_library_audit").Error; err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(ctx, got.ID, "actor", "delete-request"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatalf("media file changed: %v", err)
	}
	var audit auditModel
	if err = db.orm.Where("action = ?", "delete_library").Take(&audit).Error; err != nil || audit.TargetID != got.ID || audit.CorrelationID != "delete-request" {
		t.Fatalf("audit: %+v %v", audit, err)
	}
	for _, value := range []string{`"module":"library"`, `"operation":"delete"`, `"outcome":"failed"`, `"error_code":"service_unavailable"`, `"request_id":"delete-request"`, `"library_id":"` + got.ID + `"`, `"duration_ms":`} {
		if !strings.Contains(logs.String(), value) {
			t.Errorf("missing log %s", value)
		}
	}
	if strings.Contains(logs.String(), dir) {
		t.Error("logs contain path")
	}
}
func TestLibraryValidation(t *testing.T) {
	for _, policy := range []string{"skip", "overwrite", "", "rename"} {
		c := validLibraryConfig("/media")
		c.GenerateSTRM = false
		c.STRMExistingFilePolicy = policy
		valid := policy == "skip" || policy == "overwrite"
		if (library.Validate(c) == nil) != valid {
			t.Errorf("existing-file policy %q: expected valid=%v", policy, valid)
		}
	}
	for _, domain := range []string{"media.example.com:65536", "127.0.0.1:99999", "[::1]:99999", "media.example.com:"} {
		c := validLibraryConfig("/media")
		c.STRMDirectDomain = domain
		if library.Validate(c) == nil {
			t.Errorf("invalid direct domain accepted: %q", domain)
		}
	}
	c := validLibraryConfig("/media")
	c.DirectoryID = "1e12"
	if library.Validate(c) == nil {
		t.Fatal("invalid ID accepted")
	}
	c = validLibraryConfig("relative")
	if library.Validate(c) == nil {
		t.Fatal("relative path accepted")
	}
	c = validLibraryConfig("/media")
	c.STRMDirectDomain = "https://example.com/path"
	if library.Validate(c) == nil {
		t.Fatal("invalid direct domain accepted")
	}
	c = validLibraryConfig("/media")
	c.STRMDirectDomain = "media.example.com:443"
	if library.Validate(c) != nil {
		t.Fatal("valid direct domain rejected")
	}
	c = validLibraryConfig("/media")
	c.ExcludeDirectoryPath = "115 网盘/待排除"
	if library.Validate(c) != nil {
		t.Fatal("selected drive display path rejected")
	}
}

func TestLibraryConcurrentChangesDoNotOverwrite(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "concurrency.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	service := library.New(db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	created, err := service.Create(ctx, validLibraryConfig("/media"), "create")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := db.GetLibrary(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := db.GetLibrary(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Enabled = false
	if err = db.UpdateLibrary(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	stale.Name = "changed"
	if err = db.UpdateLibrary(ctx, stale); !errors.Is(err, library.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	got, err := db.GetLibrary(ctx, created.ID)
	if err != nil || got.Enabled || got.Name != "电影" {
		t.Fatalf("overwritten: %+v %v", got, err)
	}
}
