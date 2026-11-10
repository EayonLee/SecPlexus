package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// ChangePassword 验证当前密码、修改密码并撤销受影响的会话。
func (s *Service) ChangePassword(ctx context.Context, oldPassword, newPassword, requestID string) error {
	ctx = logging.With(ctx, "operation", "iam.password.change")
	p, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return err
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	hasher := account.PasswordHasher{}
	tx := s.Store.DB.WithContext(ctx).Begin()
	err = tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err = repository.LockManagement(ctx, tx, repository.ManagementLock); err != nil {
		return err
	}

	locked, err := repository.LockedAccount(ctx, tx, userID)
	if err != nil {
		return err
	}

	if locked.Configured {
		// 配置管理员的密码只能由部署配置更新，否则下次启动会被覆盖。
		return repository.ErrConfiguredUserImmutable
	}

	normalized, err := account.NormalizeUsername(p.Username)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	current, err := repository.GetLoginUser(ctx, tx, normalized)
	if err != nil {
		return err
	}

	if current.AuthEpoch != locked.AuthEpoch {
		return repository.ErrUnauthenticated
	}

	if err = hasher.CompareHashAndPassword(current.PasswordHash, oldPassword); err != nil {
		return account.ErrPasswordMismatch
	}

	if err = hasher.CompareHashAndPassword(current.PasswordHash, newPassword); err == nil {
		return repository.ErrInvalid
	}

	hash, err := hasher.GenerateHash(newPassword)
	if err != nil {
		return repository.ErrInvalid
	}

	if err = repository.BumpUserAuthEpoch(ctx, tx, userID); err != nil {
		return err
	}

	if err = repository.ResetPassword(ctx, tx, userID, hash); err != nil {
		return err
	}

	if err = repository.RevokeUserSessions(ctx, tx, repository.RevokeUserSessionsParams{UserID: userID,
		RevokeReason: "password_change"}); err != nil {
		return err
	}

	if err = repository.AppendAudit(ctx, tx, repository.Audit{
		ActorID:    &userID,
		ActorName:  p.Username,
		Action:     "iam.password.change",
		TargetType: "user",
		TargetID:   &userID,
		RequestID:  requestID,
	}); err != nil {
		return err
	}

	if err = tx.Commit().Error; err != nil {
		return err
	}

	logging.From(ctx).Info("Password changed and old sessions revoked", "event", "iam.password_changed", "user_id", p.UserID)
	return s.Sessions.Manager.Destroy(ctx)
}
