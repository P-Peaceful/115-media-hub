//go:build production

// production.go：嵌入构建前复制到 dist 的生产前端资源，并向 HTTP 层提供以 dist 为根的只读文件系统。

package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var assets embed.FS

// Assets 返回真实前端构建的 dist 子文件系统，供 HTTP 静态资源和页面路由使用。
func Assets() (fs.FS, error) { return fs.Sub(assets, "dist") }
