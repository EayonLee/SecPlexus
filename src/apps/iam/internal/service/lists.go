package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// AuditQuery 是审计列表的请求参数别名，绑定在 model 层。
type AuditQuery = model.AuditQuery

// ListUsers 校验当前身份的范围后读取业务列表。
func (s *Service) ListUsers(ctx context.Context, limit, offset int, query string) (map[string]any, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMPlatformUsersRead, ""); err != nil {
		return nil, err
	}

	return repository.ListUsers(ctx, s.Store.DB, limit, offset, query)
}

// ListOrganizations 校验当前身份的范围后读取业务列表。
func (s *Service) ListOrganizations(ctx context.Context, limit, offset int) (map[string]any, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMPlatformOrganizationsRead, ""); err != nil {
		return nil, err
	}

	return repository.ListOrganizations(ctx, s.Store.DB, limit, offset)
}

// ListOrganizationUsers 校验当前身份的范围后读取业务列表。
// ListOrganizationUsers 读取组织用户。subtree 为真时聚合全部后代组织的用户，
// 每行带 org_slug 标明所属组织；同一用户可能因跨组织用户关系出现多行。
func (s *Service) ListOrganizationUsers(ctx context.Context, limit, offset int, orgID string, subtree bool) (map[string]any, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMUsersRead, orgID); err != nil {
		return nil, err
	}

	return repository.ListOrganizationUsers(ctx, s.Store.DB, limit, offset, orgID, subtree)
}

// ListRoleUsers 校验当前身份的范围后读取业务列表。
func (s *Service) ListRoleUsers(ctx context.Context, limit, offset int, orgID, roleID string) (map[string]any, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMUsersRead, orgID); err != nil {
		return nil, err
	}

	if _, err := s.Authorize(ctx, iamclient.PermissionIAMRolesRead, orgID); err != nil {
		return nil, err
	}

	return repository.ListRoleUsers(ctx, s.Store.DB, limit, offset, orgID, roleID)
}

// ListAuditEvents 读取审计日志。可见范围完全由服务端授权决定：
// 拥有平台审计权限者可看全部；仅有组织审计权限者只看自己所属组织。
// 客户端提交的筛选条件不能扩大这个范围。
func (s *Service) ListAuditEvents(ctx context.Context, query AuditQuery) (map[string]any, error) {
	filter := repository.AuditFilter{
		Actor:   query.Actor,
		Outcome: query.Outcome,
		From:    query.From,
		To:      query.To,
	}

	if _, err := s.Authorize(ctx, iamclient.PermissionIAMPlatformAuditRead, ""); err != nil {
		// 平台审计不成立时退回组织范围：先确认身份，再限定到自己所属的组织。
		p, resolveErr := s.Sessions.Resolve(ctx)
		if resolveErr != nil {
			return nil, resolveErr
		}

		userID, parseErr := uuid.Parse(p.UserID)
		if parseErr != nil {
			return nil, repository.ErrUnauthenticated
		}

		organizations, orgErr := repository.VisibleOrganizations(ctx, s.Store.DB, userID, p.IsSuperAdmin)
		if orgErr != nil {
			return nil, orgErr
		}

		orgIDs := make([]string, 0, len(organizations))
		for _, organization := range organizations {
			orgIDs = append(orgIDs, organization.ID)
		}

		if len(orgIDs) == 0 {
			return map[string]any{"items": []any{}, "total": int64(0), "limit": query.Limit, "offset": query.Offset}, nil
		}

		filter.OrgIDs = orgIDs
	}

	return repository.ListAuditEvents(ctx, s.Store.DB, filter, query.Limit, query.Offset)
}

// User 查询前校验平台用户读取权限。
func (s *Service) User(ctx context.Context, id string) ([]byte, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMPlatformUsersRead, ""); err != nil {
		return nil, err
	}

	var data []byte
	err := repository.UserJSON(ctx, s.Store.DB, id, &data)
	return data, err
}
