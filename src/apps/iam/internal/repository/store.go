package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

const ManagementLock int64 = 0x73707869616d

var (
	ErrUnauthenticated = errors.New("authentication required")
	// ErrConflict 表示通用并发冲突；以下错误细分了冲突原因，便于前端给出可操作提示。
	ErrConflict                = errors.New("resource changed or is still in use")
	ErrInvalid                 = errors.New("invalid input")
	ErrNotFound                = errors.New("resource not found")
	ErrRoleInUse               = errors.New("role is still assigned to users")
	ErrRoleNameTaken           = errors.New("role name is already taken")
	ErrBuiltinRoleImmutable    = errors.New("builtin role cannot be modified or deleted")
	ErrConfiguredUserImmutable = errors.New("configured administrator cannot be deleted, disabled or re-passworded")
	// ErrLastSuperAdmin 守住平台底线：任何变更都必须留下至少一名启用中的超级管理员。
	ErrLastSuperAdmin = errors.New("at least one active super administrator is required")
	// ErrSelfTarget 拒绝对当前登录账号的自伤操作，避免会话进行中被自己删除或停用。
	ErrSelfTarget    = errors.New("operation is not allowed on the signed-in account")
	ErrUsernameTaken = errors.New("username is already taken")
	ErrEmailTaken    = errors.New("email is already taken")
)

// Store 持有 IAM 使用的 PostgreSQL 连接池。
type Store struct {
	Pool *pgxpool.Pool // SCS 与迁移检查共用的原生池，由创建者关闭。
	DB   *sql.DB       // 业务查询与事务使用的 database/sql 适配，复用同一池。
}

// NewStore 基于现有连接池创建 database/sql 适配；失败时不接管调用方连接池。
func NewStore(pool *pgxpool.Pool) (*Store, error) {
	return &Store{Pool: pool, DB: stdlib.OpenDBFromPool(pool)}, nil
}

// Close 关闭 database/sql 适配；外部拥有的 pgx 池仍由创建者关闭。
func (s *Store) Close() error { return s.DB.Close() }

// Transaction 在单一事务内执行 change，提交前的任何失败都会回滚。
// 事务句柄按 database/sql 的语义下传，仓库函数与生成查询共用同一条连接。
func (s *Store) Transaction(ctx context.Context, change func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// 已经提交后再回滚是空操作，因此这里可以无条件登记。
	defer func() { _ = tx.Rollback() }()

	if err = change(tx); err != nil {
		return err
	}

	return tx.Commit()
}

// Audit 描述需要在事务中记录的审计事件。
// 审计没有组织维度，目标由 target_type/target_id 表达。
type Audit struct {
	ActorID    *uuid.UUID        // 操作者标识，系统事件可为空。
	ActorName  string            // 审计时的操作者名称快照。
	ActorType  enum.ActorType    // 用户、系统或部署操作员，取值见 enum.ActorType。
	Action     string            // 稳定的动作名称。
	TargetType string            // 被操作资源类型。
	TargetID   *uuid.UUID        // 被操作资源标识。
	Outcome    enum.AuditOutcome // 成功、拒绝或失败，取值见 enum.AuditOutcome。
	Reason     string            // 不包含敏感值的原因代码。
	RequestID  string            // 关联本次请求的标识。
	DecisionID *uuid.UUID        // 授权决定标识，可为空。
	Metadata   map[string]any    // 经过筛选的审计扩展字段，编码后不超过 8192 字节。
}

// AppendAudit 在调用方给出的连接或事务中写入审计事件。
// 需要与业务变更同生共死时必须传事务句柄。
func AppendAudit(ctx context.Context, tx DB, a Audit) error {
	if a.ActorType == "" {
		a.ActorType = enum.ActorTypeUser
	}

	if a.Outcome == "" {
		a.Outcome = enum.AuditOutcomeSuccess
	}

	metadata := a.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	data, err := json.Marshal(metadata)
	if err != nil || len(data) > 8192 {
		return ErrInvalid
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO iam.audit_events(id,actor_type,actor_id,actor_name,action,target_type,target_id,outcome,reason_code,request_id,decision_id,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		uuid.New(),
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
	)

	return err
}

// Seed 在显式初始化或启动时幂等写入内置数据：权限目录、内置角色与配置管理员。
// 事务与 advisory 锁保证并发启动收敛到同一状态；已存在的账号不会被重复提权。
func (s *Store) Seed(ctx context.Context, username, password string) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		if err := LockManagement(ctx, tx, ManagementLock); err != nil {
			return err
		}

		if err := syncCatalog(ctx, tx); err != nil {
			return err
		}

		roleID, err := syncBuiltinRole(ctx, tx)
		if err != nil {
			return err
		}

		_, err = syncAdministrator(ctx, tx, username, password, roleID)

		return err
	})
}

// syncCatalog 以代码中的权限目录为准逐键 upsert：缺失插入、元数据更新、移除的键标记 retired。
func syncCatalog(ctx context.Context, tx *sql.Tx) error {
	for _, p := range authorization.Catalog() {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO iam.permissions(permission_key,description,risk_level,delegable,requires_recent_auth,status) VALUES($1,$2,$3,$4,$5,'active') ON CONFLICT(permission_key) DO UPDATE SET description=EXCLUDED.description,risk_level=EXCLUDED.risk_level,delegable=EXCLUDED.delegable,requires_recent_auth=EXCLUDED.requires_recent_auth,status='active',updated_at=now()`,
			p.Key,
			p.Description,
			p.RiskLevel,
			p.Delegable,
			p.RequiresRecentAuth,
		)
		if err != nil {
			return err
		}
	}

	// 目录里已经删除的键保留历史授权，但标记为 retired 并立即失效。
	keys := make([]string, 0, len(authorization.Catalog()))
	for _, p := range authorization.Catalog() {
		keys = append(keys, p.Key)
	}

	_, err := tx.ExecContext(
		ctx,
		`UPDATE iam.permissions SET status='retired',updated_at=now() WHERE status<>'retired' AND NOT (permission_key = ANY($1::text[]))`,
		keys,
	)

	return err
}

// syncBuiltinRole 保证内置角色存在并持有目录中全部 active 权限。
// 权限只增不删：目录里新增的键在下次启动时补齐，移除的键保留原授权。
func syncBuiltinRole(ctx context.Context, tx *sql.Tx) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRowContext(ctx, `SELECT id FROM iam.roles WHERE builtin`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id = uuid.New()
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO iam.roles(id,name,description,builtin) VALUES($1,$2,'',true)`,
			id, authorization.BuiltinRoleName,
		); err != nil {
			return uuid.Nil, err
		}
	} else if err != nil {
		return uuid.Nil, err
	} else if _, err = tx.ExecContext(
		ctx,
		`UPDATE iam.roles SET name=$2,description='',updated_at=now() WHERE id=$1`,
		id, authorization.BuiltinRoleName,
	); err != nil {
		return uuid.Nil, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO iam.role_permissions(role_id,permission_key) SELECT $1,permission_key FROM iam.permissions WHERE status='active' ON CONFLICT DO NOTHING`,
		id,
	)

	return id, err
}

// VerifySchema 校验迁移版本兼容，不在服务启动时自动改表。
// 内置数据由 Seed 在启动时幂等补齐，因此这里不再把数据存在性当作 schema 状态。
func (s *Store) VerifySchema(ctx context.Context) error {
	current, target, pending, err := schemaState(ctx, s.DB)
	if err != nil || pending || current != target {
		return errors.New("IAM schema version is incompatible; run the matching migration before serving")
	}

	return nil
}
