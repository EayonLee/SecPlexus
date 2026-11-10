package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// Maintain 使用模块数据库连接分批清理过期审计记录和会话。
func (s *Store) Maintain(ctx context.Context, auditDays, sessionDays int) error {
	if auditDays < 1 || sessionDays < 1 {
		return ErrInvalid
	}

	tx := s.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	q := queries(tx)
	if err = q.PurgeExpiredStoredSessions(ctx); err != nil {
		return err
	}

	if err = q.PurgeExpiredSessions(ctx, (time.Duration(sessionDays) * 24 * time.Hour).Microseconds()); err != nil {
		return err
	}

	// 保留期清理是唯一允许删除审计的路径：在本次事务内显式开闸。
	if err = q.AllowAuditPurge(ctx); err != nil {
		return err
	}

	if err = q.PurgeExpiredAuditEvents(ctx, (time.Duration(auditDays) * 24 * time.Hour).Microseconds()); err != nil {
		return err
	}

	return tx.Commit().Error
}

// RevokeAfterRestore 在恢复后的服务接收流量前撤销所有会话，避免旧备份恢复已撤销的会话。
func (s *Store) RevokeAfterRestore(ctx context.Context) error {
	tx := s.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err = LockManagement(ctx, tx, ManagementLock); err != nil {
		return err
	}

	q := queries(tx)
	if err = q.LockAllUsers(ctx); err != nil {
		return err
	}

	if err = q.BumpAllUserAuthEpochs(ctx); err != nil {
		return err
	}

	if err = q.RevokeAllActiveSessions(ctx, "database_restore"); err != nil {
		return err
	}

	if err = q.ClearStoredSessions(ctx); err != nil {
		return err
	}

	if err = AppendAudit(ctx, tx, Audit{
		ActorType:  enum.ActorTypeDeploymentOperator,
		Action:     "iam.sessions.revoke_after_restore",
		TargetType: "platform",
		RequestID:  uuid.NewString(),
	}); err != nil {
		return err
	}

	return tx.Commit().Error
}

// RecoverPassword 使用与管理员重置密码相同的密码策略、会话撤销和审计规则。
func (s *Store) RecoverPassword(ctx context.Context, userID uuid.UUID, password string) error {
	hash, err := (account.PasswordHasher{}).GenerateHash(password)
	if err != nil {
		return err
	}

	tx := s.DB.WithContext(ctx).Begin()
	err = tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err = LockManagement(ctx, tx, ManagementLock); err != nil {
		return err
	}

	user, err := LockedAccount(ctx, tx, userID)
	if err != nil {
		return err
	}

	if user.Configured {
		return ErrConfiguredUserImmutable
	}

	if err = BumpUserAuthEpoch(ctx, tx, userID); err != nil {
		return err
	}

	if err = ResetPassword(ctx, tx, userID, hash); err != nil {
		return err
	}

	if err = RevokeUserSessions(ctx, tx, RevokeUserSessionsParams{UserID: userID,
		RevokeReason: "operator_recovery"}); err != nil {
		return err
	}

	if err = AppendAudit(ctx, tx, Audit{
		ActorType:  enum.ActorTypeDeploymentOperator,
		Action:     "iam.password.recover",
		TargetType: "user",
		TargetID:   &userID,
		RequestID:  uuid.NewString(),
	}); err != nil {
		return err
	}

	return tx.Commit().Error
}
