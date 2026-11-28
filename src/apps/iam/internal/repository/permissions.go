package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// VerifyPermissionCatalog 从数据库只读核对当前版本的授权目录投影。
func (s *Store) VerifyPermissionCatalog(ctx context.Context) error {
	rows, err := queries(s.DB).ListPermissions(ctx)
	if err != nil {
		return fmt.Errorf("read IAM permission catalog: %w", err)
	}

	catalog := authorization.Catalog()
	expected := make(map[string]authorization.Permission, len(catalog))
	for _, item := range catalog {
		expected[item.Key] = item
	}

	for _, row := range rows {
		item, exists := expected[row.PermissionKey]
		if !exists && row.Status == enum.PermissionStatusRetired {
			continue
		}
		if !exists || row.Status != item.Status || row.Description != item.Description ||
			row.RiskLevel != item.RiskLevel || row.Delegable != item.Delegable || row.RequiresRecentAuth != item.RequiresRecentAuth {
			return errors.New("IAM permission catalog is incompatible; run the matching migration before serving")
		}
		delete(expected, row.PermissionKey)
	}
	if len(expected) != 0 {
		return errors.New("IAM permission catalog is incomplete; run the matching migration before serving")
	}

	return nil
}

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
