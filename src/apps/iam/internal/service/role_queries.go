package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// Roles 返回全局角色列表；实际写入时仍须在 Manage 中重新鉴权。
func (s *Service) Roles(ctx context.Context, limit, offset int) (repository.RolePage, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMRolesRead); err != nil {
		return repository.RolePage{}, err
	}

	return repository.ListRoles(ctx, s.Store.DB, limit, offset)
}

// Role 返回单个角色的公开投影。
func (s *Service) Role(ctx context.Context, roleID string) (repository.Role, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMRolesRead); err != nil {
		return repository.Role{}, err
	}

	id, err := uuid.Parse(roleID)
	if err != nil {
		return repository.Role{}, repository.ErrInvalid
	}

	return repository.RoleByID(ctx, s.Store.DB, id)
}
