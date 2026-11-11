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
	return library.Config{Name: "电影", Description: "配置", DirectoryID: "12345678901234567890", DirectoryPath: "/我的资源/电影", MediaType: "movie", Enabled: true, OutputPath: output, GenerateSTRM: true, GenerateNFO: true, PreserveStructure: true, StructureMode: "full", PathMappings: []library.Mapping{{Source: "/source", Target: "/target"}}, LinkType: "direct", LinkExpiry: "default", FilenameRule: "original", STRMExtension: ".strm", NFOName: "same_as_video", MinFileSizeMB: 100}
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
	if err != nil || got.DirectoryID != cfg.DirectoryID || len(got.PathMappings) != 1 || got.PathMappings[0] != cfg.PathMappings[0] {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	got.Description = "updated"
	if _, err = service.Update(ctx, got.ID, got.Config, got.Revision, "update-request"); err != nil {
		t.Fatal(err)
	}
	got, err = service.SetEnabled(ctx, got.ID, false, "pause-request")
	if err != nil || got.Enabled || got.Description != "updated" {
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
	c.PathMappings = nil
	if library.Validate(c) == nil {
		t.Fatal("null mappings accepted")
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
