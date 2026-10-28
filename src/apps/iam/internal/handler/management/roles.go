package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"

	"strconv"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// PreviewPermission 预览拟议权限变更，不实际授予权限。
func (s *Handler) PreviewPermission(c *gin.Context, orgId string, params router.PreviewPermissionParams) {
	w := c.Writer
	r := c.Request
	var input service.PreviewInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	response, err := s.Service.Preview(r.Context(), orgId, input)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// ListRoleMembers 查询角色关联成员，组织与角色读取权限均由用例校验。
func (s *Handler) ListRoleMembers(c *gin.Context, orgId string, id string, params router.ListRoleMembersParams) {
	w := c.Writer
	r := c.Request
	limit, offset := pagination(params.Limit, params.Offset)

	response, err := s.Service.ListRoleMembers(r.Context(), limit, offset, orgId, id)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// ListRoles 读取角色及当前操作者允许执行的动作。
func (s *Handler) ListRoles(c *gin.Context, orgId string, params router.ListRolesParams) {
	w := c.Writer
	r := c.Request
	limit, offset := pagination(params.Limit, params.Offset)

	items, total, err := s.Service.Roles(r.Context(), orgId, "", limit, offset)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

// CreateRole 创建组织自定义角色。
func (s *Handler) CreateRole(c *gin.Context, orgId string, params router.CreateRoleParams) {
	w := c.Writer
	r := c.Request
	var input service.RoleInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	id, err := s.Service.CreateRole(r.Context(), orgId, input, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// GetRole 读取当前组织内的指定角色。
func (s *Handler) GetRole(c *gin.Context, orgId string, id string) {
	w := c.Writer
	r := c.Request
	items, _, err := s.Service.Roles(r.Context(), orgId, id, 1, 0)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	if len(items) == 0 {
		httpx.Reject(c, repository.ErrNotFound)
		return
	}

	result.Success(w, items[0])
}

// UpdateRole 按版本更新角色资料。
func (s *Handler) UpdateRole(c *gin.Context, orgId string, id string, params router.UpdateRoleParams) {
	w := c.Writer
	r := c.Request
	var input service.RoleInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateRole(r.Context(), orgId, id, input, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// SetRolePermissions 按版本替换角色权限并保留委派限制。
func (s *Handler) SetRolePermissions(c *gin.Context, orgId string, id string, params router.SetRolePermissionsParams) {
	w := c.Writer
	r := c.Request
	var input service.GrantsInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.ReplaceGrants(r.Context(), orgId, id, input, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// DeleteRole 按版本删除未被成员引用的自定义角色。
func (s *Handler) DeleteRole(c *gin.Context, orgId string, id string, params router.DeleteRoleParams) {
	w := c.Writer
	r := c.Request
	version, err := strconv.ParseInt(params.Version, 10, 64)
	if err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err = s.Service.DeleteRole(r.Context(), orgId, id, version, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// SetMemberRoles 替换成员角色关系，权限和版本检查由用例完成。
func (s *Handler) SetMemberRoles(c *gin.Context, orgId string, id string, params router.SetMemberRolesParams) {
	w := c.Writer
	r := c.Request
	var input service.MemberRolesInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.ReplaceMemberRoles(r.Context(), orgId, id, input, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}
