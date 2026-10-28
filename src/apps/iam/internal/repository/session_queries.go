package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// RecordLoginFailure 记录登录失败并按数据库时间更新锁定期限。
func RecordLoginFailure(ctx context.Context, db *gorm.DB, userID uuid.UUID, loginMaxFailures int, value pgtype.Interval) error {
	return db.WithContext(ctx).Exec(`UPDATE iam.password_credentials SET failed_attempts=CASE WHEN locked_until<=clock_timestamp() THEN 1 ELSE failed_attempts+1 END,locked_until=CASE WHEN locked_until>clock_timestamp() THEN locked_until WHEN (CASE WHEN locked_until<=clock_timestamp() THEN 1 ELSE failed_attempts+1 END)>=$2 THEN clock_timestamp()+$3::interval ELSE NULL END,updated_at=now() WHERE user_id=$1`, userID, loginMaxFailures, value).Error
}

// SessionUser 读取会话所属用户，供全局锁顺序使用。
func SessionUser(ctx context.Context, db *gorm.DB, id uuid.UUID, userID *uuid.UUID) error {
	return db.WithContext(ctx).Raw(`SELECT user_id FROM iam.user_sessions WHERE id=$1`, id).Row().Scan(userID)
}

// HasStoredSession 检查 SCS 记录是否仍在有效期内。
func HasStoredSession(ctx context.Context, db *gorm.DB, tokenHash string, stored *bool) error {
	return db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM iam.scs_sessions WHERE token=$1 AND expiry>clock_timestamp())`, tokenHash).Row().Scan(stored)
}

// VerifyLoginCredential 在用户锁内再次校验凭据快照和锁定状态。
func VerifyLoginCredential(ctx context.Context, db *gorm.DB, userID uuid.UUID, passwordHash string, valid *bool) error {
	return db.WithContext(ctx).Raw(`SELECT password_hash=$2 AND (locked_until IS NULL OR locked_until<=clock_timestamp()) FROM iam.password_credentials WHERE user_id=$1 FOR UPDATE`, userID, passwordHash).Row().Scan(valid)
}

// DatabaseTime 读取数据库当前时间，避免应用时钟影响授权期限。
func DatabaseTime(ctx context.Context, db *gorm.DB, now *time.Time) error {
	return db.WithContext(ctx).Raw(`SELECT clock_timestamp()`).Row().Scan(now)
}

// RevokeRotatedSession 只撤销与旧令牌匹配的有效会话。
func RevokeRotatedSession(ctx context.Context, db *gorm.DB, oldID uuid.UUID, value string) error {
	return db.WithContext(ctx).Exec(`UPDATE iam.user_sessions SET state='revoked',revoked_at=clock_timestamp(),revoke_reason='rotation' WHERE id=$1 AND token_hash=$2 AND state='active'`, oldID, value).Error
}

// InsertSession 写入绑定令牌摘要的逻辑会话，期限由认证用例确定。
func InsertSession(ctx context.Context, db *gorm.DB, newID uuid.UUID, userID uuid.UUID, value string, authVersion int64, authenticated time.Time, reauthenticated *time.Time, absolute time.Time, idle time.Time, now time.Time, ip any, userAgent string) error {
	return db.WithContext(ctx).Exec(
		`INSERT INTO iam.user_sessions(id,user_id,token_hash,auth_version,state,authenticated_at,reauthenticated_at,absolute_expires_at,idle_expires_at,last_seen_at,client_ip,user_agent) VALUES($1,$2,$3,$4,'active',$5,$6,$7,$8,$9,$10,$11)`,
		newID,
		userID,
		value,
		authVersion,
		authenticated,
		reauthenticated,
		absolute,
		idle,
		now,
		ip,
		userAgent,
	).Error
}

// ResetLoginFailures 成功登录后清除账户失败计数。
func ResetLoginFailures(ctx context.Context, db *gorm.DB, userID uuid.UUID) error {
	return db.WithContext(ctx).Exec(`UPDATE iam.password_credentials SET failed_attempts=0,locked_until=NULL,updated_at=now() WHERE user_id=$1`, userID).Error
}

// ActiveSessions 读取仍同时满足 SCS、身份版本和逻辑期限的会话。
func ActiveSessions(ctx context.Context, db *gorm.DB, userID string, authVersion int64) (*sql.Rows, error) {
	return db.WithContext(ctx).Raw(`SELECT s.id::text,s.authenticated_at,s.last_seen_at,s.idle_expires_at,s.absolute_expires_at,s.user_agent,COALESCE(s.client_ip::text,'') FROM iam.user_sessions s JOIN iam.scs_sessions c ON c.token=s.token_hash AND c.expiry>clock_timestamp() WHERE s.user_id=$1 AND s.state='active' AND s.auth_version=$2 AND s.idle_expires_at>clock_timestamp() AND s.absolute_expires_at>clock_timestamp() ORDER BY s.created_at DESC LIMIT 100`, userID, authVersion).Rows()
}

// CompensateSession 撤销 SCS 提交失败的逻辑会话，禁止产生孤立登录。
func CompensateSession(ctx context.Context, db *gorm.DB, sessionID string) error {
	return db.WithContext(ctx).Exec(`UPDATE iam.user_sessions SET state='revoked',revoked_at=clock_timestamp(),revoke_reason='storage_failure' WHERE id=$1`, sessionID).Error
}
