package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// Me 返回当前账号、直接持有的角色、有效权限与展示时区偏好。
// 响应里没有组织、作用域或平台能力：授权只有用户 → 角色 → 权限一条路径。
func (s *Service) Me(ctx context.Context) (Identity, error) {
	p, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return Identity{}, err
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return Identity{}, repository.ErrUnauthenticated
	}

	roles, err := repository.UserRoles(ctx, s.Store.DB, userID)
	if err != nil {
		return Identity{}, err
	}

	permissions, err := repository.EffectivePermissions(ctx, s.Store.DB, userID)
	if err != nil {
		return Identity{}, err
	}

	timeZone, err := repository.UserTimeZone(ctx, s.Store.DB, userID)
	if err != nil {
		return Identity{}, err
	}

	if roles == nil {
		roles = []repository.RoleRef{}
	}

	return Identity{Principal: p, Permissions: permissions, TimeZone: timeZone, Roles: roles}, nil
}
