// Package authorization 根据身份与组织状态计算能力，不依赖 HTTP 或数据存储。
package authorization

import (
	"errors"
	"slices"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

var (
	ErrDenied             = errors.New("permission denied")
	ErrRecentAuthRequired = errors.New("recent authentication required")
)

// Snapshot 包含计算授权结果所需的身份与组织状态。
type Snapshot struct {
	Principal          iamclient.Principal // 已验证的身份投影。
	OrganizationID     string              // 本次计算的组织范围。
	OrganizationActive bool                // 组织是否有效。
	MembershipActive   bool                // 当前用户的组织成员关系是否有效。
	Permissions        []string            // 有效权限键集合。
}

// Evaluate 只返回能力授权范围；业务服务仍需校验实际资源所属组织、状态和审批要求。
func Evaluate(s Snapshot, p Permission, orgID string, now time.Time, recentWindow time.Duration) (iamclient.AuthorizedScope, error) {
	denied := iamclient.AuthorizedScope{}

	if p.Status != "active" && p.Status != "deprecated" {
		return denied, ErrDenied
	}

	if p.RequiresRecentAuth {
		authenticated := s.Principal.AuthenticatedAt
		if s.Principal.ReauthenticatedAt != nil {
			authenticated = *s.Principal.ReauthenticatedAt
		}

		if authenticated.IsZero() || now.Sub(authenticated) > recentWindow || authenticated.After(now.Add(time.Minute)) {
			return denied, ErrRecentAuthRequired
		}
	}

	switch p.Scope {
	case "platform":
		if orgID != "" || !s.Principal.IsSuperAdmin {
			return denied, ErrDenied
		}
		return iamclient.AuthorizedScope{Kind: "platform"}, nil
	case "organization":
		if orgID == "" || orgID != s.OrganizationID || !s.OrganizationActive {
			return denied, ErrDenied
		}
		if !s.Principal.IsSuperAdmin && (!s.MembershipActive || !slices.Contains(s.Permissions, p.Key)) {
			return denied, ErrDenied
		}
		return iamclient.AuthorizedScope{Kind: "organization",
			OrgID: orgID}, nil
	default:
		return denied, ErrDenied
	}
}

// CheckDelegation 同时校验原有和拟设置的权限，防止低权限操作者修改高权限角色。
func CheckDelegation(effective []string, existing, candidate []string, catalog []Permission) error {
	availableExisting, availableNew := map[string]bool{}, map[string]bool{}
	for _, p := range catalog {
		if p.Scope != "organization" || !p.Delegable || !slices.Contains(effective, p.Key) {
			continue
		}

		if p.Status == "active" || p.Status == "deprecated" {
			availableExisting[p.Key] = true
		}

		if p.Status == "active" {
			availableNew[p.Key] = true
		}
	}

	for _, key := range existing {
		if !availableExisting[key] {
			return ErrDenied
		}
	}

	for _, key := range candidate {
		if !availableNew[key] && (!availableExisting[key] || !slices.Contains(existing, key)) {
			return ErrDenied
		}
	}

	return nil
}
