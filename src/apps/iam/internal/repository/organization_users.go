package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InsertOrganizationUser 创建有效组织用户关系。
func InsertOrganizationUser(ctx context.Context, db *gorm.DB, organizationUserID uuid.UUID, orgID string, userID uuid.UUID) error {
	return db.WithContext(ctx).Table("iam.organization_users").Create(map[string]any{
		"id":      organizationUserID,
		"org_id":  orgID,
		"user_id": userID,
		"status":  "active",
	}).Error
}

// LockOrganizationUser 锁定用户并读取其关联账号，供同一事务内的用户变更使用。
func LockOrganizationUser(ctx context.Context, db *gorm.DB, orgID string, organizationUserID string, userID *uuid.UUID) error {
	return db.WithContext(ctx).Raw(
		`SELECT user_id FROM iam.organization_users WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, organizationUserID,
	).Row().Scan(userID)
}

// DeleteOrganizationUserRoles 删除目标用户的组织内角色关系。
func DeleteOrganizationUserRoles(ctx context.Context, db *gorm.DB, orgID string, organizationUserID string) error {
	return db.WithContext(ctx).Table("iam.organization_user_roles").Where("org_id = ? AND organization_user_id = ?", orgID, organizationUserID).Delete(nil).Error
}

// DeleteOrganizationUsership 删除组织用户关系。
func DeleteOrganizationUsership(ctx context.Context, db *gorm.DB, orgID string, organizationUserID string) error {
	return db.WithContext(ctx).Table("iam.organization_users").Where("org_id = ? AND id = ?", orgID, organizationUserID).Delete(nil).Error
}

// UpdateOrganizationUsership 更新组织用户状态。
func UpdateOrganizationUsership(ctx context.Context, db *gorm.DB, orgID string, organizationUserID string, status string) error {
	return db.WithContext(ctx).Table("iam.organization_users").Where("org_id = ? AND id = ?", orgID, organizationUserID).
		Updates(map[string]any{"status": status, "updated_at": gorm.Expr("now()")}).Error
}
