package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// Queries 将数据操作绑定到调用方的连接或事务；方法不会切回根连接。
type Queries struct{ db *gorm.DB }

// NewQueries 创建当前连接或事务的查询入口。
func NewQueries(db *gorm.DB) *Queries { return &Queries{db: db} }

// LoginUser 是认证所需的账户快照，密码哈希仅用于内部校验。
type LoginUser struct {
	ID             uuid.UUID          // 资源标识。
	Username       string             // 登录账号。
	DisplayName    string             // 界面显示名称。
	Status         string             // 当前启用状态。
	IsSuperAdmin   bool               // 是否具备平台管理员身份。
	AuthVersion    int64              // 身份版本；变化后旧会话失效。
	PasswordHash   string             // 仅供内部校验的密码哈希，禁止写入响应和日志。
	FailedAttempts int32              // 连续失败次数。
	LockedUntil    pgtype.Timestamptz // 账户锁定截止时间。
}

// GetLoginUser 读取账号和密码状态，不对外序列化凭据。
func (q *Queries) GetLoginUser(ctx context.Context, username string) (LoginUser, error) {
	var user LoginUser
	err := q.db.WithContext(ctx).Table("iam.users AS u").
		Select("u.id,u.username,u.display_name,u.status,u.is_super_admin,u.auth_version,c.password_hash,c.failed_attempts,c.locked_until").
		Joins("JOIN iam.password_credentials c ON c.user_id=u.id").
		Where("u.username_normalized = ?", username).Take(&user).Error
	return user, err
}

// LockUser 锁定用户行，所有身份写入均先取得用户锁。
func (q *Queries) LockUser(ctx context.Context, id uuid.UUID) (LoginUser, error) {
	var user LoginUser
	err := q.db.WithContext(ctx).Table("iam.users").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).Take(&user).Error
	return user, err
}

// Session 是数据库中可撤销的会话状态。
type Session struct {
	ID                uuid.UUID          // 资源标识。
	UserID            uuid.UUID          // 会话所属用户。
	TokenHash         string             // 会话令牌摘要，不包含原始凭据。
	AuthVersion       int64              // 身份版本；变化后旧会话失效。
	State             string             // 会话状态，active 或 revoked。
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
	ID          uuid.UUID       // 资源标识。
	TokenHash   string          // 会话令牌摘要，不包含原始凭据。
	AuthVersion int64           // 身份版本；变化后旧会话失效。
	IdleTimeout pgtype.Interval // 允许空闲的时间间隔。
}

// TouchSession 使用取得锁后的数据库时间续期；已过期状态不能被重新激活。
func (q *Queries) TouchSession(ctx context.Context, input TouchSessionParams) (Session, error) {
	var session Session
	response := q.db.WithContext(ctx).Table("iam.user_sessions").Model(&session).
		Clauses(clause.Returning{}).
		Where("id = ? AND token_hash = ? AND state = 'active' AND auth_version = ? AND idle_expires_at > clock_timestamp() AND absolute_expires_at > clock_timestamp()", input.ID, input.TokenHash, input.AuthVersion).
		Updates(map[string]any{
			"idle_expires_at": gorm.Expr("LEAST(absolute_expires_at,GREATEST(idle_expires_at,clock_timestamp()+?::interval))", input.IdleTimeout),
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
	UserID       uuid.UUID   // 会话所属用户。
	RevokeReason pgtype.Text // 撤销原因，不包含凭据。
}

// RevokeUserSessions 在当前事务内撤销该用户的全部有效会话。
func (q *Queries) RevokeUserSessions(ctx context.Context, input RevokeUserSessionsParams) error {
	return q.db.WithContext(ctx).Table("iam.user_sessions").Where("user_id = ? AND state = 'active'", input.UserID).
		Updates(map[string]any{
			"state":         "revoked",
			"revoked_at":    gorm.Expr("clock_timestamp()"),
			"revoke_reason": input.RevokeReason,
		}).Error
}

// GetOrganization 读取组织状态与权限版本。
func (q *Queries) GetOrganization(ctx context.Context, id uuid.UUID) (contract.APIOrganization, error) {
	var organization contract.APIOrganization
	err := q.db.WithContext(ctx).Table("iam.organizations").Select("id,slug,name,status,authz_version").Where("id = ?", id).Take(&organization).Error
	return organization, err
}

// GetOrganizationPermissionsParams 标识组织和用户范围。
type GetOrganizationPermissionsParams struct {
	OrgID  uuid.UUID // 组织范围；为空时表示平台范围。
	UserID uuid.UUID // 会话所属用户。
}

// GetOrganizationPermissions 查询有效成员通过有效角色获得的权限。
func (q *Queries) GetOrganizationPermissions(ctx context.Context, input GetOrganizationPermissionsParams) ([]string, error) {
	items := []string{}
	err := q.db.WithContext(ctx).Table("iam.memberships AS m").Distinct("p.key").
		Joins("JOIN iam.membership_roles mr ON mr.org_id=m.org_id AND mr.membership_id=m.id").
		Joins("JOIN iam.roles r ON r.org_id=mr.org_id AND r.id=mr.role_id AND r.status='active'").
		Joins("JOIN iam.role_permissions rp ON rp.org_id=r.org_id AND rp.role_id=r.id").
		Joins("JOIN iam.permissions p ON p.key=rp.permission_key AND p.status IN ('active','deprecated')").
		Where("m.org_id = ? AND m.user_id = ? AND m.status='active'", input.OrgID, input.UserID).Pluck("p.key", &items).Error
	return items, err
}
