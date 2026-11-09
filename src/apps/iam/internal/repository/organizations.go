package repository

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
)

// LockOrganizationVersion 锁定组织并读取权限版本。
func LockOrganizationVersion(ctx context.Context, db *gorm.DB, orgID string, version *int64) error {
	return db.WithContext(ctx).Raw(`SELECT authz_epoch FROM iam.organizations WHERE id=$1 FOR UPDATE`, orgID).Row().Scan(version)
}

// LockParentCandidate 锁定并确认候选父组织存在，返回其标识。
// 父组织不存在时返回未找到错误；存在性校验同时阻止挂到不存在的节点下。
func LockParentCandidate(ctx context.Context, db *gorm.DB, parentID string) error {
	var id uuid.UUID
	return db.WithContext(ctx).Raw(`SELECT id FROM iam.organizations WHERE id=$1 FOR UPDATE`, parentID).Row().Scan(&id)
}

// UpdateOrganizationParent 改挂父组织；depth 由 organization_tree_guard 触发器重算。
func UpdateOrganizationParent(ctx context.Context, db *gorm.DB, orgID, parentID string) error {
	return db.WithContext(ctx).Table("iam.organizations").
		Where("id = ?", orgID).
		Updates(map[string]any{"parent_id": parentID, "updated_at": gorm.Expr("now()")}).Error
}

// RecomputeSubtreeDepth 重算以 orgID 为根的整棵子树的 depth。
//
// 递归用 UNION 而非 UNION ALL：UNION 去重后在成环数据上仍会终止，
// UNION ALL 会无限递归（已实测超时）。
// 每一行经 organization_tree_guard 只做 NEW.depth = parent.depth + 1，
// 与递归算出的值一致，收敛且幂等。
func RecomputeSubtreeDepth(ctx context.Context, db *gorm.DB, orgID string) error {
	return db.WithContext(ctx).Exec(`
WITH RECURSIVE subtree AS (
  SELECT id, depth FROM iam.organizations WHERE id = $1
  UNION
  SELECT o.id, s.depth + 1 FROM iam.organizations o JOIN subtree s ON o.parent_id = s.id
)
UPDATE iam.organizations t SET depth = s.depth, updated_at = now()
FROM subtree s WHERE t.id = s.id AND t.id <> $1`, orgID).Error
}

// UpdateOrganization 更新组织名称并递增权限版本。
// slug 由数据库触发器保护，不可修改。
func UpdateOrganization(ctx context.Context, db *gorm.DB, orgID string, name string) error {
	return db.WithContext(ctx).Table("iam.organizations").Where("id = ?", orgID).
		Updates(map[string]any{
			"name":        name,
			"authz_epoch": gorm.Expr("authz_epoch+1"),
			"updated_at":  gorm.Expr("now()"),
		}).Error
}

// OrganizationTree 返回全部组织的树形投影（扁平列表，由调用方组装）。
//
// 返回截断标志：组织数量超过上限时明确告知调用方，不静默返回残缺的树。
// 祖先链递归用 UNION 而非 UNION ALL：脏数据成环时 UNION ALL 会无限递归。
func OrganizationTree(ctx context.Context, db *gorm.DB) ([]model.APIOrganization, bool, error) {
	type row struct {
		AuthzEpoch string
		Depth      int
		ID         string
		Name       string
		ParentID   *string
		PathText   string
		Slug       string
	}

	rows := []row{}
	// path 用 ' / ' 拼接成单列字符串，避免驱动扫描 text[] 的类型问题。
	// 名称内的 ' / ' 按配置约定不会出现；出现时只是显示多一段分隔符，不影响 id。
	err := db.WithContext(ctx).Raw(`
SELECT o.id::text, o.slug, o.name, o.parent_id::text, o.depth::int, o.authz_epoch::text,
       (
         WITH RECURSIVE chain AS (
           SELECT id, name, parent_id, depth FROM iam.organizations WHERE id = o.id
           UNION
           SELECT p.id, p.name, p.parent_id, p.depth
           FROM iam.organizations p JOIN chain c ON p.id = c.parent_id
         )
         SELECT string_agg(name, ' / ' ORDER BY depth) FROM chain
       ) AS path_text
FROM iam.organizations o
ORDER BY o.depth, o.created_at, o.id
LIMIT 1001`).Scan(&rows).Error
	if err != nil {
		return nil, false, err
	}

	truncated := len(rows) > 1000
	if truncated {
		rows = rows[:1000]
	}

	items := make([]model.APIOrganization, 0, len(rows))
	for _, entry := range rows {
		epoch, err := strconv.ParseInt(entry.AuthzEpoch, 10, 64)
		if err != nil {
			return nil, false, err
		}

		items = append(items, model.APIOrganization{
			AuthzEpoch: epoch,
			Depth:      entry.Depth,
			ID:         entry.ID,
			Name:       entry.Name,
			ParentID:   entry.ParentID,
			Path:       strings.Split(entry.PathText, " / "),
			Slug:       entry.Slug,
		})
	}

	return items, truncated, nil
}

// IsRootOrganization 判断目标组织是否为根（parent_id 为空）。
func IsRootOrganization(ctx context.Context, db *gorm.DB, orgID string) (bool, error) {
	var root bool
	err := db.WithContext(ctx).Raw(`SELECT parent_id IS NULL FROM iam.organizations WHERE id = $1`, orgID).Row().Scan(&root)
	return root, err
}

// OrganizationChildCount 统计直接子组织数，用于删除前置检查。
func OrganizationChildCount(ctx context.Context, db *gorm.DB, orgID string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Raw(`SELECT count(*) FROM iam.organizations WHERE parent_id = $1`, orgID).Row().Scan(&count)
	return count, err
}

// OrganizationUserCount 统计组织用户数，用于删除前置检查。
func OrganizationUserCount(ctx context.Context, db *gorm.DB, orgID string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Raw(`SELECT count(*) FROM iam.organization_users WHERE org_id = $1`, orgID).Row().Scan(&count)
	return count, err
}

// DeleteOrganization 按依赖顺序清除组织及其用户关系与角色。
// 全库外键均为 NO ACTION，必须显式按序删除；audit_events 无组织外键，审计留存。
func DeleteOrganization(ctx context.Context, db *gorm.DB, orgID string) error {
	for _, statement := range []string{
		`DELETE FROM iam.organization_user_roles WHERE org_id = $1`,
		`DELETE FROM iam.role_permissions WHERE org_id = $1`,
		`DELETE FROM iam.roles WHERE org_id = $1`,
		`DELETE FROM iam.organization_users WHERE org_id = $1`,
		`DELETE FROM iam.organizations WHERE id = $1`,
	} {
		if err := db.WithContext(ctx).Exec(statement, orgID).Error; err != nil {
			return err
		}
	}

	return nil
}
