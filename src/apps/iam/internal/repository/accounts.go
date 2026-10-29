package repository

import (
	"context"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InsertUser 创建用户资料，不对外暴露密码记录。
func InsertUser(ctx context.Context, db *gorm.DB, id uuid.UUID, value string, normalized string, displayName string, email string) error {
	return db.WithContext(ctx).Table("iam.users").Create(map[string]any{
		"id":                  id,
		"username":            value,
		"username_normalized": normalized,
		"display_name":        displayName,
		"email":               gorm.Expr("NULLIF(?, '')", email),
		"status":              "active",
	}).Error
}

// InsertPassword 为新账号写入密码哈希。
func InsertPassword(ctx context.Context, db *gorm.DB, id uuid.UUID, hash string) error {
	return db.WithContext(ctx).Table("iam.password_credentials").Create(map[string]any{
		"user_id":       id,
		"password_hash": hash,
	}).Error
}

// UpdateUser 更新资料和状态，同时增加身份版本。
func UpdateUser(ctx context.Context, db *gorm.DB, id uuid.UUID, displayName string, email omit.Val[string], status string) error {
	updates := map[string]any{
		"display_name": displayName,
		"status":       status,
		"auth_version": gorm.Expr("auth_version+1"),
		"updated_at":   gorm.Expr("now()"),
	}
	if email.IsValue() {
		updates["email"] = gorm.Expr("NULLIF(?, '')", email.MustGet())
	}

	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Updates(updates).Error
}

// SetSuperAdmin 更新管理员标记和身份版本。
func SetSuperAdmin(ctx context.Context, db *gorm.DB, id uuid.UUID, enabled bool) error {
	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Updates(map[string]any{
		"is_super_admin": enabled,
		"auth_version":   gorm.Expr("auth_version+1"),
		"updated_at":     gorm.Expr("now()"),
	}).Error
}

// BumpUserAuthVersion 增加身份版本，使旧会话失效。
func BumpUserAuthVersion(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Updates(map[string]any{
		"auth_version": gorm.Expr("auth_version+1"),
		"updated_at":   gorm.Expr("now()"),
	}).Error
}

// ResetPassword 更新密码哈希，同时清除失败和锁定状态。
func ResetPassword(ctx context.Context, db *gorm.DB, id uuid.UUID, hash string) error {
	return db.WithContext(ctx).Table("iam.password_credentials").Where("user_id = ?", id).Updates(map[string]any{
		"password_hash":   hash,
		"changed_at":      gorm.Expr("now()"),
		"failed_attempts": 0,
		"locked_until":    nil,
		"updated_at":      gorm.Expr("now()"),
	}).Error
}
