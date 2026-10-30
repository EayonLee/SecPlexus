// Package router 用原生 Gin 注册统一入口及各路由的认证规则。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
)

// RegisterUserRoutes 注册用户业务路由及模块就绪检查。
func RegisterUserRoutes(engine gin.IRouter, server *handler.Server) {
	engine.GET("/health/ready", server.Ready)
	engine.GET("/api/v1/iam/auth/context", server.AuthContext)
	engine.POST("/api/v1/iam/auth/login", server.Login)
	engine.POST("/api/v1/iam/auth/reauthenticate", server.Reauthenticate)
	engine.POST("/api/v1/iam/auth/logout", server.Logout)
	engine.GET("/api/v1/iam/me", server.Me)
	engine.POST("/api/v1/iam/me/password", server.ChangePassword)
	engine.GET("/api/v1/iam/users", server.ListUsers)
	engine.POST("/api/v1/iam/users", server.CreateUser)
	engine.PATCH("/api/v1/iam/users/:id", server.UpdateUser)
	engine.GET("/api/v1/iam/users/:id", server.GetUser)
	engine.GET("/api/v1/iam/organizations", server.ListOrganizations)
	engine.POST("/api/v1/iam/organizations", server.CreateOrganization)
	engine.PATCH("/api/v1/iam/organizations/:id", server.UpdateOrganization)
	engine.POST("/api/v1/iam/users/:id/reset-password", server.ResetUserPassword)
	engine.GET("/api/v1/iam/orgs/:org_id", server.GetOrganization)
	engine.PATCH("/api/v1/iam/orgs/:org_id", server.UpdateCurrentOrganization)
	engine.GET("/api/v1/iam/orgs/:org_id/members", server.ListMembers)
	engine.POST("/api/v1/iam/orgs/:org_id/members", server.CreateMember)
	engine.PATCH("/api/v1/iam/orgs/:org_id/members/:id", server.UpdateMember)
	engine.DELETE("/api/v1/iam/orgs/:org_id/members/:id", server.DeleteMember)
	engine.GET("/api/v1/iam/orgs/:org_id/roles", server.ListRoles)
	engine.POST("/api/v1/iam/orgs/:org_id/roles", server.CreateRole)
	engine.PATCH("/api/v1/iam/orgs/:org_id/roles/:id", server.UpdateRole)
	engine.DELETE("/api/v1/iam/orgs/:org_id/roles/:id", server.DeleteRole)
	engine.GET("/api/v1/iam/orgs/:org_id/roles/:id", server.GetRole)
	engine.GET("/api/v1/iam/orgs/:org_id/roles/:id/members", server.ListRoleMembers)
	engine.PUT("/api/v1/iam/orgs/:org_id/roles/:id/permissions", server.SetRolePermissions)
	engine.PUT("/api/v1/iam/orgs/:org_id/members/:id/roles", server.SetMemberRoles)
	engine.POST("/api/v1/iam/orgs/:org_id/permission-preview", server.PreviewPermission)
	engine.GET("/api/v1/iam/permissions", server.Permissions)
	engine.GET("/api/v1/iam/orgs/:org_id/audit-events", server.ListOrganizationAuditEvents)
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
				httpx.Reject(c, err)
				return
			}
		}
		c.Next()
	}, middleware.InputBoundary(map[string]int64{"/api/v1/iam/auth/login": 2048, "/api/v1/iam/auth/reauthenticate": 2048}))
	RegisterUserRoutes(user, server)
	return engine
}
