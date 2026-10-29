package management

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListOrganizations 返回经过平台读取权限校验的组织列表。
func (s *Handler) ListOrganizations(c *gin.Context) {
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	limit, offset := params.Limit, params.Offset

	response, err := s.Service.ListOrganizations(r.Context(), limit, offset)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// CreateOrganization 绑定创建组织参数，交给用例创建及审计。
func (s *Handler) CreateOrganization(c *gin.Context) {
	w := c.Writer
	r := c.Request
	var input service.OrganizationInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	id, err := s.Service.CreateOrganization(r.Context(), input, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// UpdateOrganization 处理平台范围的组织状态变更。
func (s *Handler) UpdateOrganization(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input service.OrganizationUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateOrganization(r.Context(), id, input, true, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// GetOrganization 查询目标组织的公开投影。
func (s *Handler) GetOrganization(c *gin.Context) {
	orgId := c.Param("org_id")
	w := c.Writer
	r := c.Request
	data, err := s.Service.Organization(r.Context(), orgId)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, json.RawMessage(data))
}

// UpdateCurrentOrganization 处理组织管理员允许修改的组织字段。
func (s *Handler) UpdateCurrentOrganization(c *gin.Context) {
	orgId := c.Param("org_id")
	w := c.Writer
	r := c.Request
	var input model.APICurrentOrganizationUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateOrganization(r.Context(), orgId, service.OrganizationUpdate{Name: input.Name, AuthzVersion: input.AuthzVersion}, false, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// ListMembers 按当前组织和分页参数读取成员。
func (s *Handler) ListMembers(c *gin.Context) {
	orgId := c.Param("org_id")
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	limit, offset := params.Limit, params.Offset

	response, err := s.Service.ListMembers(r.Context(), limit, offset, orgId)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// CreateMember 限制高成本密码计算并创建组织成员。
func (s *Handler) CreateMember(c *gin.Context) {
	orgId := c.Param("org_id")
	w := c.Writer
	r := c.Request
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMMembersManage, orgId); err != nil {
		httpx.Reject(c, err)
		return
	}

	var input service.MemberInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	select {
	case s.HashSlots <- struct{}{}:
		defer func() { <-s.HashSlots }()
	default:
		httpx.Reject(c, authentication.ErrRateLimited)
		return
	}

	id, err := s.Service.CreateMember(r.Context(), orgId, input, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// UpdateMember 绑定版本与状态，交给用例校验更新。
func (s *Handler) UpdateMember(c *gin.Context) {
	orgId := c.Param("org_id")
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input service.MemberUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateMember(r.Context(), orgId, id, input, false, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// DeleteMember 依据当前组织版本移除成员。
func (s *Handler) DeleteMember(c *gin.Context) {
	orgId := c.Param("org_id")
	id := c.Param("id")
	var params model.AuthorizationVersionQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	version := params.AuthzVersion

	if err := s.Service.UpdateMember(r.Context(), orgId, id, service.MemberUpdate{AuthzVersion: version}, true, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}
