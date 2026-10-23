package database

import (
	"path/filepath"
	"testing"
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
		if version != 1 {
			t.Fatalf("migration version = %d", version)
		}
		if sqlDB.Stats().MaxOpenConnections != 1 {
			t.Fatal("expected single write connection")
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
