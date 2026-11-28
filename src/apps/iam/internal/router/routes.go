// Package router 用原生 Gin 注册统一入口及各路由的认证规则。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// RegisterUserRoutes 注册用户业务路由及模块就绪检查。
// 授权只有「用户 → 角色 → 权限」，路由全部扁平，没有组织段。
func RegisterUserRoutes(engine gin.IRouter, server *handler.Server) {
	engine.GET("/health/ready", server.Ready)
	api := engine.Group("/api/v1/iam")
	registerAuth(api.Group("/auth"), server)
	registerMe(api.Group("/me"), server)
	registerUsers(api.Group("/users"), server)
	registerRoles(api.Group("/roles"), server)
	api.GET("/permissions", server.Permissions)
	api.GET("/audit-events", server.ListAuditEvents)
}

// Handler 在同一监听装配路由；用户请求与服务鉴权分别应用所需中间件。
func Handler(server *handler.Server) http.Handler {
	engine := middleware.NewEngine()
	engine.RemoteIPHeaders = []string{"X-Forwarded-For"}
	// 未配置可信代理时必须显式关闭转发头解析：Gin 在未调用 SetTrustedProxies 时信任所有来源，
	// 那样任何客户端都能用 X-Forwarded-For 伪造来源、绕过按 IP 的登录限流。
	if trustedProxies := server.Service.Config.App.TrustedProxyCIDRs; len(trustedProxies) == 0 {
		engine.ForwardedByClientIP = false
		if err := engine.SetTrustedProxies(nil); err != nil {
			panic("invalid trusted proxies")
		}
	} else if err := engine.SetTrustedProxies(trustedProxies); err != nil {
		panic("invalid trusted proxies")
	}
	services := engine.Group("/api/v1/iam", server.Errors)
	inputBoundary := middleware.InputBoundary(nil)
	services.POST("/authorize", inputBoundary, server.AuthorizeRequest)
	services.POST("/audit-events", inputBoundary, server.PostAuditEvent)
	services.GET("/permission-catalog", server.PermissionCatalog)
	user := engine.Group("")
	user.Use(iammiddleware.Session(server.Service.Sessions), server.Errors, server.Security.GinProtect(), func(c *gin.Context) {
		if c.FullPath() == "/api/v1/iam/auth/login" || c.FullPath() == "/api/v1/iam/auth/reauthenticate" {
			if err := server.Limiter.CheckIP(c.Request.Context(), c.ClientIP()); err != nil {
				logging.From(c.Request.Context()).Warn("Login source is rate limited", "event", "auth.login_rejected", "reason", "ip_rate_limited")
				httpx.Reject(c, err)
				return
			}
		}
		c.Next()
	}, middleware.InputBoundary(map[string]int64{"/api/v1/iam/auth/login": 2048, "/api/v1/iam/auth/reauthenticate": 2048}))
	RegisterUserRoutes(user, server)
	return engine
}
