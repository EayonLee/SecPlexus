// Package router 挂载 Console 的原生 Gin 路由。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Register 保留 SSE 的原生响应和取消语义。
func Register(engine gin.IRouter, server *handler.Server) {
	engine.GET("/health/ready", func(c *gin.Context) { result.Success(c.Writer, map[string]bool{"ready": true}) })
	engine.GET("/api/v1/console/orgs/:org_id/context", server.Context)
	engine.GET("/api/v1/console/orgs/:org_id/events", server.Events)
}

// Handler 在注册前装配公共中间件及输入边界。
func Handler(server *handler.Server) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server)
	return engine
}
