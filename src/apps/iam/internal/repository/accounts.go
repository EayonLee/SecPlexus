package repository

import (
	"context"
	"errors"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// InsertUser 创建用户资料，不对外暴露密码记录。
// configured 标记由启动配置写入的内置管理员，数据库只允许一个。
func InsertUser(ctx context.Context, db *gorm.DB, id uuid.UUID, value string, normalized string, displayName string, email string, configured bool) error {
	err := db.WithContext(ctx).Table("iam.users").Create(map[string]any{
		"id":                  id,
		"username":            value,
		"username_normalized": normalized,
		"display_name":        displayName,
		"email":               gorm.Expr("NULLIF(?, '')", email),
		"status":              string(enum.UserStatusActive),
		"configured":          configured,
	}).Error
	return mapUniqueViolation(err)
}

// mapUniqueViolation 按约束名区分用户名与邮箱冲突。
// 通用 23505 映射会落到 conflict，用户无法知道该改哪一项。
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}

	switch pgErr.ConstraintName {
	case "users_username_normalized_key":
		return ErrUsernameTaken
	case "users_email_normalized_key":
		return ErrEmailTaken
	}

	return err
}

// InsertPassword 为新账号写入密码哈希。
func InsertPassword(ctx context.Context, db *gorm.DB, id uuid.UUID, hash string) error {
	return db.WithContext(ctx).Table("iam.password_credentials").Create(map[string]any{
		"user_id":       id,
		"password_hash": hash,
	}).Error
}

// UpdateUser 更新资料与状态；只有访问状态变化才递增账号版本。
// auth_epoch 是「账号版本」：改显示名称或邮箱不影响会话，改状态才会让旧会话失效。
func UpdateUser(ctx context.Context, db *gorm.DB, id uuid.UUID, displayName string, email omit.Val[string], status enum.UserStatus, accessChanged bool) error {
	updates := map[string]any{
		"display_name": displayName,
		"status":       status,
		"updated_at":   gorm.Expr("now()"),
	}

	if accessChanged {
		updates["auth_epoch"] = gorm.Expr("auth_epoch+1")
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

// BumpUserAuthEpoch 增加账号版本，使旧会话失效。
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

// DeleteUser 删除账号。凭据、会话与角色绑定由 users 的级联删除清理，
// 历史创建人引用由 ON DELETE SET NULL 置空，不阻塞删除。
func DeleteUser(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	return db.WithContext(ctx).Table("iam.users").Where("id = ?", id).Delete(nil).Error
}
