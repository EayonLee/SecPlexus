package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// UpdateTimeZone 保存本人展示时区偏好。
// 该偏好只影响渲染，因此不递增身份版本、不撤销会话，与改密不同。
func (s *Service) UpdateTimeZone(ctx context.Context, timeZone string, requestID string) error {
	ctx = logging.With(ctx, "operation", "iam.profile.timezone")
	p, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return err
	}

	if !identity.ValidTimeZone(timeZone) {
		return repository.ErrInvalid
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	tx := s.Store.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	if err = repository.UpdateTimeZone(ctx, tx, userID, timeZone); err != nil {
		return err
	}

	if err = repository.AppendAudit(ctx, tx, repository.Audit{
		ActorID:    &userID,
		ActorName:  p.Username,
		Action:     "iam.profile.timezone",
		TargetType: "user",
		TargetID:   &userID,
		RequestID:  requestID,
	}); err != nil {
		return err
	}

	if err = tx.Commit().Error; err != nil {
		return err
	}

	logging.From(ctx).Info("展示时区已更新", "event", "iam.timezone_updated")
	return nil
}
