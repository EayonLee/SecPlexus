// Package router 挂载 Console 的原生 Gin 路由。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// Register 区分进程健康、依赖就绪和需要用户会话的授权上下文。
func Register(engine gin.IRouter, server *handler.Server, guard *iamclient.Client) {
	engine.GET("/health/live", server.Live)
	engine.GET("/health/ready", server.Ready)

	// 鉴权与审计由 iamclient 的中间件统一完成，handler 里不再做权限判断。
	if guard == nil {
		return
	}

	group := engine.Group("/api/v1/console", middleware.Guard(guard, iamclient.PermissionConsoleContextRead, "session"))
	group.GET("/context", server.Context)
}

// Handler 在注册前装配公共中间件及输入边界。
func Handler(server *handler.Server, guard *iamclient.Client) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server, guard)

	return engine
}
