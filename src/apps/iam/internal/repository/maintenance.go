package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
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

	if err = tx.WithContext(ctx).Exec(`DELETE FROM iam.scs_sessions WHERE token IN (SELECT token FROM iam.scs_sessions WHERE expiry<clock_timestamp() LIMIT 1000)`).Error; err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Exec(`DELETE FROM iam.user_sessions WHERE id IN (SELECT id FROM iam.user_sessions WHERE absolute_expires_at<clock_timestamp()-$1::interval LIMIT 1000)`, pgtype.Interval{Microseconds: (time.Duration(sessionDays) * 24 * time.Hour).Microseconds(),
		Valid: true}).Error; err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Exec(`DELETE FROM iam.audit_events WHERE id IN (SELECT id FROM iam.audit_events WHERE occurred_at<clock_timestamp()-$1::interval LIMIT 1000)`, pgtype.Interval{Microseconds: (time.Duration(auditDays) * 24 * time.Hour).Microseconds(),
		Valid: true}).Error; err != nil {
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

	if err = tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock($1)`, ManagementLock).Error; err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Exec(`SELECT id FROM iam.users ORDER BY id FOR UPDATE`).Error; err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Exec(`UPDATE iam.users SET auth_version=auth_version+1,updated_at=now()`).Error; err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Exec(`UPDATE iam.user_sessions SET state='revoked',revoked_at=clock_timestamp(),revoke_reason='database_restore' WHERE state='active'`).Error; err != nil {
		return err
	}

	if err = tx.WithContext(ctx).Exec(`DELETE FROM iam.scs_sessions`).Error; err != nil {
		return err
	}

	if err = AppendAudit(ctx, tx, Audit{
		ActorType:  "deployment_operator",
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
	hash, err := (identity.PasswordHasher{}).GenerateHash(password)
	if err != nil {
		return err
	}

	tx := s.DB.WithContext(ctx).Begin()
	err = tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err = tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock($1)`, ManagementLock).Error; err != nil {
		return err
	}

	q := NewQueries(tx)
	if _, err = q.LockMutableUser(ctx, userID); err != nil {
		return err
	}

	if err = BumpUserAuthVersion(ctx, tx, userID); err != nil {
		return err
	}

	if err = ResetPassword(ctx, tx, userID, hash); err != nil {
		return err
	}

	if err = q.RevokeUserSessions(ctx, RevokeUserSessionsParams{UserID: userID,
		RevokeReason: pgtype.Text{String: "operator_recovery",
			Valid: true}}); err != nil {
		return err
	}

	if err = AppendAudit(ctx, tx, Audit{
		ActorType:  "deployment_operator",
		Action:     "iam.password.recover",
		TargetType: "user",
		TargetID:   &userID,
		RequestID:  uuid.NewString(),
	}); err != nil {
		return err
	}

	return tx.Commit().Error
}
