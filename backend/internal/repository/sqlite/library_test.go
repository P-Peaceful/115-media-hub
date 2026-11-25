// library_test.go：验证媒体库配置持久化、删除审计回滚、输入约束和修订号并发保护，确保不修改媒体文件。

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

// validLibraryConfig 构造完整合法媒体库配置，固定现有文件策略以供持久化与验证测试复用。
func validLibraryConfig(output string) library.Config {
	return library.Config{Name: "电影", DirectoryID: "12345678901234567890", DirectoryPath: "/我的资源/电影", MediaType: "movie", Enabled: true, OutputPath: output, GenerateSTRM: true, STRMExistingFilePolicy: "skip", GenerateNFO: true, GeneratePosterCover: true, LanguagePreference: "zh-CN", ScrapeSource: "tmdb", IncludeSubdirectories: true, FollowDirectoryChanges: true, FileEncoding: "UTF-8", LinkType: "proxy", STRMLinkFormat: "original_name"}
}

// TestLibraryLifecycleAndDeleteAudit 验证配置读写、停用和删除，审计失败回滚且本地媒体文件与敏感路径不受影响。
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

// TestLibraryValidation 验证已有文件策略、播放源地址、目录 ID 和服务器输出路径边界。
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
	for _, address := range []string{
		"media.example.com:65536", "127.0.0.1:99999", "[::1]:99999", "media.example.com:",
		"https://example.com/path", "https://example.com/#", "https://example.com/?", "https://example.com/%2F",
		"https://private@example.com", "ftp://example.com", "http://[example.com]", "http://2001:db8::1:80",
	} {
		c := validLibraryConfig("/media")
		c.STRMDirectDomain = address
		if library.Validate(c) == nil {
			t.Errorf("invalid playback address accepted: %q", address)
		}
	}
	for address, want := range map[string]string{
		"":                             "",
		"media.example.com:443":        "https://media.example.com:443",
		"legacy.example.com:0":         "https://legacy.example.com:0",
		"http://192.168.1.42:8080/":    "http://192.168.1.42:8080",
		"HTTPS://media.example.com/":   "https://media.example.com",
		"https://[2001:db8::42]:8443/": "https://[2001:db8::42]:8443",
	} {
		c := validLibraryConfig("/media")
		c.STRMDirectDomain = address
		if got, valid := library.PlaybackOrigin(address); !valid || got != want || library.Validate(c) != nil {
			t.Errorf("playback address %q resolved to %q, valid=%v, want %q", address, got, valid, want)
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
	c.ExcludeDirectoryPath = "115 网盘/待排除"
	if library.Validate(c) != nil {
		t.Fatal("selected drive display path rejected")
	}
}

// TestLibraryConcurrentChangesDoNotOverwrite 验证旧修订号更新返回冲突，不覆盖已保存的新配置。
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
