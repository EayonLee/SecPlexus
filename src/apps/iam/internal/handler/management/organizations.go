package management

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// listOrganizations 返回经过平台读取权限校验的组织列表。
func (s *Handler) listOrganizations(w http.ResponseWriter, r *http.Request) error {
	limit, offset, err := pagination(r)
	if err != nil {
		return err
	}

	response, err := s.Service.ListOrganizations(r.Context(), limit, offset)
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// createOrganization 绑定创建组织参数，交给用例创建及审计。
func (s *Handler) createOrganization(w http.ResponseWriter, r *http.Request) error {
	var input service.OrganizationInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	id, err := s.Service.CreateOrganization(r.Context(), input, middleware.RequestID(r))
	if err != nil {
		return err
	}

	result.Created(w, map[string]string{"id": id})
	return nil
}

// updateOrganization 处理平台范围的组织状态变更。
func (s *Handler) updateOrganization(w http.ResponseWriter, r *http.Request) error {
	var input service.OrganizationUpdate
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.UpdateOrganization(r.Context(), r.PathValue("id"), input, true, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// getOrganization 查询目标组织的公开投影。
func (s *Handler) getOrganization(w http.ResponseWriter, r *http.Request) error {
	data, err := s.Service.Organization(r.Context(), r.PathValue("org_id"))
	if err != nil {
		return err
	}

	result.Success(w, json.RawMessage(data))
	return nil
}

// updateCurrentOrganization 处理组织管理员允许修改的组织字段。
func (s *Handler) updateCurrentOrganization(w http.ResponseWriter, r *http.Request) error {
	var input service.OrganizationUpdate
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.UpdateOrganization(r.Context(), r.PathValue("org_id"), input, false, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// listMembers 按当前组织和分页参数读取成员。
func (s *Handler) listMembers(w http.ResponseWriter, r *http.Request) error {
	limit, offset, err := pagination(r)
	if err != nil {
		return err
	}

	response, err := s.Service.ListMembers(r.Context(), limit, offset, r.PathValue("org_id"))
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// createMember 限制高成本密码计算并创建组织成员。
func (s *Handler) createMember(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.Service.Authorize(r.Context(), contract.PermissionIAMMembersManage, r.PathValue("org_id")); err != nil {
		return err
	}

	var input service.MemberInput
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	select {
	case s.HashSlots <- struct{}{}:
		defer func() { <-s.HashSlots }()
	default:
		return authentication.ErrRateLimited
	}

	id, err := s.Service.CreateMember(r.Context(), r.PathValue("org_id"), input, middleware.RequestID(r))
	if err != nil {
		return err
	}

	result.Created(w, map[string]string{"id": id})
	return nil
}

// updateMember 绑定版本与状态，交给用例校验更新。
func (s *Handler) updateMember(w http.ResponseWriter, r *http.Request) error {
	var input service.MemberUpdate
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Service.UpdateMember(r.Context(), r.PathValue("org_id"), r.PathValue("id"), input, false, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}

// deleteMember 依据当前组织版本移除成员。
func (s *Handler) deleteMember(w http.ResponseWriter, r *http.Request) error {
	version, err := strconv.ParseInt(r.URL.Query().Get("authz_version"), 10, 64)
	if err != nil {
		return repository.ErrInvalid
	}

	if err = s.Service.UpdateMember(r.Context(), r.PathValue("org_id"), r.PathValue("id"), service.MemberUpdate{AuthzVersion: version}, true, middleware.RequestID(r)); err != nil {
		return err
	}

	result.NoContent(w)
	return nil
}
