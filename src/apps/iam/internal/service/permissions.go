package service

import (
	"context"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/google/uuid"
)

// PermissionView 描述权限及当前操作者是否可以把它分配给别人。
type PermissionView struct {
	Permission  authorization.Permission `json:"permission"`   // 权限目录中的定义。
	CanDelegate bool                     `json:"can_delegate"` // 当前操作者是否可以分配该权限。
}

// PermissionCatalog 返回扁平权限目录，并按当前用户的实际权限计算可委派范围。
func (s *Service) PermissionCatalog(ctx context.Context) ([]PermissionView, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMRolesRead); err != nil {
		return nil, err
	}

	principal, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	userID, err := uuid.Parse(principal.UserID)
	if err != nil {
		return nil, repository.ErrUnauthenticated
	}

	// 权限来自数据库中的有效授权，不能根据角色名称或超管标记推断。
	granted, err := repository.EffectivePermissions(ctx, s.Store.DB, userID)
	if err != nil {
		return nil, err
	}

	effective := map[string]bool{}
	for _, key := range granted {
		effective[key] = true
	}

	catalog, err := repository.Permissions(ctx, s.Store.DB)
	if err != nil {
		return nil, err
	}

	items := make([]PermissionView, 0, len(catalog))
	for _, permission := range catalog {
		if permission.Status == enum.PermissionStatusRetired {
			continue
		}

		items = append(items, PermissionView{
			Permission:  permission,
			CanDelegate: permission.Status == enum.PermissionStatusActive && permission.Delegable && effective[permission.Key],
		})
	}

	return items, nil
}
