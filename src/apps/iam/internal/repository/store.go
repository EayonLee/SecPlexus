package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

const ManagementLock int64 = 0x73707869616d

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrConflict        = errors.New("resource changed or is still in use")
	ErrInvalid         = errors.New("invalid input")
	ErrNotFound        = errors.New("resource not found")
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

	for _, p := range contract.Catalog() {
		var compatible bool
		if err = tx.WithContext(ctx).Raw(
			`SELECT NOT EXISTS(SELECT 1 FROM iam.permissions WHERE key=$1 AND (module<>$2 OR resource_type<>$3 OR action<>$4 OR scope<>$5))`,
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
			`INSERT INTO iam.permissions(key,module,resource_type,action,description,scope,risk_level,delegable,requires_recent_auth,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description,risk_level=EXCLUDED.risk_level,delegable=EXCLUDED.delegable,requires_recent_auth=EXCLUDED.requires_recent_auth,status=EXCLUDED.status,updated_at=now()`,
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

	var count, admins int
	if err = tx.WithContext(ctx).Raw(`SELECT count(*),count(*) FILTER(WHERE is_super_admin AND status='active') FROM iam.users`).Row().Scan(&count, &admins); err != nil {
		return err
	}

	if count == 0 {
		username = strings.TrimSpace(username)
		normalized, err := identity.NormalizeUsername(username)
		if err != nil {
			return err
		}

		hash, err := (identity.PasswordHasher{}).GenerateHash(password)
		if err != nil {
			return err
		}

		userID, orgID := uuid.New(), uuid.New()
		if err = tx.WithContext(ctx).Exec(`INSERT INTO iam.users(id,username,username_normalized,display_name,status,is_super_admin) VALUES($1,$2,$3,$2,'active',true)`, userID, username, normalized).Error; err != nil {
			return err
		}

		if err = tx.WithContext(ctx).Exec(`INSERT INTO iam.password_credentials(user_id,password_hash) VALUES($1,$2)`, userID, hash).Error; err != nil {
			return err
		}

		if err = CreateOrganization(ctx, tx, orgID, "default", "默认组织", userID, userID); err != nil {
			return err
		}

		if err = AppendAudit(ctx, tx, Audit{
			ActorType:  "deployment_operator",
			Action:     "iam.initialize",
			TargetType: "organization",
			TargetID:   &orgID,
			RequestID:  uuid.NewString(),
		}); err != nil {
			return err
		}
	} else if admins == 0 {
		return errors.New("accounts exist but no active super administrator; operator recovery is required")
	}
	// 只同步内置角色授权；自定义角色属于用户数据，不能被初始化覆盖。
	if err = tx.WithContext(ctx).Exec(`INSERT INTO iam.role_permissions(org_id,role_id,permission_key) SELECT r.org_id,r.id,p.key FROM iam.roles r CROSS JOIN iam.permissions p WHERE r.kind='system' AND r.code='org_admin' AND p.scope='organization' AND p.status='active' ON CONFLICT DO NOTHING`).Error; err != nil {
		return err
	}

	return tx.Commit().Error
}

// CreateOrganization 在事务中创建组织及其管理员关系。
func CreateOrganization(ctx context.Context, tx *gorm.DB, id uuid.UUID, slug, name string, adminID, actorID uuid.UUID) error {
	if err := tx.WithContext(ctx).Exec(`INSERT INTO iam.organizations(id,slug,name,status) VALUES($1,$2,$3,'active')`, id, slug, name).Error; err != nil {
		return err
	}

	adminRole, memberRole, membership := uuid.New(), uuid.New(), uuid.New()
	if err := tx.WithContext(ctx).Exec(`INSERT INTO iam.roles(id,org_id,code,name,kind,status,created_by) VALUES($1,$3,'org_admin','组织管理员','system','active',$4),($2,$3,'member','普通成员','system','active',$4)`, adminRole, memberRole, id, actorID).Error; err != nil {
		return err
	}

	if err := tx.WithContext(ctx).Exec(`INSERT INTO iam.role_permissions(org_id,role_id,permission_key) SELECT $1,$2,key FROM iam.permissions WHERE scope='organization' AND status='active'`, id, adminRole).Error; err != nil {
		return err
	}

	if err := tx.WithContext(ctx).Exec(`INSERT INTO iam.memberships(id,org_id,user_id,status) VALUES($1,$2,$3,'active')`, membership, id, adminID).Error; err != nil {
		return err
	}

	err := tx.WithContext(ctx).Exec(`INSERT INTO iam.membership_roles(org_id,membership_id,role_id,created_by) VALUES($1,$2,$3,$4),($1,$2,$5,$4)`, id, membership, adminRole, actorID, memberRole).Error
	return err
}

// VerifySchema 校验迁移版本及初始管理员，不在服务启动时自动改表。
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
