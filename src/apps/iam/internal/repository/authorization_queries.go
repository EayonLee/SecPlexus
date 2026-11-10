package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
)

// LockManagement 取得低频管理写入的事务锁，避免管理员和授权竞态。
func LockManagement(ctx context.Context, db *gorm.DB, managementLock int64) error {
	if err := requireTransaction(db, "LockManagement"); err != nil {
		return err
	}

	return queries(db).LockManagement(ctx, managementLock)
}

// ReadPermission 读取权限定义和近期认证约束。
func ReadPermission(ctx context.Context, db *gorm.DB, action string) (authorization.Permission, error) {
	row, err := queries(db).ReadPermission(ctx, action)
	if err != nil {
		return authorization.Permission{}, err
	}

	permission := authorization.Permission{
		Key:                row.PermissionKey,
		Description:        row.Description,
		RiskLevel:          row.RiskLevel,
		Delegable:          row.Delegable,
		RequiresRecentAuth: row.RequiresRecentAuth,
		Status:             row.Status,
	}
	module, resource, act, ok := authorization.ParsePermissionKey(row.PermissionKey)
	if !ok {
		return authorization.Permission{}, errors.New("permission key is not in module.resource.action form")
	}
	permission.Module = module
	permission.ResourceType = resource
	permission.Action = act

	return permission, nil
}

// EffectivePermissions 读取用户经由角色持有的有效权限。
// 授权只有这一条路径：user_roles → role_permissions → permissions，没有作用域或继承。
func EffectivePermissions(ctx context.Context, db *gorm.DB, userID uuid.UUID) ([]string, error) {
	items, err := queries(db).EffectivePermissions(ctx, userID)
	if err != nil {
		return nil, err
	}

	return items, nil
}

// UserRoles 读取用户直接持有的角色，按名称排序。
func UserRoles(ctx context.Context, db *gorm.DB, userID uuid.UUID) ([]RoleRef, error) {
	rows, err := queries(db).UserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	items := make([]RoleRef, 0, len(rows))
	for _, row := range rows {
		items = append(items, RoleRef{ID: row.ID.String(), Name: row.Name})
	}

	return items, nil
}

// BuiltinRoleID 返回内置超级管理员角色标识。
func BuiltinRoleID(ctx context.Context, db *gorm.DB) (uuid.UUID, error) {
	return queries(db).BuiltinRoleID(ctx)
}

// ActiveSuperAdminExistsExcept 判断除指定账号外，是否仍有启用中的账号持有内置角色。
// 用于「至少一名超级管理员」不变量；指定账号自身不计入。
func ActiveSuperAdminExistsExcept(ctx context.Context, db *gorm.DB, userID uuid.UUID) (bool, error) {
	return queries(db).ActiveSuperAdminExistsExcept(ctx, userID)
}

// RoleRef 是角色在用户权限投影中的最小引用。
type RoleRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
