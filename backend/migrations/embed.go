// embed.go：将版本化 SQL 迁移嵌入应用，供数据库初始化与升级统一读取。

// Package migrations 管理应用版本化数据库迁移资源。
package migrations

import "embed"

// Files 包含随应用发布的 SQL 迁移，供迁移执行器按版本读取。
//
//go:embed *.sql
var Files embed.FS
