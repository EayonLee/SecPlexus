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

// ListUsers 按查询条件与分页读取平台用户。
func (s *Handler) ListUsers(c *gin.Context) {
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	query := params.Q

	limit, offset := params.Limit, params.Offset

	response, err := s.Service.ListUsers(r.Context(), limit, offset, query)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// GetUser 读取平台用户的公开字段。
func (s *Handler) GetUser(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	data, err := s.Service.User(r.Context(), id)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, json.RawMessage(data))
}

// CreateUser 限制密码计算并创建用户，凭据不会进入结果。
func (s *Handler) CreateUser(c *gin.Context) {
	w := c.Writer
	r := c.Request
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMPlatformUsersManage, ""); err != nil {
		httpx.Reject(c, err)
		return
	}

	var input service.UserInput
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

	id, err := s.Service.CreateUser(r.Context(), input, middleware.RequestID(r))
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Created(w, map[string]string{"id": id})
}

// UpdateUser 按身份版本更新用户资料和状态。
func (s *Handler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	var input service.UserUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Service.UpdateUser(r.Context(), id, input, middleware.RequestID(r)); err != nil {
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
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMPlatformUsersManage, ""); err != nil {
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
