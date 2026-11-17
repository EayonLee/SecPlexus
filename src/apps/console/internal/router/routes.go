// Package router 挂载 Console 的原生 Gin 路由。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Register 保留 SSE 的原生响应和取消语义；上下文与事件流都是扁平的，没有组织段。
func Register(engine gin.IRouter, server *handler.Server, guard *iamclient.Client) {
	engine.GET("/health/ready", func(c *gin.Context) { result.Success(c.Writer, map[string]bool{"ready": true}) })

	// 鉴权与审计由 iamclient 的中间件统一完成，handler 里不再做权限判断。
	if guard == nil {
		return
	}

	group := engine.Group("/api/v1/console", middleware.Guard(guard, iamclient.PermissionConsoleEventsRead, "session"))
	group.GET("/context", server.Context)
	group.GET("/events", server.Events)
}

// Handler 在注册前装配公共中间件及输入边界。
func Handler(server *handler.Server, guard *iamclient.Client) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server, guard)

	return engine
}
