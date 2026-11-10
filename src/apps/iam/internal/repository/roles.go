package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
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
func CreateRole(ctx context.Context, tx DB, id uuid.UUID, name, description, actorID string) error {
	err := queries(tx).CreateRole(ctx, sqlcgen.CreateRoleParams{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedBy:   actorID,
	})

	return mapRoleError(err)
}

// GrantRolePermissions 为新建角色一次性写入初始权限。
func GrantRolePermissions(ctx context.Context, tx DB, roleID string, permissions []string) error {
	if len(permissions) == 0 {
		return nil
	}

	return grantRolePermissions(ctx, tx, roleID, permissions)
}

// RoleGrants 锁定目标角色并读取权限，供相同事务内的委派校验使用。
func RoleGrants(ctx context.Context, tx DB, roleID string) (Role, error) {
	role := Role{Permissions: []string{}}
	id, err := uuid.Parse(roleID)
	if err != nil {
		return role, err
	}

	row, err := queries(tx).RoleGrants(ctx, id)
	if err != nil {
		return role, err
	}

	permissions, err := queries(tx).RolePermissionKeys(ctx, id)
	if err != nil {
		return role, err
	}

	role.ID = row.ID.String()
	role.Name = row.Name
	role.Description = row.Description
	role.Builtin = row.Builtin
	role.UserCount = row.UserCount
	role.Permissions = permissions
	if role.Permissions == nil {
		role.Permissions = []string{}
	}

	return role, nil
}

// UpdateRole 更新角色信息，零值描述仍会写入。
func UpdateRole(ctx context.Context, tx DB, roleID, name, description string) error {
	id, err := uuid.Parse(roleID)
	if err != nil {
		return err
	}

	err = queries(tx).UpdateRole(ctx, sqlcgen.UpdateRoleParams{ID: id, Name: name, Description: description})

	return mapRoleError(err)
}

// DeleteRole 删除角色；角色权限由外键级联删除，用户绑定会阻止删除。
func DeleteRole(ctx context.Context, tx DB, roleID string) error {
	id, err := uuid.Parse(roleID)
	if err != nil {
		return err
	}

	err = queries(tx).DeleteRole(ctx, id)

	return mapRoleError(err)
}

// ReplaceGrants 保留未变化的授权，只删除本次撤销的键，避免整批重写。
func ReplaceGrants(ctx context.Context, tx DB, roleID string, permissions []string) error {
	id, err := uuid.Parse(roleID)
	if err != nil {
		return err
	}

	keep, err := textArray(permissions)
	if err != nil {
		return err
	}

	if err = queries(tx).DeleteRoleGrantsNotIn(ctx, sqlcgen.DeleteRoleGrantsNotInParams{RoleID: id, Keep: keep}); err != nil {
		return err
	}

	if len(permissions) > 0 {
		if err = grantRolePermissions(ctx, tx, roleID, permissions); err != nil {
			return err
		}
	}

	return queries(tx).TouchRole(ctx, id)
}

// UserRoleIDs 读取目标用户当前持有的角色标识。
func UserRoleIDs(ctx context.Context, tx DB, userID string) ([]string, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	ids, err := queries(tx).UserRoleIDs(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]string, 0, len(ids))
	for _, roleID := range ids {
		items = append(items, roleID.String())
	}

	return items, nil
}

// ReplaceUserRoles 在已完成委派校验的事务内替换用户角色绑定。
func ReplaceUserRoles(ctx context.Context, tx DB, userID string, ids []string, actorID string) error {
	id, err := uuid.Parse(userID)
	if err != nil {
		return err
	}

	if err = queries(tx).DeleteUserRoles(ctx, id); err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	roles, err := textArray(ids)
	if err != nil {
		return err
	}

	return queries(tx).InsertUserRoles(ctx, sqlcgen.InsertUserRolesParams{UserID: id, RoleIds: roles, CreatedBy: actorID})
}

// grantRolePermissions 把权限集合写入角色；集合以 jsonb 传入，SQL 侧展开。
func grantRolePermissions(ctx context.Context, tx DB, roleID string, permissions []string) error {
	id, err := uuid.Parse(roleID)
	if err != nil {
		return err
	}

	encoded, err := textArray(permissions)
	if err != nil {
		return err
	}

	return queries(tx).GrantRolePermissions(ctx, sqlcgen.GrantRolePermissionsParams{RoleID: id, Permissions: encoded})
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
