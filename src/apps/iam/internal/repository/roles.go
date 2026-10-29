package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
)

// Role 保存角色及其当前权限；仅在模块内部用于委派校验。
type Role struct {
	ID          string   // 资源标识。
	Code        string   // 角色稳定代码。
	Name        string   // 角色显示名称。
	Description string   // 用途说明。
	Kind        string   // 内置或自定义角色类别。
	Status      string   // 当前启用状态。
	Version     int64    // 用于拒绝过期写入的版本号。
	MemberCount int64    // 当前关联成员数。
	Permissions []string `gorm:"-"` // 当前权限键集合。
}

// CreateRole 在调用方事务中创建自定义角色。
func CreateRole(ctx context.Context, tx *gorm.DB, id uuid.UUID, orgID, code, name, description, status, actorID string) error {
	return tx.WithContext(ctx).Table("iam.roles").Create(map[string]any{
		"id":          id,
		"org_id":      orgID,
		"code":        code,
		"name":        name,
		"description": description,
		"kind":        "custom",
		"status":      status,
		"created_by":  actorID,
	}).Error
}

// RoleGrants 锁定目标角色并读取权限，供相同事务内的版本及委派校验使用。
func RoleGrants(ctx context.Context, tx *gorm.DB, orgID, roleID string) (Role, error) {
	role := Role{Permissions: []string{}}
	err := tx.WithContext(ctx).Table("iam.roles AS r").
		Select("r.id,r.code,r.name,r.description,r.kind,r.status,r.version,(SELECT count(*) FROM iam.membership_roles mr WHERE mr.org_id=r.org_id AND mr.role_id=r.id) AS member_count").
		Clauses(clause.Locking{Strength: "UPDATE"}).Where("r.org_id = ? AND r.id = ?", orgID, roleID).Take(&role).Error
	if err != nil {
		return role, err
	}

	err = tx.WithContext(ctx).Table("iam.role_permissions").Where("org_id = ? AND role_id = ?", orgID, roleID).Order("permission_key").Pluck("permission_key", &role.Permissions).Error
	return role, err
}

// UpdateRole 更新角色信息和版本，零值描述仍会写入。
func UpdateRole(ctx context.Context, tx *gorm.DB, orgID, roleID, name, description, status string) error {
	return tx.WithContext(ctx).Table("iam.roles").Where("org_id = ? AND id = ?", orgID, roleID).
		Updates(map[string]any{
			"name":        name,
			"description": description,
			"status":      status,
			"version":     gorm.Expr("version+1"),
			"updated_at":  gorm.Expr("now()"),
		}).Error
}

// DeleteRole 删除角色及其授权，外部引用由约束与用例校验保护。
func DeleteRole(ctx context.Context, tx *gorm.DB, orgID, roleID string) error {
	if err := tx.WithContext(ctx).Table("iam.role_permissions").Where("org_id = ? AND role_id = ?", orgID, roleID).Delete(nil).Error; err != nil {
		return err
	}

	return tx.WithContext(ctx).Table("iam.roles").Where("org_id = ? AND id = ?", orgID, roleID).Delete(nil).Error
}

// ReplaceGrants 保留未变化的授权，避免重插已弃用权限触发数据库约束。
func ReplaceGrants(ctx context.Context, tx *gorm.DB, orgID, roleID string, permissions []string) error {
	query := tx.WithContext(ctx).Table("iam.role_permissions").Where("org_id = ? AND role_id = ?", orgID, roleID)
	if len(permissions) > 0 {
		query = query.Where("permission_key NOT IN ?", permissions)
	}

	if err := query.Delete(nil).Error; err != nil {
		return err
	}

	var existing []string
	if err := tx.WithContext(ctx).Table("iam.role_permissions").Where("org_id = ? AND role_id = ?", orgID, roleID).Pluck("permission_key", &existing).Error; err != nil {
		return err
	}
	previous := make(map[string]bool, len(existing))
	for _, key := range existing {
		previous[key] = true
	}
	grants := make([]rolePermission, 0, len(permissions))
	for _, key := range permissions {
		// 仅插入新增关系，历史已弃用授权仍可保留，不重新触发新增授权约束。
		if !previous[key] {
			grants = append(grants, rolePermission{OrgID: orgID, RoleID: roleID, PermissionKey: key})
		}
	}
	if len(grants) > 0 {
		if err := gorm.G[rolePermission](tx).Table("iam.role_permissions").CreateInBatches(ctx, &grants, 100); err != nil {
			return err
		}
	}

	return tx.WithContext(ctx).Table("iam.roles").Where("org_id = ? AND id = ?", orgID, roleID).
		Updates(map[string]any{"version": gorm.Expr("version+1"), "updated_at": gorm.Expr("now()")}).Error
}

// OrganizationVersion 锁定组织并读取当前权限版本。
func OrganizationVersion(ctx context.Context, tx *gorm.DB, orgID string) (int64, error) {
	var row struct{ AuthzVersion int64 }
	err := tx.WithContext(ctx).Table("iam.organizations").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orgID).Take(&row).Error
	return row.AuthzVersion, err
}

// MemberExists 判断目标是否属于当前组织。
func MemberExists(ctx context.Context, tx *gorm.DB, orgID, memberID string) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Table("iam.memberships").Where("org_id = ? AND id = ?", orgID, memberID).Count(&count).Error
	return count > 0, err
}

// MemberRoleIDs 读取目标成员的角色标识。
func MemberRoleIDs(ctx context.Context, tx *gorm.DB, orgID, memberID string) ([]string, error) {
	ids := []string{}
	err := tx.WithContext(ctx).Table("iam.membership_roles").Where("org_id = ? AND membership_id = ?", orgID, memberID).Pluck("role_id::text", &ids).Error
	return ids, err
}

// ReplaceMemberRoles 在已完成授权和版本检查的事务内替换角色关系。
func ReplaceMemberRoles(ctx context.Context, tx *gorm.DB, orgID, memberID string, ids []string, actorID string) error {
	if err := tx.WithContext(ctx).Table("iam.membership_roles").Where("org_id = ? AND membership_id = ?", orgID, memberID).Delete(nil).Error; err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}
	rows := make([]membershipRole, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, membershipRole{OrgID: orgID, MembershipID: memberID, RoleID: id, CreatedBy: actorID})
	}
	return gorm.G[membershipRole](tx).Table("iam.membership_roles").CreateInBatches(ctx, &rows, 100)

}

// ProtectLastAdministrators 保持平台和各有效组织至少一个有效管理员。
// 此查询须在管理锁内执行，防止两个并发撤销同时通过。
func ProtectLastAdministrators(ctx context.Context, tx *gorm.DB) error {
	var invalid bool
	err := tx.WithContext(ctx).Raw(`
 SELECT NOT EXISTS(SELECT 1 FROM iam.users WHERE is_super_admin AND status='active')
 OR EXISTS(SELECT 1 FROM iam.organizations o WHERE o.status='active' AND NOT EXISTS(
 SELECT 1 FROM iam.memberships m JOIN iam.users u ON u.id=m.user_id AND u.status='active'
 JOIN iam.membership_roles mr ON mr.org_id=m.org_id AND mr.membership_id=m.id
 JOIN iam.roles r ON r.org_id=mr.org_id AND r.id=mr.role_id AND r.kind='system' AND r.code='org_admin' AND r.status='active'
 WHERE m.org_id=o.id AND m.status='active'))`).Row().Scan(&invalid)
	if err != nil {
		return err
	}

	if invalid {
		return ErrConflict
	}

	return nil
}

// ListRoles 返回角色公开投影；数组通过 pgx 的 SQL Scanner 解码，不依赖驱动返回原生切片。
func ListRoles(ctx context.Context, db *gorm.DB, org uuid.UUID, target any, limit, offset int) ([]model.APIRole, int64, error) {
	filter := db.WithContext(ctx).Table("iam.roles").Where("org_id = ?", org)
	if target != nil {
		filter = filter.Where("id = ?", target)
	}

	var total int64
	if err := filter.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	rows, err := db.WithContext(ctx).Raw(`
 SELECT r.id::text,r.code,r.name,r.description,r.kind,r.status,r.version,
 ARRAY(SELECT permission_key FROM iam.role_permissions rp WHERE rp.org_id=r.org_id AND rp.role_id=r.id ORDER BY permission_key),
 (SELECT count(*) FROM iam.membership_roles mr WHERE mr.org_id=r.org_id AND mr.role_id=r.id)
 FROM iam.roles r WHERE r.org_id=$1 AND ($2::uuid IS NULL OR r.id=$2)
 ORDER BY r.kind DESC,r.created_at,r.id LIMIT $3 OFFSET $4`, org, target, limit, offset).Rows()
	if err != nil {
		return nil, 0, err
	}

	defer func() { _ = rows.Close() }()

	items := []model.APIRole{}
	for rows.Next() {
		var role model.APIRole
		role.Permissions = []string{}
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.Kind, &role.Status, &role.Version, pgtype.NewMap().SQLScanner(&role.Permissions), &role.MemberCount); err != nil {
			return nil, 0, err
		}

		items = append(items, role)
	}

	return items, total, rows.Err()
}

// rolePermission 是组织隔离的授权关联写入模型。
type rolePermission struct {
	OrgID         string
	RoleID        string
	PermissionKey string
}

// membershipRole 是已经通过组织与委派校验的成员角色关联。
type membershipRole struct {
	OrgID        string
	MembershipID string
	RoleID       string
	CreatedBy    string
}
