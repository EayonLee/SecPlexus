package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

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
func GetLoginUser(ctx context.Context, db DB, username string) (LoginUser, error) {
	row, err := queries(db).GetLoginUser(ctx, username)
	if err != nil {
		return LoginUser{}, err
	}

	user := LoginUser{
		UserRow: UserRow{
			ID:          row.ID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Status:      row.Status,
			Configured:  row.Configured,
			AuthEpoch:   row.AuthEpoch,
		},
		PasswordHash:   row.PasswordHash,
		FailedAttempts: row.FailedAttempts,
	}

	if row.LockedUntil.Valid {
		user.LockedUntil = &row.LockedUntil.Time
	}

	return user, nil
}

// LockedAccount 锁定用户行，所有账号写入均先取得用户锁。
// 只读取账号与访问状态：凭据属于 password_credentials，不在这张表里。
func LockedAccount(ctx context.Context, db DB, id uuid.UUID) (UserRow, error) {
	row, err := queries(db).LockedAccount(ctx, id)
	if err != nil {
		return UserRow{}, err
	}

	return UserRow{
		ID:          row.ID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Status:      row.Status,
		Configured:  row.Configured,
		AuthEpoch:   row.AuthEpoch,
	}, nil
}

// Session 是数据库中可撤销的会话状态。
type Session struct {
	ID                uuid.UUID         // 资源标识。
	UserID            uuid.UUID         // 会话所属用户。
	TokenHash         string            // 会话令牌摘要，不包含原始凭据。
	AuthEpoch         int64             // 账号版本；变化后旧会话失效。
	State             enum.SessionState // 会话状态，取值见 enum.SessionState。
	AuthenticatedAt   time.Time         // 首次认证时间。
	ReauthenticatedAt *time.Time        // 最近一次密码确认时间；空表示未确认。
	AbsoluteExpiresAt time.Time         // 会话绝对到期时间。
	IdleExpiresAt     time.Time         // 空闲到期时间，不得晚于绝对到期。
}

// LockedSession 在用户锁之后取得会话行锁。
func LockedSession(ctx context.Context, db DB, id uuid.UUID) (Session, error) {
	row, err := queries(db).LockedSession(ctx, id)
	if err != nil {
		return Session{}, err
	}

	session := Session{
		ID:                row.ID,
		UserID:            row.UserID,
		TokenHash:         row.TokenHash,
		AuthEpoch:         row.AuthEpoch,
		State:             row.State,
		AuthenticatedAt:   row.AuthenticatedAt,
		AbsoluteExpiresAt: row.AbsoluteExpiresAt,
		IdleExpiresAt:     row.IdleExpiresAt,
	}

	if row.ReauthenticatedAt.Valid {
		session.ReauthenticatedAt = &row.ReauthenticatedAt.Time
	}

	return session, nil
}

// TouchSessionParams 是会话续期需要的当前版本和空闲时长。
type TouchSessionParams struct {
	ID          uuid.UUID     // 资源标识。
	TokenHash   string        // 会话令牌摘要，不包含原始凭据。
	AuthEpoch   int64         // 账号版本；变化后旧会话失效。
	IdleTimeout time.Duration // 允许空闲的时间间隔；由持久化层换算为微秒传入。
}

// TouchSession 使用取得锁后的数据库时间续期；已过期状态不能被重新激活。
// 没有任何一行被更新说明会话已过期、已撤销或令牌与版本不匹配，返回 sql.ErrNoRows。
func TouchSession(ctx context.Context, db DB, input TouchSessionParams) (Session, error) {
	row, err := queries(db).TouchSession(ctx, sqlcgen.TouchSessionParams{
		ID:         input.ID,
		TokenHash:  input.TokenHash,
		AuthEpoch:  input.AuthEpoch,
		IdleMicros: input.IdleTimeout.Microseconds(),
	})
	if err != nil {
		return Session{}, err
	}

	session := Session{
		ID:                row.ID,
		UserID:            row.UserID,
		TokenHash:         row.TokenHash,
		AuthEpoch:         row.AuthEpoch,
		State:             row.State,
		AuthenticatedAt:   row.AuthenticatedAt,
		AbsoluteExpiresAt: row.AbsoluteExpiresAt,
		IdleExpiresAt:     row.IdleExpiresAt,
	}

	if row.ReauthenticatedAt.Valid {
		session.ReauthenticatedAt = &row.ReauthenticatedAt.Time
	}

	return session, nil
}

// RevokeUserSessionsParams 指定需要撤销的用户和原因。
type RevokeUserSessionsParams struct {
	UserID       uuid.UUID // 会话所属用户。
	RevokeReason string    // 撤销原因，不包含凭据；空串表示不记录。
}

// RevokeUserSessions 在当前事务内撤销该用户的全部有效会话。
func RevokeUserSessions(ctx context.Context, db DB, input RevokeUserSessionsParams) error {
	return queries(db).RevokeUserSessions(ctx, sqlcgen.RevokeUserSessionsParams{
		UserID:       input.UserID,
		RevokeReason: input.RevokeReason,
	})
}

// InsertSessionParams 是一次新会话的全部期限与归属，期限由认证用例计算。
type InsertSessionParams struct {
	ID                uuid.UUID  // 会话标识。
	UserID            uuid.UUID  // 会话所属用户。
	TokenHash         string     // 会话令牌摘要。
	AuthEpoch         int64      // 账号版本。
	AuthenticatedAt   time.Time  // 首次认证时间。
	ReauthenticatedAt *time.Time // 最近一次密码确认时间。
	AbsoluteExpiresAt time.Time  // 绝对到期时间。
	IdleExpiresAt     time.Time  // 空闲到期时间。
	Now               time.Time  // 数据库时间。
}

// InsertSession 写入绑定令牌摘要的逻辑会话。
func InsertSession(ctx context.Context, db DB, session InsertSessionParams) error {
	params := sqlcgen.InsertSessionParams{
		ID:                session.ID,
		UserID:            session.UserID,
		TokenHash:         session.TokenHash,
		AuthEpoch:         session.AuthEpoch,
		AuthenticatedAt:   session.AuthenticatedAt,
		AbsoluteExpiresAt: session.AbsoluteExpiresAt,
		IdleExpiresAt:     session.IdleExpiresAt,
		LastSeenAt:        session.Now,
	}

	if session.ReauthenticatedAt != nil {
		params.ReauthenticatedAt = sql.NullTime{Time: *session.ReauthenticatedAt, Valid: true}
	}

	return queries(db).InsertSession(ctx, params)
}

// RevokeOwnSession 撤销指定用户的单个有效会话；没有命中说明会话已经失效或被转让。
func RevokeOwnSession(ctx context.Context, db DB, sessionID uuid.UUID, userID uuid.UUID, reason string) error {
	affected, err := queries(db).RevokeOwnSession(ctx, sqlcgen.RevokeOwnSessionParams{
		ID:           sessionID,
		UserID:       userID,
		RevokeReason: reason,
	})
	if err != nil {
		return err
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}
