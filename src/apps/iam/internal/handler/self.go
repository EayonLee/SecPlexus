package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// stringAccount 提取限流用的规范化账号，不记录原始凭据。
func stringAccount(body []byte) string {
	var input struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal(body, &input)
	return strings.ToLower(strings.TrimSpace(input.Username))
}

// encodeLogin 为已有账号的密码确认构造认证库输入。
func encodeLogin(username, password string) ([]byte, error) {
	return json.Marshal(map[string]string{"username": username, "password": password})
}

// me 返回当前身份和组织能力，具体查询由业务用例负责。
func (s *Server) me(w http.ResponseWriter, r *http.Request) error {
	response, err := s.Service.Identity(r.Context(), r.URL.Query().Get("org_id"))
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// changePassword 绑定新旧密码，限流与用例校验后撤销旧会话。
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	var input struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if err := s.Limiter.Check(r.Context(), s.clientIP(r), "password:"+s.Service.Sessions.Manager.GetString(r.Context(), authentication.LogicalSessionKey)); err != nil {
		return err
	}

	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return authentication.ErrRateLimited
	}

	if err := s.Service.ChangePassword(r.Context(), input.OldPassword, input.NewPassword, requestID(r)); err != nil {
		return err
	}

	s.Security.ClearCSRF(w)
	result.NoContent(w)
	return nil
}

// logout 结束当前浏览器会话。
func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	return s.endSessions(w, r, false)
}

// logoutAll 结束当前用户的全部会话。
func (s *Server) logoutAll(w http.ResponseWriter, r *http.Request) error {
	return s.endSessions(w, r, true)
}

// endSessions 先撤销数据库会话，再删除浏览器会话状态。
func (s *Server) endSessions(w http.ResponseWriter, r *http.Request, all bool) error {
	p, err := s.Service.Sessions.Resolve(r.Context())
	if err != nil {
		return err
	}

	if err = s.Service.Sessions.Revoke(r.Context(), p, p.SessionID, all, "logout", requestID(r)); err != nil {
		return err
	}

	if err = s.Service.Sessions.Manager.Destroy(r.Context()); err != nil {
		return err
	}

	s.Security.ClearCSRF(w)
	result.NoContent(w)
	return nil
}

// listSessions 读取当前用户的有效会话列表。
func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) error {
	response, err := s.Service.SessionsList(r.Context())
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// revokeSession 仅允许近期认证的当前用户撤销自己的会话。
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) error {
	p, err := s.Service.Sessions.Resolve(r.Context())
	if err != nil {
		return err
	}

	authenticated := p.AuthenticatedAt
	if p.ReauthenticatedAt != nil {
		authenticated = *p.ReauthenticatedAt
	}

	if time.Since(authenticated) > s.Service.Config.Auth.ReauthWindow {
		return authorization.ErrRecentAuthRequired
	}

	id := r.PathValue("id")
	if err = s.Service.Sessions.Revoke(r.Context(), p, id, false, "user_revoked", requestID(r)); err != nil {
		return err
	}

	if id == p.SessionID {
		if err = s.Service.Sessions.Manager.Destroy(r.Context()); err != nil {
			return err
		}
	}

	result.NoContent(w)
	return nil
}
