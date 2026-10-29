package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Me 返回当前身份和组织能力，具体查询由业务用例负责。
func (s *Server) Me(c *gin.Context) {
	var params model.OrganizationQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	w := c.Writer
	r := c.Request
	orgID := params.OrgID
	response, err := s.Service.Identity(r.Context(), orgID)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
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
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
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

// Logout 结束当前浏览器会话。
func (s *Server) Logout(c *gin.Context) {
	s.endSessions(c, false)
}

// LogoutAll 结束当前用户的全部会话。
func (s *Server) LogoutAll(c *gin.Context) {
	s.endSessions(c, true)
}

// endSessions 先撤销数据库会话，再删除浏览器会话状态。
func (s *Server) endSessions(c *gin.Context, all bool) {
	w := c.Writer
	r := c.Request
	p, err := s.Service.Sessions.Resolve(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	if err = s.Service.Sessions.Revoke(r.Context(), p, p.SessionID, all, "logout", requestID(r)); err != nil {
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

// ListSessions 读取当前用户的有效会话列表。
func (s *Server) ListSessions(c *gin.Context) {
	w := c.Writer
	r := c.Request
	response, err := s.Service.SessionsList(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// RevokeSession 仅允许近期认证的当前用户撤销自己的会话。
func (s *Server) RevokeSession(c *gin.Context) {
	id := c.Param("id")
	w := c.Writer
	r := c.Request
	p, err := s.Service.Sessions.Resolve(r.Context())
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	authenticated := p.AuthenticatedAt
	if p.ReauthenticatedAt != nil {
		authenticated = *p.ReauthenticatedAt
	}

	if time.Since(authenticated) > s.Service.Config.Auth.ReauthWindow {
		httpx.Reject(c, authorization.ErrRecentAuthRequired)
		return
	}

	if err = s.Service.Sessions.Revoke(r.Context(), p, id, false, "user_revoked", requestID(r)); err != nil {
		httpx.Reject(c, err)
		return
	}

	if id == p.SessionID {
		if err = s.Service.Sessions.Manager.Destroy(r.Context()); err != nil {
			httpx.Reject(c, err)
			return
		}
	}

	result.NoContent(w)
}
