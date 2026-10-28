package management

import (
	"net/http"

	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// listOrganizationAuditEvents 读取当前组织的审计列表。
func (s *Handler) listOrganizationAuditEvents(w http.ResponseWriter, r *http.Request) error {
	return s.auditEvents(w, r, r.PathValue("org_id"), false)
}

// listAuditEvents 读取平台管理员可见的审计范围。
func (s *Handler) listAuditEvents(w http.ResponseWriter, r *http.Request) error {
	return s.auditEvents(w, r, r.URL.Query().Get("org_id"), true)
}

// auditEvents 绑定分页与范围参数，查询和授权由业务用例处理。
func (s *Handler) auditEvents(w http.ResponseWriter, r *http.Request, orgID string, platform bool) error {
	limit, offset, err := pagination(r)
	if err != nil {
		return err
	}

	response, err := s.Service.ListAuditEvents(r.Context(), orgID, platform, limit, offset)
	if err != nil {
		return err
	}

	result.Success(w, response)
	return nil
}

// securityOverview 返回可公开的认证策略摘要，不包含密钥。
func (s *Handler) securityOverview(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.Service.Authorize(r.Context(), contract.PermissionIAMPlatformSecurityRead, ""); err != nil {
		return err
	}

	cfg := s.Service.Config
	result.Success(w, map[string]any{
		"session_idle_timeout":     cfg.Auth.SessionIdleTimeout.String(),
		"session_absolute_timeout": cfg.Auth.SessionAbsoluteTimeout.String(),
		"reauth_window":            cfg.Auth.ReauthWindow.String(),
		"login_max_failures":       cfg.Auth.LoginMaxFailures,
		"login_lock_duration":      cfg.Auth.LoginLockDuration.String(),
		"self_registration":        false,
	})
	return nil
}
