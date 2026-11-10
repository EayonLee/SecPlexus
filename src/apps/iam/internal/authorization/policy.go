// Package authorization 根据账号与权限键计算授权，不依赖 HTTP 或数据存储。
package authorization

import (
	"errors"
	"slices"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

var (
	ErrDenied             = errors.New("permission denied")
	ErrRecentAuthRequired = errors.New("recent authentication required")
)

// Snapshot 包含计算授权结果所需的账号与有效权限。
// 扁平授权没有组织、成员关系或祖先链，判定只看账号与权限键。
type Snapshot struct {
	Principal   iamclient.Principal // 已验证的账号投影。
	Permissions []string            // 有效权限键集合。
}

// Evaluate 只判断权限状态、近期认证与权限键，不做任何作用域分支。
// 业务服务仍需自行校验实际资源归属、状态和审批要求。
func Evaluate(s Snapshot, p Permission, now time.Time, recentWindow time.Duration) error {
	if p.Status != enum.PermissionStatusActive {
		return ErrDenied
	}

	if p.RequiresRecentAuth {
		authenticated := s.Principal.AuthenticatedAt
		if s.Principal.ReauthenticatedAt != nil {
			authenticated = *s.Principal.ReauthenticatedAt
		}

		if authenticated.IsZero() || now.Sub(authenticated) > recentWindow || authenticated.After(now.Add(time.Minute)) {
			return ErrRecentAuthRequired
		}
	}

	if !slices.Contains(s.Permissions, p.Key) {
		return ErrDenied
	}

	return nil
}

// CheckDelegation 同时校验原有和拟设置的权限，防止低权限操作者修改高权限角色。
// 操作者只能授予自己当前持有、且目录标记 delegable 的权限；没有作用域条件。
func CheckDelegation(effective []string, existing, candidate []string, catalog []Permission) error {
	availableExisting, availableNew := map[string]bool{}, map[string]bool{}
	for _, p := range catalog {
		if !p.Delegable || !slices.Contains(effective, p.Key) {
			continue
		}

		availableExisting[p.Key] = true

		if p.Status == enum.PermissionStatusActive {
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
