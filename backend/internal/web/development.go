//go:build !production

// development.go：在开发构建中禁用前端嵌入，让独立运行的 Vite 提供前端页面与资源。

// Package web 为开发及生产构建选择前端资源来源。
package web

import "io/fs"

// Assets 在开发构建中返回 nil 文件系统，前端资源由 Vite 单独提供。
func Assets() (fs.FS, error) { return nil, nil }
