// Package router 用原生 Gin 注册公开与隔离的内部入口。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
)

// RegisterPublic 注册公开业务路由及模块就绪检查。
func RegisterPublic(engine gin.IRouter, server *handler.Server) {
	engine.GET("/health/ready", server.Ready)
	engine.GET("/api/v1/iam/auth/context", server.AuthContext)
	engine.POST("/api/v1/iam/auth/login", server.Login)
	engine.POST("/api/v1/iam/auth/reauthenticate", server.Reauthenticate)
	engine.POST("/api/v1/iam/auth/logout", server.Logout)
	engine.POST("/api/v1/iam/auth/logout-all", server.LogoutAll)
	engine.GET("/api/v1/iam/me", server.Me)
	engine.POST("/api/v1/iam/me/password", server.ChangePassword)
	engine.GET("/api/v1/iam/me/sessions", server.ListSessions)
	engine.DELETE("/api/v1/iam/me/sessions/:id", server.RevokeSession)
	engine.GET("/api/v1/iam/users", server.ListUsers)
	engine.POST("/api/v1/iam/users", server.CreateUser)
	engine.PATCH("/api/v1/iam/users/:id", server.UpdateUser)
	engine.GET("/api/v1/iam/users/:id", server.GetUser)
	engine.GET("/api/v1/iam/organizations", server.ListOrganizations)
	engine.POST("/api/v1/iam/organizations", server.CreateOrganization)
	engine.PATCH("/api/v1/iam/organizations/:id", server.UpdateOrganization)
	engine.POST("/api/v1/iam/users/:id/reset-password", server.ResetUserPassword)
	engine.PUT("/api/v1/iam/users/:id/super-admin", server.SetSuperAdmin)
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
	engine.GET("/api/v1/iam/security/overview", server.SecurityOverview)
}

// PublicHandler 装配会话、错误、CSRF、限流和原生输入边界。
func PublicHandler(server *handler.Server) http.Handler {
	engine := middleware.NewEngine()
	engine.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := engine.SetTrustedProxies(server.Service.Config.App.TrustedProxyCIDRs); err != nil {
		panic("invalid trusted proxies")
	}
	public := engine.Group("")
	public.Use(iammiddleware.Session(server.Service.Sessions), server.Errors, server.Security.GinProtect(), func(c *gin.Context) {
		if c.FullPath() == "/api/v1/iam/auth/login" || c.FullPath() == "/api/v1/iam/auth/reauthenticate" {
			if err := server.Limiter.CheckIP(c.Request.Context(), c.ClientIP()); err != nil {
				httpx.Reject(c, err)
				return
			}
		}
		c.Next()
	}, middleware.InputBoundary(map[string]int64{"/api/v1/iam/auth/login": 2048, "/api/v1/iam/auth/reauthenticate": 2048}))
	RegisterPublic(public, server)
	return engine
}

// RegisterInternal 仅注册服务间授权入口。
func RegisterInternal(engine gin.IRouter, server *handler.Server) {
	engine.POST("/internal/v1/iam/authorize", server.AuthorizeInternal)
}

// InternalHandler 不挂载任何公开业务路由。
func InternalHandler(server *handler.Server) http.Handler {
	engine := middleware.NewEngine()
	engine.Use(server.Errors, middleware.InputBoundary(nil))
	RegisterInternal(engine, server)
	return engine
}
