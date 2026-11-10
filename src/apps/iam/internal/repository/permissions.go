package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
)

// Permissions 读取权限目录；module/resource_type/action 由权限键派生，不来自数据库。
func Permissions(ctx context.Context, db *gorm.DB) ([]authorization.Permission, error) {
	items := []authorization.Permission{}
	err := db.WithContext(ctx).Table("iam.permissions").Order("permission_key").Find(&items).Error
	if err != nil {
		return nil, err
	}

	for i := range items {
		module, resource, action, ok := authorization.ParsePermissionKey(items[i].Key)
		if !ok {
			continue
		}

		items[i].Module, items[i].ResourceType, items[i].Action = module, resource, action
	}

	return items, nil
}
