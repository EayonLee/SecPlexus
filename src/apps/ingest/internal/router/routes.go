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

// devicePrefix 是设备接入路径前缀。
// 单独列出来是因为它必须与管理面共享同一个监听端口、但用不同的输入边界。
const devicePrefix = "/api/v1/ingest/events/"

// Register 注册管理面路由。
// 设备推送不在这里：它有自己的体积与媒体类型规则，不能继承管理面的边界。
func Register(engine gin.IRouter, server *handler.Server, channels Channels) {
	engine.GET("/health/ready", server.ReadyHandler)
}

// registerPush 注册设备推送路由。
func registerPush(engine gin.IRouter, channels Channels) {
	if channels.Registry == nil || channels.Limiter == nil || channels.Pipeline == nil {
		return
	}

	push := httpchannel.New(channels.Registry, channels.Limiter, channels.Pipeline)
	push.MaxBodyBytes = channels.MaxBody

	engine.POST(devicePrefix+":connector_id", func(c *gin.Context) {
		push.Handle(c.Writer, c.Request, c.Param("connector_id"))
	})
}

// Handler 返回管理面处理器（含管理面输入边界）。
func Handler(server *handler.Server, channels Channels) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server, channels)

	return engine
}

// DeviceHandler 返回设备接入处理器。
//
// 它刻意不挂 middleware.InputBoundary：那是给浏览器与管理 API 用的边界
// （32 KiB、仅 application/json），套在设备推送上会把 10 MiB 配置压成 32 KiB、
// 并拒绝设备常用的 text/plain。设备通道自己的体积与类型约束在 httpchannel 内。
func DeviceHandler(channels Channels) http.Handler {
	engine := middleware.NewEngine()
	registerPush(engine, channels)

	return engine
}

// RootHandler 装配完整入口：设备推送走设备边界，其余走管理边界。
// 两条边界共用同一个监听端口，由路径前缀分流。
func RootHandler(server *handler.Server, channels Channels) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(devicePrefix, DeviceHandler(channels))
	mux.Handle("/", Handler(server, channels))

	return mux
}
