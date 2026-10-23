package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open initializes a local SQLite database and applies embedded SQL migrations.
func Open(path string) (*gorm.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	// SQLite inherits the existing file's permissions; create it privately first.
	f, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: absolute}
	q := uri.Query()
	q.Set("_journal_mode", "WAL")
	q.Set("_foreign_keys", "on")
	q.Set("_busy_timeout", "5000")
	uri.RawQuery = q.Encode()
	db, err := gorm.Open(sqlite.Open(uri.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.Files)
	if err == nil {
		_, err = provider.Up(context.Background())
	}
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate SQLite: %w", err)
	}
	return db, nil
}
