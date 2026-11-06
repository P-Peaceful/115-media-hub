package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

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
	// Force a replacement connection and verify connection-scoped foreign keys survive.
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
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	var applicationID int
	if err := database.sql.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil || applicationID != 0 {
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
