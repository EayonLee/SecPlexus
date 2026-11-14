// Package router 用原生 Gin 注册 Ingest 的路由与认证规则。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Register 注册模块路由。骨架阶段只有就绪检查；
// 数据接入的管理路由（/api/v1/ingest/connectors）与设备推送路由在后续批次挂载。
func Register(engine gin.IRouter, server *handler.Server) {
	engine.GET("/health/ready", func(c *gin.Context) { result.Success(c.Writer, map[string]bool{"ready": true}) })
}

// Handler 在注册前装配公共中间件及输入边界。
func Handler(server *handler.Server) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server)
	return engine
}
