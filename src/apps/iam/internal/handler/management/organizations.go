package management

import (
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

// OrganizationTree 返回当前身份可见的组织树。
func (s *Handler) OrganizationTree(c *gin.Context) {
	w := c.Writer
	r := c.Request

	items, truncated, err := s.Service.OrganizationTree(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, map[string]any{"items": items, "truncated": truncated})
}

// DeleteOrganization 删除空组织；有子组织或用户时拒绝。
func (s *Handler) DeleteOrganization(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request

	if err := s.Service.DeleteOrganization(r.Context(), id, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
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

	if err := s.Service.UpdateOrganization(r.Context(), id, input, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// ListOrganizationUsers 按当前组织和分页参数读取用户。
func (s *Handler) ListOrganizationUsers(c *gin.Context) {
	orgId := c.Param("org_id")
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	limit, offset := params.Limit, params.Offset

	response, err := s.Service.ListOrganizationUsers(r.Context(), limit, offset, orgId, params.Scope == "subtree")
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// CreateOrganizationUser 限制高成本密码计算并创建组织用户。
func (s *Handler) CreateOrganizationUser(c *gin.Context) {
	orgId := c.Param("org_id")
	w := c.Writer
	r := c.Request
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMUsersManage, orgId); err != nil {
		httpx.Reject(c, err)
		return
	}

	var input service.OrganizationUserInput
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

	id, err := s.Service.CreateOrganizationUser(r.Context(), orgId, input, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// UpdateOrganizationUser 绑定版本与状态，交给用例校验更新。
func (s *Handler) UpdateOrganizationUser(c *gin.Context) {
	orgId := c.Param("org_id")
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input service.OrganizationUserUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateOrganizationUser(r.Context(), orgId, id, input, false, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// DeleteOrganizationUser 依据当前组织版本移除用户。
func (s *Handler) DeleteOrganizationUser(c *gin.Context) {
	orgId := c.Param("org_id")
	id := c.Param("id")
	var params model.AuthorizationVersionQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	version := params.AuthzEpoch

	if err := s.Service.UpdateOrganizationUser(r.Context(), orgId, id, service.OrganizationUserUpdate{AuthzEpoch: version}, true, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}
