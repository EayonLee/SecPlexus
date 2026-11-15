// Package router 用原生 Gin 注册 Ingest 的路由与认证规则。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/source"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
)

// Deps 是通道处理器的依赖集合。
// 单独建模而不是塞进 Server：设备侧路由与管理侧路由的依赖不同，
// 混在一起会让管理侧也被迫持有通道实现。
type Deps struct {
	Registry source.Lookup       // 接入点与令牌查询。
	Limiter  httpchannel.Limiter // 每接入点频率限制。
	Pipeline *pipeline.Pipeline  // 受理后的解析与发布。
	MaxBody  int64               // 请求体上限，由配置注入通道。
	Admin    handler.SourceStore // 接入点与规则存取；nil 表示管理面未启用。
	Guard    gin.HandlerFunc     // 管理面会话守卫；nil 表示不注册管理路由。
}

// devicePrefix 是设备接入路径前缀。
// 单独列出来是因为它必须与管理面共享同一个监听端口、但用不同的输入边界。
const devicePrefix = "/api/v1/ingest/events/"

// AdminGuard 校验发起管理请求的用户会话。
//
// 本项目不新增权限点（全员可见决策）：守卫只需要确认"是一个有效会话"，
// 不做细粒度授权。为 nil 时管理路由不注册——绝不暴露无鉴权的写接口。
type AdminGuard gin.HandlerFunc

// registerAdmin 注册接入点与解析规则的管理路由。
func registerAdmin(engine gin.IRouter, deps Deps) {
	if deps.Admin == nil || deps.Guard == nil {
		return
	}

	admin := &handler.Admin{Store: deps.Admin}
	group := engine.Group("/api/v1/ingest", deps.Guard)

	group.GET("/sources", admin.ListSources)
	group.POST("/sources", admin.CreateSource)
	group.PATCH("/sources/:id", admin.UpdateSource)
	group.DELETE("/sources/:id", admin.DeleteSource)

	group.GET("/parse-rules", admin.ListRules)
	group.POST("/parse-rules", admin.CreateRule)
	group.PATCH("/parse-rules/:id", admin.UpdateRule)
	group.DELETE("/parse-rules/:id", admin.DeleteRule)
}

// Register 注册管理面路由。
// 设备推送不在这里：它有自己的体积与媒体类型规则，不能继承管理面的边界。
func Register(engine gin.IRouter, server *handler.Server, deps Deps) {
	engine.GET("/health/ready", server.ReadyHandler)
}

// registerPush 注册设备推送路由。
func registerPush(engine gin.IRouter, deps Deps) {
	if deps.Registry == nil || deps.Limiter == nil || deps.Pipeline == nil {
		return
	}

	push := httpchannel.New(deps.Registry, deps.Limiter, deps.Pipeline)
	push.MaxBodyBytes = deps.MaxBody

	engine.POST(devicePrefix+":source_id", func(c *gin.Context) {
		push.Handle(c.Writer, c.Request, c.Param("source_id"))
	})
}

// Handler 返回管理面处理器（含管理面输入边界）。
func Handler(server *handler.Server, deps Deps) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(middleware.InputBoundary(nil))
	Register(engine, server, deps)
	registerAdmin(engine, deps)

	return engine
}

// DeviceHandler 返回设备接入处理器。
//
// 它刻意不挂 middleware.InputBoundary：那是给浏览器与管理 API 用的边界
// （32 KiB、仅 application/json），套在设备推送上会把 10 MiB 配置压成 32 KiB、
// 并拒绝设备常用的 text/plain。设备通道自己的体积与类型约束在 httpchannel 内。
func DeviceHandler(deps Deps) http.Handler {
	engine := middleware.NewEngine()
	registerPush(engine, deps)

	return engine
}

// RootHandler 装配完整入口：设备推送走设备边界，其余走管理边界。
// 两条边界共用同一个监听端口，由路径前缀分流。
func RootHandler(server *handler.Server, deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(devicePrefix, DeviceHandler(deps))
	mux.Handle("/", Handler(server, deps))

	return mux
}
