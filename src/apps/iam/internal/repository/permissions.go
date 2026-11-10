package repository

import (
	"context"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
)

// Permissions 读取权限目录；module/resource_type/action 由权限键派生，不来自数据库。
func Permissions(ctx context.Context, db DB) ([]authorization.Permission, error) {
	rows, err := queries(db).ListPermissions(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]authorization.Permission, 0, len(rows))
	for _, row := range rows {
		item := authorization.Permission{
			Key:                row.PermissionKey,
			Description:        row.Description,
			RiskLevel:          row.RiskLevel,
			Delegable:          row.Delegable,
			RequiresRecentAuth: row.RequiresRecentAuth,
			Status:             row.Status,
		}

		module, resource, action, ok := authorization.ParsePermissionKey(item.Key)
		if !ok {
			continue
		}

		item.Module, item.ResourceType, item.Action = module, resource, action
		items = append(items, item)
	}

	return items, nil
}
