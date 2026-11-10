package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
)

// LockManagement 取得低频管理写入的事务锁，避免管理员和授权竞态。
func LockManagement(ctx context.Context, db *gorm.DB, managementLock int64) error {
	return db.WithContext(ctx).Exec(`SELECT pg_advisory_xact_lock($1)`, managementLock).Error
}

// ReadPermission 读取权限定义和近期认证约束。
func ReadPermission(ctx context.Context, db *gorm.DB, action string) (authorization.Permission, error) {
	var permission authorization.Permission
	err := db.WithContext(ctx).Table("iam.permissions").Where("permission_key = ?", action).Take(&permission).Error
	return permission, err
}

// EffectivePermissions 读取用户经由角色持有的有效权限。
// 授权只有这一条路径：user_roles → role_permissions → permissions，没有作用域或继承。
func EffectivePermissions(ctx context.Context, db *gorm.DB, userID uuid.UUID) ([]string, error) {
	items := []string{}
	err := db.WithContext(ctx).Raw(`
		SELECT DISTINCT p.permission_key
		FROM iam.user_roles ur
		JOIN iam.role_permissions rp ON rp.role_id = ur.role_id
		JOIN iam.permissions p ON p.permission_key = rp.permission_key AND p.status = 'active'
		WHERE ur.user_id = $1
		ORDER BY p.permission_key`, userID).Scan(&items).Error
	return items, err
}

// UserRoles 读取用户直接持有的角色，按名称排序。
func UserRoles(ctx context.Context, db *gorm.DB, userID uuid.UUID) ([]RoleRef, error) {
	items := []RoleRef{}
	err := db.WithContext(ctx).Raw(`
		SELECT r.id::text, r.name
		FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.name, r.id`, userID).Scan(&items).Error
	return items, err
}

// BuiltinRoleID 返回内置超级管理员角色标识。
func BuiltinRoleID(ctx context.Context, db *gorm.DB) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.WithContext(ctx).Raw("SELECT id FROM iam.roles WHERE builtin").Row().Scan(&id)
	return id, err
}

// ActiveSuperAdminExistsExcept 判断除指定账号外，是否仍有启用中的账号持有内置角色。
// 用于「至少一名超级管理员」不变量；指定账号自身不计入。
func ActiveSuperAdminExistsExcept(ctx context.Context, db *gorm.DB, userID uuid.UUID) (bool, error) {
	var exists bool
	err := db.WithContext(ctx).Raw(
		"SELECT EXISTS(SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id=ur.role_id AND r.builtin JOIN iam.users u ON u.id=ur.user_id AND u.status='active' WHERE ur.user_id <> $1)",
		userID,
	).Row().Scan(&exists)
	return exists, err
}

// RoleRef 是角色在用户权限投影中的最小引用。
type RoleRef struct {
	ID   string `json:"id" gorm:"column:id"`
	Name string `json:"name" gorm:"column:name"`
}
