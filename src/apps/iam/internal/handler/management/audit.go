package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListOrganizationAuditEvents 读取当前组织的审计列表。
func (s *Handler) ListOrganizationAuditEvents(c *gin.Context) {
	orgId := c.Param("org_id")
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	limit, offset := params.Limit, params.Offset
	s.auditEvents(c, orgId, false, limit, offset)
}

// ListAuditEvents 读取平台管理员可见的审计范围。
func (s *Handler) ListAuditEvents(c *gin.Context) {
	var params model.ListQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	orgID := params.OrgID
	limit, offset := params.Limit, params.Offset
	s.auditEvents(c, orgID, true, limit, offset)
}

// auditEvents 绑定分页与范围参数，查询和授权由业务用例处理。
func (s *Handler) auditEvents(c *gin.Context, orgID string, platform bool, limit, offset int) {
	w := c.Writer
	r := c.Request

	response, err := s.Service.ListAuditEvents(r.Context(), orgID, platform, limit, offset)
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	result.Success(w, response)
}

// SecurityOverview 返回可公开的认证策略摘要，不包含密钥。
func (s *Handler) SecurityOverview(c *gin.Context) {
	w := c.Writer
	r := c.Request
	if _, err := s.Service.Authorize(r.Context(), iamclient.PermissionIAMPlatformSecurityRead, ""); err != nil {
		httpx.Reject(c, err)
		return
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
}
