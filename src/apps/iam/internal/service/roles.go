package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// validName 拒绝只有空白的业务名称；字段长度由入口绑定约束。
func validName(name string) bool {
	return strings.TrimSpace(name) != ""
}

// CreateRole 创建全局角色；角色名全局唯一。
func (s *Service) CreateRole(ctx context.Context, input RoleCreate, requestID string) (string, error) {
	if !validName(input.Name) {
		return "", repository.ErrInvalid
	}

	description := input.Description.GetOrZero()
	name := strings.TrimSpace(input.Name)
	id := uuid.New()
	err := s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		requestID,
		"role",
		&id,
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			// 新角色没有任何既有授权，这里只需确认调用方有权授予这些权限。
			if err := s.delegation(ctx, tx, p, nil, input.Permissions); err != nil {
				return err
			}

			metadata["name"] = name
			if err := repository.CreateRole(ctx, tx, id, name, description, p.UserID); err != nil {
				return err
			}

			return repository.GrantRolePermissions(ctx, tx, id.String(), input.Permissions)
		},
	)
	return id.String(), err
}

// delegation 校验操作者现有权限能否覆盖变更前后的授权。
// 只由「用户 → 角色 → 权限」计算，没有组织、超管标记或作用域旁路。
func (s *Service) delegation(ctx context.Context, tx *sql.Tx, p iamclient.Principal, old, next []string) error {
	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	effective, err := repository.EffectivePermissions(ctx, tx, userID)
	if err != nil {
		return err
	}

	catalog, err := repository.Permissions(ctx, tx)
	if err != nil {
		return err
	}

	return authorization.CheckDelegation(effective, old, next, catalog)
}

// UpdateRole 更新角色信息；内置角色不可修改。
func (s *Service) UpdateRole(ctx context.Context, roleID string, input RoleUpdate, requestID string) error {
	if !validName(input.Name) {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, roleID)
			if err != nil {
				return err
			}

			if role.Builtin {
				return repository.ErrBuiltinRoleImmutable
			}

			if err = s.delegation(ctx, tx, p, role.Permissions, role.Permissions); err != nil {
				return err
			}

			name := strings.TrimSpace(input.Name)
			metadata["name"] = name
			description := input.Description.GetOr(role.Description)
			return repository.UpdateRole(ctx, tx, roleID, name, description)
		},
	)
}

// ReplaceGrants 替换角色权限并检查可委派范围；内置角色不可修改。
func (s *Service) ReplaceGrants(ctx context.Context, roleID string, input RoleGrants, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, roleID)
			if err != nil {
				return err
			}

			if role.Builtin {
				return repository.ErrBuiltinRoleImmutable
			}

			if err = s.delegation(ctx, tx, p, role.Permissions, input.Permissions); err != nil {
				return err
			}

			added, removed := stringDelta(role.Permissions, input.Permissions)
			metadata["permissions_added"] = added
			metadata["permissions_removed"] = removed
			return repository.ReplaceGrants(ctx, tx, roleID, input.Permissions)
		},
	)
}

// DeleteRole 删除角色；内置角色或仍有用户持有的角色不可删除。
func (s *Service) DeleteRole(ctx context.Context, roleID, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, roleID)
			if err != nil {
				return err
			}

			if role.Builtin {
				return repository.ErrBuiltinRoleImmutable
			}

			if role.UserCount > 0 {
				return repository.ErrRoleInUse
			}

			if err = s.delegation(ctx, tx, p, role.Permissions, nil); err != nil {
				return err
			}

			return repository.DeleteRole(ctx, tx, roleID)
		},
	)
}

// ReplaceUserRoles 全量替换用户角色并检查可委派范围。
func (s *Service) ReplaceUserRoles(ctx context.Context, userID string, input UserRoles, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMUsersRoles,
		requestID,
		"user",
		uuidPointer(userID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			id, err := uuid.Parse(userID)
			if err != nil {
				return repository.ErrInvalid
			}

			target, err := repository.LockedAccount(ctx, tx, id)
			if err != nil {
				return err
			}

			previous, err := repository.UserRoleIDs(ctx, tx, id.String())
			if err != nil {
				return err
			}

			oldGrants, err := grantsFor(ctx, tx, previous)
			if err != nil {
				return err
			}

			newGrants, err := grantsFor(ctx, tx, input.RoleIDs)
			if err != nil {
				return err
			}

			if err = s.delegation(ctx, tx, p, oldGrants, newGrants); err != nil {
				return err
			}

			builtin, err := repository.BuiltinRoleID(ctx, tx)
			if err != nil {
				return err
			}

			keepsBuiltin := slices.Contains(input.RoleIDs, builtin.String())
			// 配置管理员的内置角色由部署配置保证；页面摘除只会在下次启动时被补回。
			if target.Configured && !keepsBuiltin {
				return repository.ErrConfiguredUserImmutable
			}

			if slices.Contains(previous, builtin.String()) && !keepsBuiltin {
				if err = s.requireOtherSuperAdmin(ctx, tx, id); err != nil {
					return err
				}
			}

			added, removed := stringDelta(previous, input.RoleIDs)
			metadata["roles_added"] = added
			metadata["roles_removed"] = removed
			return repository.ReplaceUserRoles(ctx, tx, id.String(), input.RoleIDs, p.UserID)
		},
	)
}

// grantsFor 读取一组角色授予的权限；任何角色不存在都视为无效输入。
func grantsFor(ctx context.Context, tx *sql.Tx, roleIDs []string) ([]string, error) {
	ids := slices.Clone(roleIDs)
	// 统一加锁顺序，避免两个并发变更以相反顺序锁同一组角色。
	slices.Sort(ids)
	permissions := []string{}
	for _, raw := range ids {
		if _, err := uuid.Parse(raw); err != nil {
			return nil, repository.ErrInvalid
		}

		role, err := repository.RoleGrants(ctx, tx, raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrInvalid
		}

		if err != nil {
			return nil, err
		}

		permissions = append(permissions, role.Permissions...)
	}

	return permissions, nil
}

// uuidPointer 把可选资源标识转换为审计目标。
func uuidPointer(value string) *uuid.UUID {
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}

	return &id
}

// requireOtherSuperAdmin 保证变更后仍有其它启用中的账号持有内置角色。
// 只在目标账号确实持有内置角色时校验；否则任何变更都不会减少超级管理员数量。
func (s *Service) requireOtherSuperAdmin(ctx context.Context, tx *sql.Tx, userID uuid.UUID) error {
	holds, err := userHoldsBuiltin(ctx, tx, userID)
	if err != nil || !holds {
		return err
	}

	exists, err := repository.ActiveSuperAdminExistsExcept(ctx, tx, userID)
	if err != nil {
		return err
	}

	if !exists {
		return repository.ErrLastSuperAdmin
	}

	return nil
}

// userHoldsBuiltin 判断账号是否直接持有内置超级管理员角色。
func userHoldsBuiltin(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (bool, error) {
	builtin, err := repository.BuiltinRoleID(ctx, tx)
	if err != nil {
		return false, err
	}

	roles, err := repository.UserRoleIDs(ctx, tx, userID.String())
	if err != nil {
		return false, err
	}

	return slices.Contains(roles, builtin.String()), nil
}

// stringDelta 返回 after 新增与缺少的键；审计与预览都只关心差异。
// 返回值永远是空切片而不是 nil，调用方无需再区分 JSON 里的 [] 与 null。
func stringDelta(before, after []string) (added, removed []string) {
	added, removed = []string{}, []string{}
	for _, value := range after {
		if !slices.Contains(before, value) {
			added = append(added, value)
		}
	}

	for _, value := range before {
		if !slices.Contains(after, value) {
			removed = append(removed, value)
		}
	}

	return added, removed
}
