package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
)

// LockManagement 取得低频管理写入的事务锁，避免管理员和授权竞态。
func LockManagement(ctx context.Context, db *gorm.DB, managementLock int64) error {
	return db.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock($1)`, managementLock).Error
}

// EffectiveRoleGrants 读取用户通过有效角色持有的权限。
func EffectiveRoleGrants(ctx context.Context, db *gorm.DB, orgID string, userID string) (*sql.Rows, error) {
	return db.WithContext(ctx).Raw(`
		SELECT DISTINCT rp.permission_key
		FROM iam.organization_users m
		JOIN iam.organization_user_roles mr ON mr.org_id = m.org_id AND mr.organization_user_id = m.id
		JOIN iam.roles ro ON ro.org_id = mr.org_id AND ro.id = mr.role_id
		JOIN iam.role_permissions rp ON rp.org_id = ro.org_id AND rp.role_id = ro.id
		WHERE m.org_id = $1 AND m.user_id = $2 AND m.status = 'active'`, orgID, userID).Rows()
}

// OrganizationUserPreviewState 读取预览所需的用户、组织版本和用户有效状态。
func OrganizationUserPreviewState(ctx context.Context, db *gorm.DB, orgID string, organizationUserID string, userID *uuid.UUID, version *int64, organizationUserActive *bool, systemAdmin *bool) error {
	return db.WithContext(ctx).Raw(`SELECT m.user_id,o.authz_epoch,m.status='active' AND u.status='active',u.is_super_admin FROM iam.organization_users m JOIN iam.organizations o ON o.id=m.org_id JOIN iam.users u ON u.id=m.user_id WHERE m.org_id=$1 AND m.id=$2`, orgID, organizationUserID).Row().Scan(userID, version, organizationUserActive, systemAdmin)
}

// ReadPermission 读取权限定义和近期认证约束。
func ReadPermission(ctx context.Context, db *gorm.DB, action string) (authorization.Permission, error) {
	var permission authorization.Permission
	err := db.WithContext(ctx).Table("iam.permissions").Where("permission_key = ?", action).Take(&permission).Error
	return permission, err
}

// LockOrganization 在管理事务内锁定组织。
func LockOrganization(ctx context.Context, db *gorm.DB, orgID string, lockedID *uuid.UUID) error {
	return db.WithContext(ctx).Raw(`SELECT id FROM iam.organizations WHERE id=$1 FOR UPDATE`, orgID).Row().Scan(lockedID)
}

// BumpOrganizationAuthzEpoch 递增组织权限版本，使其他请求及时重算权限。
func BumpOrganizationAuthzEpoch(ctx context.Context, db *gorm.DB, parsed uuid.UUID) error {
	return db.WithContext(ctx).Exec(`UPDATE iam.organizations SET authz_epoch=authz_epoch+1,updated_at=now() WHERE id=$1`, parsed).Error
}

// OrgAuthSnapshot 一次读取授权判定所需的组织状态、用户有效性和有效权限集。
// 合并查询的理由：授权判定在每个受保护请求上都会执行，原来分三次往返
// （读组织、读用户、五表 join 读权限），同一事务内三次 RTT 是纯开销。
//
// 权限沿祖先链向下继承：在任一祖先组织被授予的权限，对其全部后代同样生效。
// 反向不成立——子组织的授权不会上溯给祖先，兄弟组织之间互不影响。
// 递归用 UNION 而非 UNION ALL：UNION 去重后在成环数据上仍会终止。
func OrgAuthSnapshot(ctx context.Context, db *gorm.DB, org uuid.UUID, userID uuid.UUID, orgActive *bool, organizationUserActive *bool, permissions *[]string) error {
	return db.WithContext(ctx).Raw(`
		WITH RECURSIVE ancestry AS (
			SELECT o.id, o.parent_id FROM iam.organizations o WHERE o.id = $1
			UNION
			SELECT p.id, p.parent_id FROM iam.organizations p
			JOIN ancestry a ON p.id = a.parent_id
		)
		SELECT
			-- 组织无停用态：只要行存在即视为有效。
			TRUE,
			EXISTS(
				SELECT 1 FROM iam.organization_users m
				JOIN ancestry a ON a.id = m.org_id
				WHERE m.user_id = $2 AND m.status = 'active'
			),
			COALESCE(ARRAY(
				SELECT DISTINCT p.permission_key
				FROM iam.organization_users m
				JOIN ancestry a ON a.id = m.org_id
				JOIN iam.organization_user_roles mr ON mr.org_id = m.org_id AND mr.organization_user_id = m.id
				JOIN iam.roles r ON r.org_id = mr.org_id AND r.id = mr.role_id
				JOIN iam.role_permissions rp ON rp.org_id = r.org_id AND rp.role_id = r.id
				JOIN iam.permissions p ON p.permission_key = rp.permission_key AND p.status IN ('active','deprecated')
				WHERE m.user_id = $2 AND m.status = 'active'
			), '{}')
		FROM iam.organizations o
		WHERE o.id = $1`, org, userID).Row().Scan(orgActive, organizationUserActive, permissions)
}
