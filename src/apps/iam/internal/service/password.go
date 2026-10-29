package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// ChangePassword 验证当前密码、修改密码并撤销受影响的会话。
func (s *Service) ChangePassword(ctx context.Context, oldPassword, newPassword, requestID string) error {
	ctx = logging.With(ctx, "operation", "iam.password.change")
	p, err := s.Sessions.Resolve(ctx)
	if err != nil {
		return err
	}

	normalized, err := identity.NormalizeUsername(p.Username)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	current, err := repository.NewQueries(s.Store.DB).GetLoginUser(ctx, normalized)
	if err != nil {
		return err
	}

	hasher := identity.PasswordHasher{}
	if err = hasher.CompareHashAndPassword(current.PasswordHash, oldPassword); err != nil {
		return identity.ErrPasswordMismatch
	}

	if err = hasher.CompareHashAndPassword(current.PasswordHash, newPassword); err == nil {
		return repository.ErrInvalid
	}

	hash, err := hasher.GenerateHash(newPassword)
	if err != nil {
		return repository.ErrInvalid
	}

	tx := s.Store.DB.WithContext(ctx).Begin()
	err = tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err = repository.LockManagement(ctx, tx, repository.ManagementLock); err != nil {
		return err
	}

	fresh, err := s.resolve(ctx, tx)
	if err != nil {
		return err
	}

	if fresh.UserID != p.UserID || fresh.AuthVersion != current.AuthVersion {
		return repository.ErrUnauthenticated
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return err
	}

	if err = repository.BumpUserAuthVersion(ctx, tx, userID); err != nil {
		return err
	}

	if err = repository.ResetPassword(ctx, tx, userID, hash); err != nil {
		return err
	}

	if err = repository.NewQueries(tx).RevokeUserSessions(ctx, repository.RevokeUserSessionsParams{UserID: userID,
		RevokeReason: text("password_change")}); err != nil {
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

	logging.From(ctx).Info("密码已更新并撤销旧会话", "event", "iam.password_changed", "user_id", p.UserID)
	return s.Sessions.Manager.Destroy(ctx)
}
