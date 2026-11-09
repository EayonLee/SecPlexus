package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
)

const ManagementLock int64 = 0x73707869616d

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrConflict        = errors.New("resource changed or is still in use")
	ErrInvalid         = errors.New("invalid input")
	ErrNotFound        = errors.New("resource not found")
	// ErrHasChildren 与 ErrHasUsers 区分组织删除的两种阻塞原因，
	// 否则二者都落到通用 conflict，用户不知道要先处理哪一项。
	ErrHasChildren = errors.New("organization still has child organizations")
	ErrHasUsers    = errors.New("organization still has users")
	// ErrRootOrganization 阻止删除全局唯一的根组织：删除后整棵树失去根，
	// organizations_single_root 不变量被破坏，祖先链查询随之失效。
	ErrRootOrganization = errors.New("root organization cannot be deleted")
	ErrUsernameTaken    = errors.New("username is already taken")
	ErrEmailTaken       = errors.New("email is already taken")
)

// Store 持有 IAM 使用的 PostgreSQL 连接池。
type Store struct {
	Pool  *pgxpool.Pool // SCS 与必要的原生查询共用此池，由创建者关闭。
	DB    *gorm.DB      // 业务查询和显式事务使用的 ORM，同样复用上述池。
	sqlDB *sql.DB
}

// NewStore 基于现有连接池创建 ORM；失败时不接管调用方连接池。
func NewStore(pool *pgxpool.Pool) (*Store, error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true, Logger: queryLogger{level: logger.Warn}})
	if err != nil {
		closeErr := sqlDB.Close()
		return nil, errors.Join(err, closeErr)
	}

	return &Store{
		Pool:  pool,
		DB:    db,
		sqlDB: sqlDB,
	}, nil
}

// Close 关闭 ORM 的 database/sql 适配；外部拥有的 pgx 池仍由创建者关闭。
func (s *Store) Close() error { return s.sqlDB.Close() }

// Audit 描述需要在事务中记录的审计事件。
type Audit struct {
	OrgID      *uuid.UUID     // 组织范围；为空时表示平台范围。
	ActorID    *uuid.UUID     // 操作者标识，系统事件可为空。
	ActorName  string         // 审计时的操作者名称快照。
	ActorType  string         // 用户、系统或部署操作员。
	Action     string         // 稳定的动作名称。
	TargetType string         // 被操作资源类型。
	TargetID   *uuid.UUID     // 被操作资源标识。
	Outcome    string         // 成功、拒绝或失败。
	Reason     string         // 不包含敏感值的原因代码。
	RequestID  string         // 关联本次请求的标识。
	DecisionID *uuid.UUID     // 授权决定标识，可为空。
	Metadata   map[string]any // 经过筛选的审计扩展字段，编码后不超过 8192 字节。
}

// AppendAudit 在调用方事务中写入审计事件。
func AppendAudit(ctx context.Context, tx *gorm.DB, a Audit) error {
	if a.ActorType == "" {
		a.ActorType = "user"
	}

	if a.Outcome == "" {
		a.Outcome = "success"
	}

	metadata := a.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	data, err := json.Marshal(metadata)
	if err != nil || len(data) > 8192 {
		return ErrInvalid
	}

	err = tx.WithContext(ctx).Exec(
		`INSERT INTO iam.audit_events(id,org_id,actor_type,actor_id,actor_name,action,target_type,target_id,outcome,reason_code,request_id,decision_id,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		uuid.New(),
		a.OrgID,
		a.ActorType,
		a.ActorID,
		a.ActorName,
		a.Action,
		a.TargetType,
		a.TargetID,
		a.Outcome,
		a.Reason,
		a.RequestID,
		a.DecisionID,
		data,
	).Error
	return err
}

// Seed 在显式初始化时同步权限和创建首个管理员；锁与事务保证幂等。
// 已有账号不会被提权或重新分配密码。
func (s *Store) Seed(ctx context.Context, username, password string) error {
	tx := s.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err = tx.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock($1)`, ManagementLock).Error; err != nil {
		return err
	}

	for _, p := range authorization.Catalog() {
		var compatible bool
		if err = tx.WithContext(ctx).Raw(
			`SELECT NOT EXISTS(SELECT 1 FROM iam.permissions WHERE permission_key=$1 AND (module<>$2 OR resource_type<>$3 OR action<>$4 OR scope<>$5))`,
			p.Key,
			p.Module,
			p.ResourceType,
			p.Action,
			p.Scope,
		).Row().Scan(&compatible); err != nil {
			return err
		}

		if !compatible {
			return errors.New("permission meaning changed without a reviewed migration")
		}

		err = tx.WithContext(ctx).Exec(
			`INSERT INTO iam.permissions(permission_key,module,resource_type,action,description,scope,risk_level,delegable,requires_recent_auth,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(permission_key) DO UPDATE SET description=EXCLUDED.description,risk_level=EXCLUDED.risk_level,delegable=EXCLUDED.delegable,requires_recent_auth=EXCLUDED.requires_recent_auth,status=EXCLUDED.status,updated_at=now()`,
			p.Key,
			p.Module,
			p.ResourceType,
			p.Action,
			p.Description,
			p.Scope,
			p.RiskLevel,
			p.Delegable,
			p.RequiresRecentAuth,
			p.Status,
		).Error
		if err != nil {
			return err
		}
	}

	userID, err := syncAdministrator(ctx, tx, username, password)
	if err != nil {
		return err
	}

	// 根组织由 00010 迁移创建（slug=master）。这里只保证它有管理员关系，
	// 不重复建组织，也不依赖 users 是否为空：组织重建后既有账号仍需可登录。
	var rootID string
	if err = tx.WithContext(ctx).Raw(`SELECT id::text FROM iam.organizations WHERE parent_id IS NULL`).Row().Scan(&rootID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("root organization is missing; run the organization tree migration before seeding")
		}

		return err
	}

	orgID, err := uuid.Parse(rootID)
	if err != nil {
		return err
	}

	var attached bool
	if err = tx.WithContext(ctx).Raw(
		`SELECT EXISTS(SELECT 1 FROM iam.organization_users WHERE org_id = $1 AND user_id = $2)`, orgID, userID,
	).Row().Scan(&attached); err != nil {
		return err
	}

	if !attached {
		if err = AttachRootMembership(ctx, tx, orgID, userID); err != nil {
			return err
		}

		if err = AppendAudit(ctx, tx, Audit{ActorType: "deployment_operator", Action: "iam.initialize", TargetType: "organization", TargetID: &orgID, RequestID: uuid.NewString()}); err != nil {
			return err
		}
	}

	return tx.Commit().Error
}

// CreateOrganization 在事务中创建组织，并把指定的初始用户挂到该组织。
// parentID 为空表示创建根组织；深度由 organization_tree_guard 触发器计算。
func CreateOrganization(ctx context.Context, tx *gorm.DB, id uuid.UUID, slug, name string, parentID *uuid.UUID, organizationUserID, actorID uuid.UUID) error {
	if err := tx.WithContext(ctx).Exec(`INSERT INTO iam.organizations(id,slug,name,parent_id) VALUES($1,$2,$3,$4)`, id, slug, name, parentID).Error; err != nil {
		return err
	}

	return AttachRootMembership(ctx, tx, id, organizationUserID)
}

// AttachRootMembership 把指定用户作为有效用户挂到组织下。
//
// 组织不再自带内置角色：角色完全由使用者按需创建并授予权限，
// 因此这里只建用户关系，不写任何角色或授权。
func AttachRootMembership(ctx context.Context, tx *gorm.DB, orgID, userID uuid.UUID) error {
	return tx.WithContext(ctx).Exec(
		`INSERT INTO iam.organization_users(id,org_id,user_id,status) VALUES($1,$2,$3,'active') ON CONFLICT (org_id,user_id) DO NOTHING`,
		uuid.New(), orgID, userID,
	).Error
}

// VerifySchema 校验迁移版本及初始超级管理员，不在服务启动时自动改表。
func (s *Store) VerifySchema(ctx context.Context) error {
	var err error
	current, target, pending, err := schemaState(ctx, s.sqlDB)
	if err != nil || pending || current != target {
		return errors.New("IAM schema version is incompatible; run the matching migration before serving")
	}

	var ready bool
	if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM iam.users WHERE is_super_admin AND status='active')`).Scan(&ready); err != nil {
		return err
	}

	if !ready {
		return errors.New("IAM is not initialized")
	}

	return nil
}
