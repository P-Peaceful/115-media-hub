// Package sqlite owns the SQLite connection and migration boundary.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wenzhengcheng/115-media-hub/backend/migrations"
)

// Database owns the single-connection pool. Closing it releases SQLite resources.
type Database struct {
	sql *sql.DB
	orm *gorm.DB
}

// Open configures every connection and migrates before the database becomes ready.
func Open(ctx context.Context, path string) (_ *Database, resultErr error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: absolute}
	query := url.Values{"_journal_mode": {"WAL"}, "_foreign_keys": {"on"}, "_busy_timeout": {"5000"}}
	uri.RawQuery = query.Encode()
	orm, err := gorm.Open(sqlite.Open(uri.String()), &gorm.Config{
		Logger:               logger.Default.LogMode(logger.Silent),
		DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	pool, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("get database pool: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, pool.Close())
		}
	}()
	// Serialize writes; per-connection DSN settings also apply after replacement.
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if err := pool.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, migrations.Files)
	if err != nil {
		return nil, fmt.Errorf("prepare migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Database{sql: pool, orm: orm}, nil
}

// Ping checks that SQLite can still accept connections.
func (d *Database) Ping(ctx context.Context) error { return d.sql.PingContext(ctx) }

// Close releases the connection pool after HTTP shutdown.
func (d *Database) Close() error { return d.sql.Close() }
