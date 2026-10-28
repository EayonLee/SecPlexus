package service

import (
	"context"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// ListUsers 校验当前身份的范围后读取业务列表。
func (s *Service) ListUsers(ctx context.Context, limit, offset int, query string) (map[string]any, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMPlatformUsersRead, ""); err != nil {
		return nil, err
	}

	return repository.ListUsers(ctx, s.Store.DB, limit, offset, query)
}

// ListOrganizations 校验当前身份的范围后读取业务列表。
func (s *Service) ListOrganizations(ctx context.Context, limit, offset int) (map[string]any, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMPlatformOrganizationsRead, ""); err != nil {
		return nil, err
	}

	return repository.ListOrganizations(ctx, s.Store.DB, limit, offset)
}

// ListMembers 校验当前身份的范围后读取业务列表。
func (s *Service) ListMembers(ctx context.Context, limit, offset int, orgID string) (map[string]any, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMMembersRead, orgID); err != nil {
		return nil, err
	}

	return repository.ListMembers(ctx, s.Store.DB, limit, offset, orgID)
}

// ListRoleMembers 校验当前身份的范围后读取业务列表。
func (s *Service) ListRoleMembers(ctx context.Context, limit, offset int, orgID, roleID string) (map[string]any, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMMembersRead, orgID); err != nil {
		return nil, err
	}

	if _, err := s.Authorize(ctx, contract.PermissionIAMRolesRead, orgID); err != nil {
		return nil, err
	}

	return repository.ListRoleMembers(ctx, s.Store.DB, limit, offset, orgID, roleID)
}

// ListAuditEvents 区分平台审计能力与组织审计能力，组织参数不替代授权。
func (s *Service) ListAuditEvents(ctx context.Context, orgID string, platform bool, limit, offset int) (map[string]any, error) {
	action := contract.PermissionIAMAuditRead
	scope := orgID
	if platform {
		action = contract.PermissionIAMPlatformAuditRead
		scope = ""
	}

	if _, err := s.Authorize(ctx, action, scope); err != nil {
		return nil, err
	}

	return repository.ListAuditEvents(ctx, s.Store.DB, orgID, limit, offset)
}

// User 查询前校验平台用户读取权限。
func (s *Service) User(ctx context.Context, id string) ([]byte, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMPlatformUsersRead, ""); err != nil {
		return nil, err
	}

	var data []byte
	err := repository.UserJSON(ctx, s.Store.DB, id, &data)
	return data, err
}

// Organization 查询前校验目标组织读取权限。
func (s *Service) Organization(ctx context.Context, orgID string) ([]byte, error) {
	if _, err := s.Authorize(ctx, contract.PermissionIAMOrganizationsRead, orgID); err != nil {
		return nil, err
	}

	var data []byte
	err := repository.OrganizationJSON(ctx, s.Store.DB, orgID, &data)
	return data, err
}
