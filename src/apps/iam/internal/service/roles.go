package service

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// RoleInput 是修改角色的请求参数。
type RoleInput = model.APIRoleInput

// RoleCreateInput 是创建角色的请求参数。
type RoleCreateInput = model.APIRoleCreate

// GrantsInput 是角色权限变更的请求参数。
type GrantsInput = model.APIGrantsInput

// OrganizationUserRolesInput 是用户角色变更的请求参数。
type OrganizationUserRolesInput = model.APIOrganizationUserRolesInput

// validName 拒绝只有空白的业务名称；字段长度由入口绑定约束。
func validName(name string) bool {
	return strings.TrimSpace(name) != ""
}

// CreateRole 创建组织角色。
func (s *Service) CreateRole(ctx context.Context, orgID string, input RoleCreateInput, requestID string) (string, error) {
	if !validName(input.Name) {
		return "", repository.ErrInvalid
	}

	description := input.Description.GetOrZero()
	id := uuid.New()
	err := s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		&id,
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			// 新角色没有任何既有授权，这里只需确认调用方有权授予这些权限。
			if err := s.delegation(ctx, tx, p, orgID, nil, input.Permissions); err != nil {
				return err
			}

			metadata["name"] = input.Name
			if err := repository.CreateRole(ctx, tx, id, orgID, input.Name, description, p.UserID); err != nil {
				return err
			}

			return repository.GrantRolePermissions(ctx, tx, orgID, id.String(), input.Permissions)
		},
	)
	return id.String(), err
}

// delegation 校验操作者现有权限能否覆盖变更前后的授权。
func (s *Service) delegation(ctx context.Context, tx *gorm.DB, p iamclient.Principal, orgID string, old, next []string) error {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return repository.ErrInvalid
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return err
	}

	effective, err := repository.NewQueries(tx).GetOrganizationPermissions(ctx, repository.GetOrganizationPermissionsParams{OrgID: org,
		UserID: userID})
	if err != nil {
		return err
	}

	catalog, err := repository.Permissions(ctx, tx)
	if err != nil {
		return err
	}

	if p.IsSuperAdmin {
		for _, permission := range catalog {
			if permission.Scope == "organization" {
				effective = append(effective, permission.Key)
			}
		}
	}

	return authorization.CheckDelegation(effective, old, next, catalog)
}

// UpdateRole 更新角色信息并检查版本。
func (s *Service) UpdateRole(ctx context.Context, orgID, roleID string, input RoleInput, requestID string) error {
	if !validName(input.Name) {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, orgID, roleID)
			if err != nil {
				return err
			}

			if role.Version != input.Version {
				return repository.ErrConflict
			}

			if err = s.delegation(ctx, tx, p, orgID, role.Permissions, role.Permissions); err != nil {
				return err
			}

			metadata["previous_version"] = strconv.FormatInt(role.Version, 10)
			metadata["name"] = input.Name
			description := input.Description.GetOr(role.Description)
			return repository.UpdateRole(ctx, tx, orgID, roleID, input.Name, description)
		},
	)
}

// ReplaceGrants 替换角色权限并检查可委派范围。
func (s *Service) ReplaceGrants(ctx context.Context, orgID, roleID string, input GrantsInput, requestID string) error {

	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, orgID, roleID)
			if err != nil {
				return err
			}

			if role.Version != input.Version {
				return repository.ErrConflict
			}

			if err = s.delegation(ctx, tx, p, orgID, role.Permissions, input.Permissions); err != nil {
				return err
			}

			metadata["before_permissions"] = role.Permissions
			metadata["after_permissions"] = input.Permissions
			metadata["previous_version"] = strconv.FormatInt(role.Version, 10)
			return repository.ReplaceGrants(ctx, tx, orgID, roleID, input.Permissions)
		},
	)
}

// DeleteRole 删除自定义角色并检查版本。
func (s *Service) DeleteRole(ctx context.Context, orgID, roleID string, version int64, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, orgID, roleID)
			if err != nil {
				return err
			}

			if role.Version != version || role.UserCount > 0 {
				return repository.ErrConflict
			}

			if err = s.delegation(ctx, tx, p, orgID, role.Permissions, nil); err != nil {
				return err
			}

			return repository.DeleteRole(ctx, tx, orgID, roleID)
		},
	)
}

// ReplaceOrganizationUserRoles 替换用户角色并检查可委派范围。
func (s *Service) ReplaceOrganizationUserRoles(ctx context.Context, orgID, organizationUserID string, input OrganizationUserRolesInput, requestID string) error {

	return s.Manage(
		ctx,
		iamclient.PermissionIAMRolesAssign,
		orgID,
		requestID,
		"membership",
		uuidPointer(organizationUserID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			// 先锁定组织并核对版本，防止覆盖其他管理员刚提交的变更。
			version, err := repository.OrganizationVersion(ctx, tx, orgID)
			if err != nil {
				return err
			}

			if version != input.AuthzEpoch {
				return repository.ErrConflict
			}

			// 用户必须属于当前组织；不能只凭客户端提交的 ID 分配角色。
			exists, err := repository.OrganizationUserExists(ctx, tx, orgID, organizationUserID)
			if err != nil {
				return err
			}

			if !exists {
				return repository.ErrNotFound
			}

			var targetID uuid.UUID
			if err := repository.LockOrganizationUser(ctx, tx, orgID, organizationUserID, &targetID); err != nil {
				return err
			}
			if _, err := repository.NewQueries(tx).LockMutableUser(ctx, targetID); err != nil {
				return err
			}

			previous, err := repository.OrganizationUserRoleIDs(ctx, tx, orgID, organizationUserID)
			if err != nil {
				return err
			}

			// 不能授予自己无权管理的授权。
			for _, set := range [][]string{previous, input.RoleIDs} {
				for _, id := range set {
					role, err := repository.RoleGrants(ctx, tx, orgID, id)
					if err != nil {
						return err
					}

					{
						existingGrants := role.Permissions
						if !slices.Contains(previous, id) {
							existingGrants = nil
						}

						candidateGrants := []string{}
						if slices.Contains(input.RoleIDs, id) {
							candidateGrants = role.Permissions
						}

						if err = s.delegation(ctx, tx, p, orgID, existingGrants, candidateGrants); err != nil {
							return err
						}
					}
				}
			}

			// 业务变更、管理员保护与审计由 Manage 在同一事务内提交。
			metadata["before_role_ids"] = previous
			metadata["after_role_ids"] = input.RoleIDs
			metadata["expected_authz_epoch"] = strconv.FormatInt(input.AuthzEpoch, 10)
			if err := repository.ReplaceOrganizationUserRoles(ctx, tx, orgID, organizationUserID, input.RoleIDs, p.UserID); err != nil {
				return err
			}

			return nil
		},
	)
}

// uuidPointer 把可选资源标识转换为审计目标。
func uuidPointer(value string) *uuid.UUID {
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}

	return &id
}
