package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LockUsers 按统一 UUID 顺序锁定用户，登录轮换与管理变更遵循同一锁顺序。
func LockUsers(ctx context.Context, tx *gorm.DB, ids []uuid.UUID) error {
	err := tx.WithContext(ctx).Exec(`SELECT id FROM iam.users WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids).Error
	return err
}
