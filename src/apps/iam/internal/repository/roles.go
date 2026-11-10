package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Role 保存角色及其当前权限；仅在模块内部用于委派校验。
type Role struct {
	ID          string   // 资源标识。
	Name        string   // 角色显示名称。
	Description string   // 用途说明。
	Builtin     bool     // 是否为内置角色；内置角色不可修改或删除。
	UserCount   int64    // 当前关联用户数。
	Permissions []string // 当前权限键集合。
}

// CreateRole 在调用方事务中创建全局角色。
func CreateRole(ctx context.Context, tx *gorm.DB, id uuid.UUID, name, description, actorID string) error {
	err := tx.WithContext(ctx).Table("iam.roles").Create(map[string]any{
		"id":          id,
		"name":        name,
		"description": description,
		"created_by":  actorID,
	}).Error
	return mapRoleError(err)
}

// GrantRolePermissions 为新建角色一次性写入初始权限。
func GrantRolePermissions(ctx context.Context, tx *gorm.DB, roleID string, permissions []string) error {
	if len(permissions) == 0 {
		return nil
	}

	return tx.WithContext(ctx).Exec(`
		INSERT INTO iam.role_permissions(role_id,permission_key)
		SELECT $1, permission_key FROM unnest($2::text[]) AS permission_key
		ON CONFLICT DO NOTHING`, roleID, permissions).Error
}

// RoleGrants 锁定目标角色并读取权限，供相同事务内的委派校验使用。
func RoleGrants(ctx context.Context, tx *gorm.DB, roleID string) (Role, error) {
	role := Role{Permissions: []string{}}
	err := tx.WithContext(ctx).Raw(`
		SELECT r.id::text,r.name,r.description,r.builtin,
		       (SELECT count(*) FROM iam.user_roles ur WHERE ur.role_id=r.id) AS user_count
		FROM iam.roles r WHERE r.id=$1 FOR UPDATE`, roleID).Row().
		Scan(&role.ID, &role.Name, &role.Description, &role.Builtin, &role.UserCount)
	if err != nil {
		return role, err
	}

	err = tx.WithContext(ctx).Table("iam.role_permissions").Where("role_id = ?", roleID).
		Order("permission_key").Pluck("permission_key", &role.Permissions).Error
	return role, err
}

// UpdateRole 更新角色信息，零值描述仍会写入。
func UpdateRole(ctx context.Context, tx *gorm.DB, roleID, name, description string) error {
	err := tx.WithContext(ctx).Table("iam.roles").Where("id = ?", roleID).
		Updates(map[string]any{
			"name":        name,
			"description": description,
			"updated_at":  gorm.Expr("now()"),
		}).Error
	return mapRoleError(err)
}

// DeleteRole 删除角色；角色权限由外键级联删除，用户绑定会阻止删除。
func DeleteRole(ctx context.Context, tx *gorm.DB, roleID string) error {
	err := tx.WithContext(ctx).Table("iam.roles").Where("id = ?", roleID).Delete(nil).Error
	return mapRoleError(err)
}

// ReplaceGrants 保留未变化的授权，只删除本次撤销的键，避免整批重写。
func ReplaceGrants(ctx context.Context, tx *gorm.DB, roleID string, permissions []string) error {
	query := tx.WithContext(ctx).Table("iam.role_permissions").Where("role_id = ?", roleID)
	if len(permissions) > 0 {
		query = query.Where("permission_key NOT IN ?", permissions)
	}

	if err := query.Delete(nil).Error; err != nil {
		return err
	}

	if len(permissions) > 0 {
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO iam.role_permissions(role_id,permission_key)
			SELECT $1, permission_key FROM unnest($2::text[]) AS permission_key
			ON CONFLICT DO NOTHING`, roleID, permissions).Error; err != nil {
			return err
		}
	}

	return tx.WithContext(ctx).Table("iam.roles").Where("id = ?", roleID).
		Updates(map[string]any{"updated_at": gorm.Expr("now()")}).Error
}

// UserRoleIDs 读取目标用户当前持有的角色标识。
func UserRoleIDs(ctx context.Context, tx *gorm.DB, userID string) ([]string, error) {
	ids := []string{}
	err := tx.WithContext(ctx).Table("iam.user_roles").Where("user_id = ?", userID).
		Order("role_id").Pluck("role_id::text", &ids).Error
	return ids, err
}

// ReplaceUserRoles 在已完成委派校验的事务内替换用户角色绑定。
func ReplaceUserRoles(ctx context.Context, tx *gorm.DB, userID string, ids []string, actorID string) error {
	if err := tx.WithContext(ctx).Table("iam.user_roles").Where("user_id = ?", userID).Delete(nil).Error; err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	return tx.WithContext(ctx).Exec(`
		INSERT INTO iam.user_roles(user_id,role_id,created_by)
		SELECT $1, role_id, $3 FROM unnest($2::uuid[]) AS role_id
		ON CONFLICT DO NOTHING`, userID, ids, actorID).Error
}

// mapRoleError 把角色名称冲突与被引用删除映射为业务可区分的错误。
// 通用约束映射只能给出 conflict，调用方无法知道该改名还是先解除用户绑定。
func mapRoleError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch {
	case pgErr.Code == "23505" && pgErr.ConstraintName == "roles_name_normalized_key":
		return ErrRoleNameTaken
	case pgErr.Code == "23503" && pgErr.ConstraintName == "user_roles_role_id_fkey":
		return ErrRoleInUse
	}

	return err
}
