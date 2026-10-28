package repository

import (
	"context"

	"gorm.io/gorm"

	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// Permissions 使用当前连接或事务读取权限目录，不重复维护另一份权限表。
func Permissions(ctx context.Context, db *gorm.DB) ([]contract.Permission, error) {
	items := []contract.Permission{}
	err := db.WithContext(ctx).Table("iam.permissions").Order("module,resource_type,key").Find(&items).Error
	return items, err
}
