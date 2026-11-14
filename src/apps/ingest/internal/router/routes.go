// Package router 用原生 Gin 注册 Ingest 的路由与认证规则。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
)

// Channels 是通道处理器的依赖集合。
// 单独建模而不是塞进 Server：设备侧路由与管理侧路由的依赖不同，
// 混在一起会让管理侧也被迫持有通道实现。
type Channels struct {
	Registry connector.Registry  // 接入点与令牌查询。
	Limiter  httpchannel.Limiter // 每接入点频率限制。
	Pipeline *pipeline.Pipeline  // 受理后的解析与发布。
	MaxBody  int64               // 请求体上限，由配置注入通道。
}

// Register 注册模块路由。
// 设备推送路由直接暴露：设备带接入点令牌，不经过浏览器会话。
// 管理路由（/api/v1/ingest/connectors）在第五组随数据库接入。
func Register(engine gin.IRouter, server *handler.Server, channels Channels) {
	engine.GET("/health/ready", server.ReadyHandler)

	if channels.Registry != nil && channels.Limiter != nil && channels.Pipeline != nil {
		push := httpchannel.New(channels.Registry, channels.Limiter, channels.Pipeline)
		push.MaxBodyBytes = channels.MaxBody
		engine.POST("/api/v1/ingest/events/:connector_id", func(c *gin.Context) {
			push.Handle(c.Writer, c.Request, c.Param("connector_id"))
		})
	}
}

// Handler 在注册前装配公共中间件及输入边界。
func Handler(server *handler.Server, channels Channels) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server, channels)

	return engine
}
