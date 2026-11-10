package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// Queries 将数据操作绑定到调用方的连接或事务；方法不会切回根连接。
type Queries struct{ db *gorm.DB }

// NewQueries 创建当前连接或事务的查询入口。
func NewQueries(db *gorm.DB) *Queries { return &Queries{db: db} }

// UserRow 是账号在写入路径上的投影，只含账号与访问状态，不含凭据。
type UserRow struct {
	ID          uuid.UUID       // 资源标识。
	Username    string          // 登录账号。
	DisplayName string          // 界面显示名称。
	Status      enum.UserStatus // 当前启用状态，取值见 enum.UserStatus。
	Configured  bool            // 是否为配置托管的内置管理员。
	AuthEpoch   int64           // 账号版本；变化后旧会话失效。
}

// LoginUser 在 UserRow 之上补齐登录所需的凭据状态，密码哈希禁止写入响应和日志。
type LoginUser struct {
	UserRow
	PasswordHash   string     // 仅供内部校验的密码哈希。
	FailedAttempts int32      // 连续失败次数。
	LockedUntil    *time.Time // 账户锁定截止时间；空表示未锁定。
}

// GetLoginUser 读取账号和密码状态，不对外序列化凭据。
func (q *Queries) GetLoginUser(ctx context.Context, username string) (LoginUser, error) {
	var user LoginUser
	err := q.db.WithContext(ctx).Table("iam.users AS u").
		Select("u.id,u.username,u.display_name,u.status,u.configured,u.auth_epoch,c.password_hash,c.failed_attempts,c.locked_until").
		Joins("JOIN iam.password_credentials c ON c.user_id=u.id").
		Where("u.username_normalized = ?", username).Take(&user).Error
	return user, err
}

// LockUser 锁定用户行，所有账号写入均先取得用户锁。
// 只读取账号与访问状态：凭据属于 password_credentials，不在这张表里。
func (q *Queries) LockUser(ctx context.Context, id uuid.UUID) (UserRow, error) {
	return gorm.G[UserRow](q.db, clause.Locking{Strength: "UPDATE"}).Table("iam.users").Where("id = ?", id).Take(ctx)
}

// Session 是数据库中可撤销的会话状态。
type Session struct {
	ID                uuid.UUID          // 资源标识。
	UserID            uuid.UUID          // 会话所属用户。
	TokenHash         string             // 会话令牌摘要，不包含原始凭据。
	AuthEpoch         int64              // 账号版本；变化后旧会话失效。
	State             enum.SessionState  // 会话状态，取值见 enum.SessionState。
	AuthenticatedAt   time.Time          // 首次认证时间。
	ReauthenticatedAt pgtype.Timestamptz // 最近一次密码确认时间。
	AbsoluteExpiresAt time.Time          // 会话绝对到期时间。
	IdleExpiresAt     time.Time          // 空闲到期时间，不得晚于绝对到期。
}

// LockSession 在用户锁之后取得会话行锁。
func (q *Queries) LockSession(ctx context.Context, id uuid.UUID) (Session, error) {
	var session Session
	err := q.db.WithContext(ctx).Table("iam.user_sessions").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).Take(&session).Error
	return session, err
}

// TouchSessionParams 是会话续期需要的当前版本和空闲时长。
type TouchSessionParams struct {
	ID          uuid.UUID     // 资源标识。
	TokenHash   string        // 会话令牌摘要，不包含原始凭据。
	AuthEpoch   int64         // 账号版本；变化后旧会话失效。
	IdleTimeout time.Duration // 允许空闲的时间间隔；由持久化层转换为数据库区间。
}

// TouchSession 使用取得锁后的数据库时间续期；已过期状态不能被重新激活。
func (q *Queries) TouchSession(ctx context.Context, input TouchSessionParams) (Session, error) {
	idle := pgtype.Interval{Microseconds: input.IdleTimeout.Microseconds(), Valid: true}
	var session Session
	response := q.db.WithContext(ctx).Table("iam.user_sessions").Model(&session).
		Clauses(clause.Returning{}).
		Where("id = ? AND token_hash = ? AND state = 'active' AND auth_epoch = ? AND idle_expires_at > clock_timestamp() AND absolute_expires_at > clock_timestamp()", input.ID, input.TokenHash, input.AuthEpoch).
		Updates(map[string]any{
			"idle_expires_at": gorm.Expr("LEAST(absolute_expires_at,GREATEST(idle_expires_at,clock_timestamp()+?::interval))", idle),
			"last_seen_at":    gorm.Expr("GREATEST(last_seen_at,clock_timestamp())"),
		})
	if response.Error != nil {
		return session, response.Error
	}

	if response.RowsAffected == 0 {
		return session, gorm.ErrRecordNotFound
	}

	return session, nil
}

// RevokeUserSessionsParams 指定需要撤销的用户和原因。
type RevokeUserSessionsParams struct {
	UserID       uuid.UUID // 会话所属用户。
	RevokeReason string    // 撤销原因，不包含凭据；空串表示不记录。
}

// RevokeUserSessions 在当前事务内撤销该用户的全部有效会话。
func (q *Queries) RevokeUserSessions(ctx context.Context, input RevokeUserSessionsParams) error {
	return q.db.WithContext(ctx).Table("iam.user_sessions").Where("user_id = ? AND state = 'active'", input.UserID).
		Updates(map[string]any{
			"state":         string(enum.SessionStateRevoked),
			"revoked_at":    gorm.Expr("clock_timestamp()"),
			"revoke_reason": input.RevokeReason,
		}).Error
}
