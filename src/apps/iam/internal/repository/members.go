package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InsertMembership 创建有效组织成员关系。
func InsertMembership(ctx context.Context, db *gorm.DB, membershipID uuid.UUID, orgID string, userID uuid.UUID) error {
	return db.WithContext(ctx).Table("iam.memberships").Create(map[string]any{
		"id":      membershipID,
		"org_id":  orgID,
		"user_id": userID,
		"status":  "active",
	}).Error
}

// AssignMemberRole 为新成员关联组织内置普通成员角色。
func AssignMemberRole(ctx context.Context, db *gorm.DB, orgID string, membershipID uuid.UUID, userID string) error {
	return db.WithContext(ctx).Exec(`INSERT INTO iam.membership_roles(org_id,membership_id,role_id,created_by) SELECT $1,$2,id,$3 FROM iam.roles WHERE org_id=$1 AND kind='system' AND code='member'`, orgID, membershipID, userID).Error
}

// LockMembershipAdministrator 锁定成员并检查其是否持有组织管理员角色。
func LockMembershipAdministrator(ctx context.Context, db *gorm.DB, orgID string, memberID string, userID *uuid.UUID, admin *bool) error {
	return db.WithContext(ctx).Raw(`SELECT user_id,EXISTS(SELECT 1 FROM iam.membership_roles mr JOIN iam.roles r ON r.org_id=mr.org_id AND r.id=mr.role_id WHERE mr.org_id=m.org_id AND mr.membership_id=m.id AND r.kind='system' AND r.code='org_admin') FROM iam.memberships m WHERE m.org_id=$1 AND m.id=$2 FOR UPDATE`, orgID, memberID).Row().Scan(userID, admin)
}

// DeleteMemberRoles 删除目标成员的组织内角色关系。
func DeleteMemberRoles(ctx context.Context, db *gorm.DB, orgID string, memberID string) error {
	return db.WithContext(ctx).Table("iam.membership_roles").Where("org_id = ? AND membership_id = ?", orgID, memberID).Delete(nil).Error
}

// DeleteMembership 删除组织成员关系。
func DeleteMembership(ctx context.Context, db *gorm.DB, orgID string, memberID string) error {
	return db.WithContext(ctx).Table("iam.memberships").Where("org_id = ? AND id = ?", orgID, memberID).Delete(nil).Error
}

// UpdateMembership 更新组织成员状态。
func UpdateMembership(ctx context.Context, db *gorm.DB, orgID string, memberID string, status string) error {
	return db.WithContext(ctx).Table("iam.memberships").Where("org_id = ? AND id = ?", orgID, memberID).
		Updates(map[string]any{"status": status, "updated_at": gorm.Expr("now()")}).Error
}
