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
	engine.GET("/api/v1/iam/auth/context", server.AuthContext)
	engine.POST("/api/v1/iam/auth/login", server.Login)
	engine.POST("/api/v1/iam/auth/reauthenticate", server.Reauthenticate)
	engine.POST("/api/v1/iam/auth/logout", server.Logout)
	engine.GET("/api/v1/iam/me", server.Me)
	engine.PATCH("/api/v1/iam/me", server.UpdateProfile)
	engine.POST("/api/v1/iam/me/password", server.ChangePassword)
	engine.GET("/api/v1/iam/users", server.ListUsers)
	engine.POST("/api/v1/iam/users", server.CreateUser)
	engine.GET("/api/v1/iam/users/:id", server.GetUser)
	engine.PATCH("/api/v1/iam/users/:id", server.UpdateUser)
	engine.DELETE("/api/v1/iam/users/:id", server.DeleteUser)
	engine.PUT("/api/v1/iam/users/:id/roles", server.SetUserRoles)
	engine.POST("/api/v1/iam/users/:id/reset-password", server.ResetUserPassword)
	engine.GET("/api/v1/iam/roles", server.ListRoles)
	engine.POST("/api/v1/iam/roles", server.CreateRole)
	engine.GET("/api/v1/iam/roles/:id", server.GetRole)
	engine.PATCH("/api/v1/iam/roles/:id", server.UpdateRole)
	engine.DELETE("/api/v1/iam/roles/:id", server.DeleteRole)
	engine.PUT("/api/v1/iam/roles/:id/permissions", server.SetRolePermissions)
	engine.GET("/api/v1/iam/permissions", server.Permissions)
	engine.GET("/api/v1/iam/audit-events", server.ListAuditEvents)
}

// Handler 在同一监听装配路由；用户请求与服务鉴权分别应用所需中间件。
func Handler(server *handler.Server) http.Handler {
	engine := middleware.NewEngine()
	engine.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := engine.SetTrustedProxies(server.Service.Config.App.TrustedProxyCIDRs); err != nil {
		panic("invalid trusted proxies")
	}
	engine.POST("/api/v1/iam/authorize", server.Errors, middleware.InputBoundary(nil), server.AuthorizeRequest)
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
