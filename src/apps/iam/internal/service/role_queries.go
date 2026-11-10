package service

import (
	"context"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// Roles 返回全局角色列表；实际写入时仍须在 Manage 中重新鉴权。
func (s *Service) Roles(ctx context.Context, limit, offset int) (map[string]any, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMRolesRead); err != nil {
		return nil, err
	}

	return repository.ListRoles(ctx, s.Store.DB, limit, offset)
}

// Role 返回单个角色的公开投影。
func (s *Service) Role(ctx context.Context, roleID string) ([]byte, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMRolesRead); err != nil {
		return nil, err
	}

	return repository.RoleJSON(ctx, s.Store.DB, roleID)
}
