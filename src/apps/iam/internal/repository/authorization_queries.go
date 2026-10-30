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

// EffectiveRoleGrants 读取成员通过有效角色持有的权限。
func EffectiveRoleGrants(ctx context.Context, db *gorm.DB, orgID string, userID string) (*sql.Rows, error) {
	return db.WithContext(ctx).Raw(`
		SELECT DISTINCT rp.permission_key
		FROM iam.memberships m
		JOIN iam.membership_roles mr ON mr.org_id = m.org_id AND mr.membership_id = m.id
		JOIN iam.roles ro ON ro.org_id = mr.org_id AND ro.id = mr.role_id AND ro.status = 'active'
		JOIN iam.role_permissions rp ON rp.org_id = ro.org_id AND rp.role_id = ro.id
		WHERE m.org_id = $1 AND m.user_id = $2 AND m.status = 'active'`, orgID, userID).Rows()
}

// MemberPreviewState 读取预览所需的用户、组织版本和成员有效状态。
func MemberPreviewState(ctx context.Context, db *gorm.DB, orgID string, membershipID string, userID *uuid.UUID, version *int64, memberActive *bool, systemAdmin *bool) error {
	return db.WithContext(ctx).Raw(`SELECT m.user_id,o.authz_version,m.status='active' AND u.status='active' AND o.status='active',u.is_super_admin FROM iam.memberships m JOIN iam.organizations o ON o.id=m.org_id JOIN iam.users u ON u.id=m.user_id WHERE m.org_id=$1 AND m.id=$2`, orgID, membershipID).Row().Scan(userID, version, memberActive, systemAdmin)
}

// ReadPermission 读取权限定义和近期认证约束。
func ReadPermission(ctx context.Context, db *gorm.DB, action string) (authorization.Permission, error) {
	var permission authorization.Permission
	err := db.WithContext(ctx).Table("iam.permissions").Where("key = ?", action).Take(&permission).Error
	return permission, err
}

// ActiveMembership 检查用户是否为组织的有效成员。
func ActiveMembership(ctx context.Context, db *gorm.DB, org uuid.UUID, userID uuid.UUID, value *bool) error {
	return db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM iam.memberships WHERE org_id=$1 AND user_id=$2 AND status='active')`, org, userID).Row().Scan(value)
}

// LockOrganization 在管理事务内锁定组织。
func LockOrganization(ctx context.Context, db *gorm.DB, orgID string, lockedID *uuid.UUID) error {
	return db.WithContext(ctx).Raw(`SELECT id FROM iam.organizations WHERE id=$1 FOR UPDATE`, orgID).Row().Scan(lockedID)
}

// BumpOrganizationVersion 递增组织权限版本，使其他请求及时重算权限。
func BumpOrganizationVersion(ctx context.Context, db *gorm.DB, parsed uuid.UUID) error {
	return db.WithContext(ctx).Exec(`UPDATE iam.organizations SET authz_version=authz_version+1,updated_at=now() WHERE id=$1`, parsed).Error
}
