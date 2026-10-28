// Package router 集中定义 IAM 的 HTTP 路由。
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
)

// Handlers 包含 IAM 路由需要的请求处理函数。
type Handlers struct {
	Ready                       http.HandlerFunc // 就绪检查。
	AuthContext                 http.HandlerFunc // 认证上下文。
	Login                       http.HandlerFunc // 登录。
	Reauthenticate              http.HandlerFunc // 二次认证。
	Me                          http.HandlerFunc // 当前用户。
	ChangePassword              http.HandlerFunc // 修改密码。
	Logout                      http.HandlerFunc // 退出当前会话。
	LogoutAll                   http.HandlerFunc // 退出所有会话。
	ListSessions                http.HandlerFunc // 查询会话。
	RevokeSession               http.HandlerFunc // 撤销会话。
	AuthorizeInternal           http.HandlerFunc // 内部鉴权。
	PreviewPermission           http.HandlerFunc // 预览权限变更。
	ListRoleMembers             http.HandlerFunc // 查询角色成员。
	ListUsers                   http.HandlerFunc // 查询用户列表。
	GetUser                     http.HandlerFunc // 查询用户详情。
	CreateUser                  http.HandlerFunc // 创建用户。
	UpdateUser                  http.HandlerFunc // 更新用户。
	SetSuperAdmin               http.HandlerFunc // 调整超级管理员。
	ResetUserPassword           http.HandlerFunc // 重置用户密码。
	ListOrganizations           http.HandlerFunc // 查询组织列表。
	CreateOrganization          http.HandlerFunc // 创建组织。
	UpdateOrganization          http.HandlerFunc // 平台更新组织。
	GetOrganization             http.HandlerFunc // 查询组织详情。
	UpdateCurrentOrganization   http.HandlerFunc // 更新当前组织。
	ListMembers                 http.HandlerFunc // 查询组织成员。
	CreateMember                http.HandlerFunc // 创建组织成员。
	UpdateMember                http.HandlerFunc // 更新组织成员。
	DeleteMember                http.HandlerFunc // 移除组织成员。
	ListRoles                   http.HandlerFunc // 查询角色。
	CreateRole                  http.HandlerFunc // 创建角色。
	GetRole                     http.HandlerFunc // 查询角色详情。
	UpdateRole                  http.HandlerFunc // 更新角色。
	SetRolePermissions          http.HandlerFunc // 设置角色权限。
	DeleteRole                  http.HandlerFunc // 删除角色。
	SetMemberRoles              http.HandlerFunc // 设置成员角色。
	Permissions                 http.HandlerFunc // 查询权限目录。
	ListOrganizationAuditEvents http.HandlerFunc // 查询组织审计记录。
	ListAuditEvents             http.HandlerFunc // 查询平台审计记录。
	SecurityOverview            http.HandlerFunc // 查询安全配置概览。
}

// RegisterPublic 按业务注册公开接口。
func RegisterPublic(mux chi.Router, h Handlers) {
	mux = mux.With(middleware.CaptureRoute)
	httpx.RouterErrors(mux)
	mux.MethodFunc("GET", "/health/ready", h.Ready)
	mux.Route("/api/v1/iam", func(r chi.Router) {
		registerSessionRoutes(r, h)
		registerUserRoutes(r, h)
		registerOrganizationRoutes(r, h)
		registerRoleRoutes(r, h)
		registerAuditRoutes(r, h)
	})
}

// registerSessionRoutes 注册认证与当前会话接口。
func registerSessionRoutes(mux chi.Router, h Handlers) {
	mux.MethodFunc("GET", "/auth/context", h.AuthContext)
	mux.MethodFunc("POST", "/auth/login", h.Login)
	mux.MethodFunc("POST", "/auth/reauthenticate", h.Reauthenticate)
	mux.MethodFunc("GET", "/me", h.Me)
	mux.MethodFunc("POST", "/me/password", h.ChangePassword)
	mux.MethodFunc("POST", "/auth/logout", h.Logout)
	mux.MethodFunc("POST", "/auth/logout-all", h.LogoutAll)
	mux.MethodFunc("GET", "/me/sessions", h.ListSessions)
	mux.MethodFunc("DELETE", "/me/sessions/{id}", h.RevokeSession)
}

// registerUserRoutes 注册平台用户接口。
func registerUserRoutes(mux chi.Router, h Handlers) {
	mux.MethodFunc("GET", "/users", h.ListUsers)
	mux.MethodFunc("GET", "/users/{id}", h.GetUser)
	mux.MethodFunc("POST", "/users", h.CreateUser)
	mux.MethodFunc("PATCH", "/users/{id}", h.UpdateUser)
	mux.MethodFunc("PUT", "/users/{id}/super-admin", h.SetSuperAdmin)
	mux.MethodFunc("POST", "/users/{id}/reset-password", h.ResetUserPassword)
}

// registerOrganizationRoutes 注册组织和成员接口。
func registerOrganizationRoutes(mux chi.Router, h Handlers) {
	mux.MethodFunc("GET", "/organizations", h.ListOrganizations)
	mux.MethodFunc("POST", "/organizations", h.CreateOrganization)
	mux.MethodFunc("PATCH", "/organizations/{id}", h.UpdateOrganization)
	mux.MethodFunc("GET", "/orgs/{org_id}", h.GetOrganization)
	mux.MethodFunc("PATCH", "/orgs/{org_id}", h.UpdateCurrentOrganization)
	mux.MethodFunc("GET", "/orgs/{org_id}/members", h.ListMembers)
	mux.MethodFunc("POST", "/orgs/{org_id}/members", h.CreateMember)
	mux.MethodFunc("PATCH", "/orgs/{org_id}/members/{id}", h.UpdateMember)
	mux.MethodFunc("DELETE", "/orgs/{org_id}/members/{id}", h.DeleteMember)
}

// registerRoleRoutes 注册角色与权限接口。
func registerRoleRoutes(mux chi.Router, h Handlers) {
	mux.MethodFunc("POST", "/orgs/{org_id}/permission-preview", h.PreviewPermission)
	mux.MethodFunc("GET", "/orgs/{org_id}/roles/{id}/members", h.ListRoleMembers)
	mux.MethodFunc("GET", "/orgs/{org_id}/roles", h.ListRoles)
	mux.MethodFunc("POST", "/orgs/{org_id}/roles", h.CreateRole)
	mux.MethodFunc("GET", "/orgs/{org_id}/roles/{id}", h.GetRole)
	mux.MethodFunc("PATCH", "/orgs/{org_id}/roles/{id}", h.UpdateRole)
	mux.MethodFunc("PUT", "/orgs/{org_id}/roles/{id}/permissions", h.SetRolePermissions)
	mux.MethodFunc("DELETE", "/orgs/{org_id}/roles/{id}", h.DeleteRole)
	mux.MethodFunc("PUT", "/orgs/{org_id}/members/{id}/roles", h.SetMemberRoles)
	mux.MethodFunc("GET", "/permissions", h.Permissions)
}

// registerAuditRoutes 注册审计与安全概览接口。
func registerAuditRoutes(mux chi.Router, h Handlers) {
	mux.MethodFunc("GET", "/orgs/{org_id}/audit-events", h.ListOrganizationAuditEvents)
	mux.MethodFunc("GET", "/audit-events", h.ListAuditEvents)
	mux.MethodFunc("GET", "/security/overview", h.SecurityOverview)
}

// RegisterInternal 注册内部接口。
func RegisterInternal(mux chi.Router, h Handlers) {
	mux = mux.With(middleware.CaptureRoute)
	httpx.RouterErrors(mux)

	mux.Route("/internal/v1/iam", func(r chi.Router) {
		r.MethodFunc("POST", "/authorize", h.AuthorizeInternal)
	})
}
