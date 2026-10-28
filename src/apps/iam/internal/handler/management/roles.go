package management

import (
	"net/http"
	"strconv"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// previewPermission 预览拟议权限变更，不实际授予权限。
func (s *Handler) previewPermission(w http.ResponseWriter, r *http.Request) error {
	var input service.PreviewInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	response, err := s.Service.Preview(r.Context(), r.PathValue("org_id"), input)
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// listRoleMembers 查询角色关联成员，组织与角色读取权限均由用例校验。
func (s *Handler) listRoleMembers(w http.ResponseWriter, r *http.Request) error {
	limit, offset, err := pagination(r)
	if err != nil {
		return err
	}

	response, err := s.Service.ListRoleMembers(r.Context(), limit, offset, r.PathValue("org_id"), r.PathValue("id"))
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// listRoles 读取角色及当前操作者允许执行的动作。
func (s *Handler) listRoles(w http.ResponseWriter, r *http.Request) error {
	limit, offset, err := pagination(r)
	if err != nil {
		return err
	}

	items, total, err := s.Service.Roles(r.Context(), r.PathValue("org_id"), "", limit, offset)
	if err != nil {
		return err
	}

	result.Success(w, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
	return nil
}

// createRole 创建组织自定义角色。
func (s *Handler) createRole(w http.ResponseWriter, r *http.Request) error {
	var input service.RoleInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	id, err := s.Service.CreateRole(r.Context(), r.PathValue("org_id"), input, middleware.RequestID(r))
	if err != nil {
		return err
	}

	result.Created(w, map[string]string{"id": id})
	return nil
}

// getRole 读取当前组织内的指定角色。
func (s *Handler) getRole(w http.ResponseWriter, r *http.Request) error {
	items, _, err := s.Service.Roles(r.Context(), r.PathValue("org_id"), r.PathValue("id"), 1, 0)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		return repository.ErrNotFound
	}

	result.Success(w, items[0])
	return nil
}

// updateRole 按版本更新角色资料。
func (s *Handler) updateRole(w http.ResponseWriter, r *http.Request) error {
	var input service.RoleInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.UpdateRole(r.Context(), r.PathValue("org_id"), r.PathValue("id"), input, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// setRolePermissions 按版本替换角色权限并保留委派限制。
func (s *Handler) setRolePermissions(w http.ResponseWriter, r *http.Request) error {
	var input service.GrantsInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.ReplaceGrants(r.Context(), r.PathValue("org_id"), r.PathValue("id"), input, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// deleteRole 按版本删除未被成员引用的自定义角色。
func (s *Handler) deleteRole(w http.ResponseWriter, r *http.Request) error {
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil {
		return repository.ErrInvalid
	}

	if err = s.Service.DeleteRole(r.Context(), r.PathValue("org_id"), r.PathValue("id"), version, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// setMemberRoles 替换成员角色关系，权限和版本检查由用例完成。
func (s *Handler) setMemberRoles(w http.ResponseWriter, r *http.Request) error {
	var input service.MemberRolesInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.ReplaceMemberRoles(r.Context(), r.PathValue("org_id"), r.PathValue("id"), input, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}
