package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListUsers 按查询条件与分页读取用户。
func (s *Handler) ListUsers(c *gin.Context) {
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request

	page, err := s.Service.ListUsers(r.Context(), params.Limit, params.Offset, params.Q)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	items := make([]model.APIUser, 0, len(page.Items))
	for _, user := range page.Items {
		items = append(items, apiUser(user))
	}

	result.Success(w, model.APIPage[model.APIUser]{
		Items:  items,
		Total:  page.Total,
		Limit:  page.Limit,
		Offset: page.Offset,
	})
}

// GetUser 读取用户的公开字段与角色。
func (s *Handler) GetUser(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	user, err := s.Service.User(r.Context(), id)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, apiUser(user))
}

// CreateUser 限制密码计算并创建用户，凭据不会进入结果。
func (s *Handler) CreateUser(c *gin.Context) {
	w := c.Writer
	r := c.Request
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMUsersManage); err != nil {
		httpx.Reject(c, err)
		return
	}

	var input model.APIUserInput
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

	id, err := s.Service.CreateUser(r.Context(), service.UserCreate{
		Username:    input.Username,
		DisplayName: input.DisplayName,
		Email:       input.Email,
		Password:    input.Password,
		RoleIDs:     input.RoleIDs,
	}, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// UpdateUser 按账号版本更新用户资料和状态。
func (s *Handler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input model.APIUserUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateUser(r.Context(), id, service.UserUpdate{
		DisplayName: input.DisplayName,
		Email:       input.Email,
		Status:      enum.UserStatus(input.Status),
	}, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// DeleteUser 删除用户；配置托管账号与仍被角色绑定引用的账号由用例拒绝。
func (s *Handler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	if err := s.Service.DeleteUser(r.Context(), id, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// SetUserRoles 全量替换用户角色，权限和委派上限由用例校验。
func (s *Handler) SetUserRoles(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input model.APIUserRolesInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.ReplaceUserRoles(r.Context(), id, service.UserRoles{RoleIDs: input.RoleIDs}, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// ResetUserPassword 限制密码计算并通过用例重置密码。
func (s *Handler) ResetUserPassword(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMUsersReset); err != nil {
		httpx.Reject(c, err)
		return
	}

	var input model.APIResetPasswordInput
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

	if err := s.Service.ResetPassword(r.Context(), id, input.Password, middleware.RequestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	result.NoContent(w)
}

// apiUser 把账号投影映射为 HTTP DTO；角色引用直接来自数据库。
func apiUser(user repository.UserSummary) model.APIUser {
	roles := make([]model.APIRoleRef, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, model.APIRoleRef{ID: role.ID, Name: role.Name})
	}

	return model.APIUser{
		ID:          user.ID.String(),
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Status:      string(user.Status),
		Configured:  user.Configured,
		AuthEpoch:   user.AuthEpoch,
		Roles:       roles,
	}
}
