package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Me 返回当前账号、角色与有效权限，具体查询由业务用例负责。
func (s *Server) Me(c *gin.Context) {
	w := c.Writer
	r := c.Request
	me, err := s.Service.Me(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	roles := make([]model.APIRoleRef, 0, len(me.Roles))
	for _, role := range me.Roles {
		roles = append(roles, model.APIRoleRef{ID: role.ID, Name: role.Name})
	}

	result.Success(w, model.APIMe{
		Principal:   me.Principal,
		Permissions: me.Permissions,
		Roles:       roles,
		TimeZone:    me.TimeZone,
	})
}

// ChangePassword 绑定新旧密码，限流与用例校验后撤销旧会话。
func (s *Server) ChangePassword(c *gin.Context) {
	w := c.Writer
	r := c.Request
	var input model.APIChangePasswordInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if err := s.Limiter.Check(r.Context(), c.ClientIP(), "password:"+s.Service.Sessions.Manager.GetString(r.Context(), authentication.LogicalSessionKey)); err != nil {
		httpx.Reject(c, err)
		return
	}

	select {
	case s.HashSlots <- struct{}{}:
		defer func() { <-s.HashSlots }()
	default:
		httpx.Reject(c, authentication.ErrRateLimited)
		return
	}

	if err := s.Service.ChangePassword(r.Context(), input.OldPassword, input.NewPassword, requestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	s.Security.ClearCSRF(w)
	result.NoContent(w)
}

// Logout 先撤销当前会话，再删除浏览器状态。
func (s *Server) Logout(c *gin.Context) {
	w := c.Writer
	r := c.Request
	p, err := s.Service.Sessions.Resolve(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	if err = s.Service.Sessions.Revoke(r.Context(), p, p.SessionID, "logout", requestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	if err = s.Service.Sessions.Manager.Destroy(r.Context()); err != nil {
		httpx.Reject(c, err)
		return
	}

	s.Security.ClearCSRF(w)
	result.NoContent(w)
}

// UpdateProfile 更新本人展示时区偏好，未提交该字段时保持不变。
func (s *Server) UpdateProfile(c *gin.Context) {
	w := c.Writer
	r := c.Request
	var input model.APIProfileUpdate
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	if input.TimeZone != nil {
		if err := s.Service.UpdateTimeZone(r.Context(), *input.TimeZone, requestID(r)); err != nil {
			httpx.Reject(c, err)
			return
		}

		result.Success(w, gin.H{"timezone": *input.TimeZone})
		return
	}

	me, err := s.Service.Me(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, model.APIProfileUpdateResponse{TimeZone: me.TimeZone})
}
