package service

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

var roleCode = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

// RoleInput 是创建或修改角色的请求参数。
type RoleInput = contract.APIRoleInput

// GrantsInput 是角色权限变更的请求参数。
type GrantsInput = contract.APIGrantsInput

// MemberRolesInput 是成员角色变更的请求参数。
type MemberRolesInput = contract.APIMemberRolesInput

// validName 限制非空显示名称的字符数。
func validName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 100
}

// validRole 校验角色资料的业务约束，不改变系统角色保护。
func validRole(input RoleInput) bool {
	return validName(input.Name) && utf8.RuneCountInString(input.Description) <= 1000 && (input.Status == "active" || input.Status == "disabled")
}

// CreateRole 创建组织自定义角色。
func (s *Service) CreateRole(ctx context.Context, orgID string, input RoleInput, requestID string) (string, error) {
	if input.Status == "" {
		input.Status = "active"
	}

	if !roleCode.MatchString(input.Code) || slices.Contains([]string{"org_admin", "member", "super_admin"}, input.Code) || !validRole(input) {
		return "", repository.ErrInvalid
	}

	id := uuid.New()
	err := s.Manage(
		ctx,
		contract.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		&id,
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			metadata["code"] = input.Code
			metadata["name"] = input.Name
			return repository.CreateRole(ctx, tx, id, orgID, input.Code, input.Name, input.Description, input.Status, p.UserID)
		},
	)
	return id.String(), err
}

// delegation 校验操作者现有权限能否覆盖变更前后的授权。
func (s *Service) delegation(ctx context.Context, tx *gorm.DB, p contract.Principal, orgID string, old, next []string) error {
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
	if !validRole(input) || input.Version < 1 || input.Code != "" {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		contract.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, orgID, roleID)
			if err != nil {
				return err
			}

			if role.Kind != "custom" {
				return authorization.ErrDenied
			}

			if role.Version != input.Version {
				return repository.ErrConflict
			}

			if err = s.delegation(ctx, tx, p, orgID, role.Permissions, role.Permissions); err != nil {
				return err
			}

			metadata["previous_version"] = strconv.FormatInt(role.Version, 10)
			metadata["name"] = input.Name
			metadata["status"] = input.Status
			return repository.UpdateRole(ctx, tx, orgID, roleID, input.Name, input.Description, input.Status)
		},
	)
}

// ReplaceGrants 替换角色权限并检查可委派范围。
func (s *Service) ReplaceGrants(ctx context.Context, orgID, roleID string, input GrantsInput, requestID string) error {
	if input.Permissions == nil || input.Version < 1 || len(input.Permissions) > 200 {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		contract.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, orgID, roleID)
			if err != nil {
				return err
			}

			if role.Kind != "custom" {
				return authorization.ErrDenied
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
			seen := map[string]bool{}
			for _, key := range input.Permissions {
				if seen[key] {
					return repository.ErrInvalid
				}

				seen[key] = true
			}

			return repository.ReplaceGrants(ctx, tx, orgID, roleID, input.Permissions)
		},
	)
}

// DeleteRole 删除自定义角色并检查版本。
func (s *Service) DeleteRole(ctx context.Context, orgID, roleID string, version int64, requestID string) error {
	return s.Manage(
		ctx,
		contract.PermissionIAMRolesManage,
		orgID,
		requestID,
		"role",
		uuidPointer(roleID),
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			role, err := repository.RoleGrants(ctx, tx, orgID, roleID)
			if err != nil {
				return err
			}

			if role.Kind != "custom" {
				return authorization.ErrDenied
			}

			if role.Version != version || role.MemberCount > 0 {
				return repository.ErrConflict
			}

			if err = s.delegation(ctx, tx, p, orgID, role.Permissions, nil); err != nil {
				return err
			}

			return repository.DeleteRole(ctx, tx, orgID, roleID)
		},
	)
}

// ReplaceMemberRoles 替换成员角色并检查可委派范围。
func (s *Service) ReplaceMemberRoles(ctx context.Context, orgID, membershipID string, input MemberRolesInput, requestID string) error {
	if input.RoleIDs == nil || input.AuthzVersion < 1 || len(input.RoleIDs) > 50 {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		contract.PermissionIAMRolesAssign,
		orgID,
		requestID,
		"membership",
		uuidPointer(membershipID),
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			// 先锁定组织并核对版本，防止覆盖其他管理员刚提交的变更。
			version, err := repository.OrganizationVersion(ctx, tx, orgID)
			if err != nil {
				return err
			}

			if version != input.AuthzVersion {
				return repository.ErrConflict
			}

			// 成员必须属于当前组织；不能只凭客户端提交的 ID 分配角色。
			exists, err := repository.MemberExists(ctx, tx, orgID, membershipID)
			if err != nil {
				return err
			}

			if !exists {
				return repository.ErrNotFound
			}

			previous, err := repository.MemberRoleIDs(ctx, tx, orgID, membershipID)
			if err != nil {
				return err
			}

			// 拒绝重复角色，旧、新集合都参与后续委派校验。
			seen := map[string]bool{}
			for _, id := range input.RoleIDs {
				if seen[id] {
					return repository.ErrInvalid
				}

				seen[id] = true
			}

			// 普通组织管理员不能调整管理员身份，也不能撤销自己无权管理的授权。
			for _, set := range [][]string{previous, input.RoleIDs} {
				for _, id := range set {
					role, err := repository.RoleGrants(ctx, tx, orgID, id)
					if err != nil {
						return err
					}

					if role.Kind == "system" && role.Code == "org_admin" && !p.IsSuperAdmin &&
						slices.Contains(previous, id) != slices.Contains(input.RoleIDs, id) {
						return authorization.ErrDenied
					}

					if role.Kind == "custom" {
						if role.Status != "active" && slices.Contains(input.RoleIDs, id) {
							return repository.ErrInvalid
						}

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
			metadata["expected_authz_version"] = strconv.FormatInt(input.AuthzVersion, 10)
			if err := repository.ReplaceMemberRoles(ctx, tx, orgID, membershipID, input.RoleIDs, p.UserID); err != nil {
				return err
			}

			return repository.ProtectLastAdministrators(ctx, tx)
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
