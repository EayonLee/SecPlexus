package service

import (
	"context"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// PermissionView 描述权限及当前操作者是否可以把它分配给别人。
type PermissionView struct {
	Permission  contract.Permission `json:"permission"`   // 权限目录中的定义。
	CanDelegate bool                `json:"can_delegate"` // 当前操作者是否可以分配该权限。
}

// PermissionCatalog 返回组织权限目录，并按当前用户的实际权限计算可委派范围。
func (s *Service) PermissionCatalog(ctx context.Context, orgID string) ([]PermissionView, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMRolesRead, orgID); err != nil {
		return nil, err
	}

	principal, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	// 权限来自数据库中的有效授权，不能根据角色名称推断。
	rows, err := repository.EffectiveRoleGrants(ctx, s.Store.DB, orgID, principal.UserID)
	if err != nil {
		return nil, err
	}

	effective := map[string]bool{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}

		effective[key] = true
	}

	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	catalog, err := repository.Permissions(ctx, s.Store.DB)
	if err != nil {
		return nil, err
	}

	items := make([]PermissionView, 0, len(catalog))
	for _, permission := range catalog {
		if permission.Scope != "organization" || permission.Status == "retired" {
			continue
		}

		items = append(items, PermissionView{
			Permission:  permission,
			CanDelegate: permission.Status == "active" && permission.Delegable && (principal.IsSuperAdmin || effective[permission.Key]),
		})
	}

	return items, nil
}
