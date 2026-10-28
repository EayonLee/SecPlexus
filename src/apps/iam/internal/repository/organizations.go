package repository

import (
	"context"

	"gorm.io/gorm"
)

// LockOrganizationVersion 锁定组织并读取权限版本。
func LockOrganizationVersion(ctx context.Context, db *gorm.DB, orgID string, version *int64) error {
	return db.WithContext(ctx).Raw(`SELECT authz_version FROM iam.organizations WHERE id=$1 FOR UPDATE`, orgID).Row().Scan(version)
}

// UpdateOrganization 更新组织信息，仅平台操作额外增加权限版本。
func UpdateOrganization(ctx context.Context, db *gorm.DB, orgID string, name string, status string, platform bool) error {
	values := map[string]any{"name": name, "updated_at": gorm.Expr("now()")}
	if status != "" {
		values["status"] = status
	}

	if platform {
		values["authz_version"] = gorm.Expr("authz_version+1")
	}

	return db.WithContext(ctx).Table("iam.organizations").Where("id = ?", orgID).Updates(values).Error
}
