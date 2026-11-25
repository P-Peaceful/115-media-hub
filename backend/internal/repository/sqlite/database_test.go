// database_test.go：使用真实临时 SQLite 验证连接参数、迁移重入、不可逆降级拒绝及启动取消。

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

// TestDatabaseConfigurationAndMigrations 验证 WAL、外键、忙等待和单连接池，替换连接仍保留配置并拒绝破坏性降级。
func TestDatabaseConfigurationAndMigrations(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "nested", "app.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, setting := range []struct {
		name string
		want string
	}{{"journal_mode", "wal"}, {"foreign_keys", "1"}, {"busy_timeout", "5000"}, {"application_id", "115"}} {
		var got string
		if err := database.sql.QueryRowContext(ctx, "PRAGMA "+setting.name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != setting.want {
			t.Errorf("%s = %s, want %s", setting.name, got, setting.want)
		}
	}
	// 强制替换底层连接，验证连接级外键配置仍然生效。
	database.sql.SetMaxIdleConns(0)
	var foreignKeys int
	if err := database.sql.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("replacement connection: %d, %v", foreignKeys, err)
	}
	if database.sql.Stats().MaxOpenConnections != 1 {
		t.Fatal("writes are not serialized")
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, database.sql, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 4); err == nil {
		t.Fatal("task history migration must reject destructive downgrade")
	}
	var applicationID int
	if err := database.sql.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil || applicationID != 115 {
		t.Fatalf("down migration: %d, %v", applicationID, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("repeated startup: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestDatabaseCancelledStartup 验证已取消上下文不能成功启动数据库，意外建立的资源也会关闭。
func TestDatabaseCancelledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "app.db"))
	if err == nil {
		if closeErr := database.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("cancelled startup succeeded")
	}
}
