package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

func TestOpenEnablesSQLiteSafetyAndMigrations(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "app.db")
	for range 2 {
		db, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		var journal string
		if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
			t.Fatal(err)
		}
		if journal != "wal" {
			t.Fatalf("journal mode: %s", journal)
		}
		for pragma, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000} {
			var got int
			if err := sqlDB.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("%s = %d, want %d", pragma, got, want)
			}
		}
		var version int
		if err := sqlDB.QueryRow("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != 3 {
			t.Fatalf("migration version = %d, want 3", version)
		}
		var profileColumn int
		if err := sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('connections') WHERE name = 'profile_json'").Scan(&profileColumn); err != nil {
			t.Fatal(err)
		}
		if profileColumn != 1 {
			t.Fatal("profile_json column was not migrated")
		}
		if sqlDB.Stats().MaxOpenConnections != 1 {
			t.Fatal("expected single write connection")
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConnectionMigrationsRollbackAndReapply(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(context.Background(), 1); err != nil {
		t.Fatalf("rollback connection migrations: %v", err)
	}
	var tableCount int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'connections'").Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Fatal("connections table still exists after rolling back its migrations")
	}
	if _, err := provider.UpTo(context.Background(), 3); err != nil {
		t.Fatalf("reapply connection migrations: %v", err)
	}
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'connections'").Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 1 {
		t.Fatal("connections table was not restored after reapplying migrations")
	}
}
