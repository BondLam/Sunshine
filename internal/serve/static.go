package serve

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

// staticFiles 是嵌进二进制的前端静态文件（与 templates/templates.go
// 同模式的 //go:embed；Alpine.js 已 vendored，不走 CDN）。
//
//go:embed all:static
var staticFiles embed.FS

// handleIndex 返回首页 index.html。
func (s *Server) handleIndex(c *gin.Context) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "读取首页失败：%v", err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// handleStatic 返回 /static/* 下的嵌进二进制的静态文件。
func (s *Server) handleStatic(c *gin.Context) {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "读取静态文件失败：%v", err)
		return
	}
	http.StripPrefix("/static/", http.FileServer(http.FS(sub))).ServeHTTP(c.Writer, c.Request)
}
