// database.go：建立 SQLite 单连接池，配置连接参数并在对外可用前完成数据库迁移。

// Package sqlite 管理 SQLite 连接、迁移与持久化边界。
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

// Database 拥有单连接数据库池及 ORM 入口，关闭时释放 SQLite 资源。
type Database struct {
	sql *sql.DB
	orm *gorm.DB
}

// Open 创建数据库目录并配置每个连接的 WAL、外键和忙等待；迁移失败时关闭已建立的池。
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
	// 使用单连接串行写入；DSN 中的连接参数在底层连接替换后仍生效。
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

// Ping 使用调用方上下文检查数据库连接是否可用。
func (d *Database) Ping(ctx context.Context) error { return d.sql.PingContext(ctx) }

// Close 释放数据库连接池，应在服务停止使用数据库后调用。
func (d *Database) Close() error { return d.sql.Close() }
