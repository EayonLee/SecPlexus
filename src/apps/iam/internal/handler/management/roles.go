package management

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListRoles 读取全局角色列表。
func (s *Handler) ListRoles(c *gin.Context) {
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request

	response, err := s.Service.Roles(r.Context(), params.Limit, params.Offset)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// CreateRole 创建全局角色。
func (s *Handler) CreateRole(c *gin.Context) {
	w := c.Writer
	r := c.Request
	var input model.APIRoleCreate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	id, err := s.Service.CreateRole(r.Context(), service.RoleCreate{
		Name:        input.Name,
		Description: input.Description,
		Permissions: input.Permissions,
	}, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// GetRole 读取指定角色。
func (s *Handler) GetRole(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	data, err := s.Service.Role(r.Context(), id)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, json.RawMessage(data))
}

// UpdateRole 更新角色资料。
func (s *Handler) UpdateRole(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input model.APIRoleInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateRole(r.Context(), id, service.RoleUpdate{
		Name:        input.Name,
		Description: input.Description,
	}, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// SetRolePermissions 全量替换角色权限并保留委派限制。
func (s *Handler) SetRolePermissions(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input model.APIGrantsInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.ReplaceGrants(r.Context(), id, service.RoleGrants{Permissions: input.Permissions}, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// DeleteRole 删除未被用户引用的角色。
func (s *Handler) DeleteRole(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request

	if err := s.Service.DeleteRole(r.Context(), id, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}
