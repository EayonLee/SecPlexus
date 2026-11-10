package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// ListUsers 校验用户读取权限后读取列表；支持用户名、显示名与邮箱模糊搜索。
func (s *Service) ListUsers(ctx context.Context, limit, offset int, query string) (repository.UserPage, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMUsersRead); err != nil {
		return repository.UserPage{}, err
	}

	return repository.ListUsers(ctx, s.Store.DB, limit, offset, query)
}

// ListAuditEvents 读取审计日志。可见范围完全由 iam.audit.read 决定，
// 客户端提交的筛选条件只能收窄结果；分页只向前，避免深 OFFSET。
func (s *Service) ListAuditEvents(ctx context.Context, filter AuditFilter) (repository.AuditPage, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMAuditRead); err != nil {
		return repository.AuditPage{}, err
	}

	return repository.ListAuditEvents(ctx, s.Store.DB, repository.AuditFilter{
		Actor:   filter.Actor,
		Outcome: filter.Outcome,
		From:    filter.From,
		To:      filter.To,
	}, filter.Limit, filter.Cursor)
}

// User 查询前校验用户读取权限。
func (s *Service) User(ctx context.Context, id string) (repository.UserSummary, error) {
	if _, err := s.Authorize(ctx, iamclient.PermissionIAMUsersRead); err != nil {
		return repository.UserSummary{}, err
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		return repository.UserSummary{}, repository.ErrInvalid
	}

	return repository.UserByID(ctx, s.Store.DB, userID)
}
