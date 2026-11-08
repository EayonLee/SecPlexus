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
		"auth_epoch":   gorm.Expr("auth_epoch+1"),
		"updated_at":   gorm.Expr("now()"),
	}
	if email.IsValue() {
		updates["email"] = gorm.Expr("NULLIF(?, '')", email.MustGet())
	}

	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Updates(updates).Error
}

// UpdateTimeZone 写入本人展示时区偏好；空串存为 NULL 表示未设置。
// 该偏好只影响渲染，不递增 auth_epoch，也不撤销会话。
func UpdateTimeZone(ctx context.Context, db *gorm.DB, id uuid.UUID, timeZone string) error {
	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Updates(map[string]any{
		"timezone":   gorm.Expr("NULLIF(?, '')", timeZone),
		"updated_at": gorm.Expr("now()"),
	}).Error
}

// BumpUserAuthEpoch 增加身份版本，使旧会话失效。
func BumpUserAuthEpoch(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Updates(map[string]any{
		"auth_epoch": gorm.Expr("auth_epoch+1"),
		"updated_at": gorm.Expr("now()"),
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

// UserTimeZone 读取本人展示时区偏好；未设置时返回空串。
func UserTimeZone(ctx context.Context, db *gorm.DB, id uuid.UUID) (string, error) {
	var value *string
	err := db.WithContext(ctx).Table("iam.users").Select("timezone").Where("id = ?", id).Scan(&value).Error
	if err != nil {
		return "", err
	}

	if value == nil {
		return "", nil
	}

	return *value, nil
}
