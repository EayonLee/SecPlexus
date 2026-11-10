package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// RecordLoginFailure 记录登录失败并按数据库时间更新锁定期限。
func RecordLoginFailure(ctx context.Context, db *gorm.DB, userID uuid.UUID, loginMaxFailures int, lockDuration time.Duration) error {
	return queries(db).RecordLoginFailure(ctx, sqlcgen.RecordLoginFailureParams{
		UserID:      userID,
		MaxFailures: int32(loginMaxFailures),
		LockMicros:  lockDuration.Microseconds(),
	})
}

// SessionUser 读取会话所属用户，供全局锁顺序使用；会话不存在时返回 sql.ErrNoRows。
func SessionUser(ctx context.Context, db *gorm.DB, id uuid.UUID) (uuid.UUID, error) {
	return queries(db).SessionUserID(ctx, id)
}

// HasStoredSession 检查 SCS 记录是否仍在有效期内。
func HasStoredSession(ctx context.Context, db *gorm.DB, tokenHash string) (bool, error) {
	return queries(db).HasStoredSession(ctx, tokenHash)
}

// VerifyLoginCredential 在用户锁内再次校验凭据快照和锁定状态。
func VerifyLoginCredential(ctx context.Context, db *gorm.DB, userID uuid.UUID, passwordHash string) (bool, error) {
	return queries(db).VerifyLoginCredential(ctx, sqlcgen.VerifyLoginCredentialParams{
		UserID:       userID,
		PasswordHash: passwordHash,
	})
}

// DatabaseTime 读取数据库当前时间，避免应用时钟影响授权期限。
func DatabaseTime(ctx context.Context, db *gorm.DB) (time.Time, error) {
	return queries(db).DatabaseTime(ctx)
}

// RevokeRotatedSession 只撤销与旧令牌匹配的有效会话。
func RevokeRotatedSession(ctx context.Context, db *gorm.DB, oldID uuid.UUID, value string) error {
	return queries(db).RevokeRotatedSession(ctx, sqlcgen.RevokeRotatedSessionParams{ID: oldID, TokenHash: value})
}

// ResetLoginFailures 成功登录后清除账户失败计数。
func ResetLoginFailures(ctx context.Context, db *gorm.DB, userID uuid.UUID) error {
	return queries(db).ResetLoginFailures(ctx, userID)
}

// CompensateSession 撤销 SCS 提交失败的逻辑会话，禁止产生孤立登录。
func CompensateSession(ctx context.Context, db *gorm.DB, sessionID string) error {
	return queries(db).CompensateSession(ctx, sessionID)
}
