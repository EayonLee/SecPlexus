// Package management 处理用户、组织、角色和审计管理接口。
package management

import (
	"net/http"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
)

// Handler 显式接收管理用例和有限的密码计算槽；不读取全局配置或连接数据库。
type Handler struct {
	Service   *service.Service                     // 管理用例及权限边界。
	HashSlots chan struct{}                        // 限制同时执行的高成本密码计算。
	Handle    func(httpx.Handler) http.HandlerFunc // 与认证入口共用的错误及路径参数适配。
}

// Routes 返回管理接口处理函数，具体 URL 仍由 router 注册。
func (s *Handler) Routes() router.Handlers {
	return router.Handlers{
		PreviewPermission:           s.Handle(s.previewPermission),
		ListRoleMembers:             s.Handle(s.listRoleMembers),
		ListUsers:                   s.Handle(s.listUsers),
		GetUser:                     s.Handle(s.getUser),
		CreateUser:                  s.Handle(s.createUser),
		UpdateUser:                  s.Handle(s.updateUser),
		SetSuperAdmin:               s.Handle(s.setSuperAdmin),
		ResetUserPassword:           s.Handle(s.resetUserPassword),
		ListOrganizations:           s.Handle(s.listOrganizations),
		CreateOrganization:          s.Handle(s.createOrganization),
		UpdateOrganization:          s.Handle(s.updateOrganization),
		GetOrganization:             s.Handle(s.getOrganization),
		UpdateCurrentOrganization:   s.Handle(s.updateCurrentOrganization),
		ListMembers:                 s.Handle(s.listMembers),
		CreateMember:                s.Handle(s.createMember),
		UpdateMember:                s.Handle(s.updateMember),
		DeleteMember:                s.Handle(s.deleteMember),
		ListRoles:                   s.Handle(s.listRoles),
		CreateRole:                  s.Handle(s.createRole),
		GetRole:                     s.Handle(s.getRole),
		UpdateRole:                  s.Handle(s.updateRole),
		SetRolePermissions:          s.Handle(s.setRolePermissions),
		DeleteRole:                  s.Handle(s.deleteRole),
		SetMemberRoles:              s.Handle(s.setMemberRoles),
		Permissions:                 s.Handle(s.permissions),
		ListOrganizationAuditEvents: s.Handle(s.listOrganizationAuditEvents),
		ListAuditEvents:             s.Handle(s.listAuditEvents),
		SecurityOverview:            s.Handle(s.securityOverview),
	}
}
