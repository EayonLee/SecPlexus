package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// PreviewInput 描述拟预览的角色或成员权限变更。
type PreviewInput struct {
	RoleID       string    `json:"role_id"`              // 目标角色标识。
	Permissions  *[]string `json:"permissions"`          // 有效权限键集合。
	Version      int64     `json:"version,string"`       // 角色乐观锁版本。
	MembershipID string    `json:"membership_id"`        // 目标组织成员关系。
	RoleIDs      *[]string `json:"role_ids"`             // 拟设置的角色标识集合。
	AuthzVersion int64     `json:"authz_version,string"` // 组织权限版本，防止过期写入。
}

// Preview 描述权限变更前后的差异及影响范围。
type Preview struct {
	Before          []string `json:"before"`           // 变更前有效权限。
	After           []string `json:"after"`            // 拟变更后的有效权限。
	Added           []string `json:"added"`            // 新增权限。
	Removed         []string `json:"removed"`          // 移除权限。
	AffectedMembers int64    `json:"affected_members"` // 受影响成员数量。
}

// Preview 比较角色权限或成员角色变更前后的权限。
func (s *Service) Preview(ctx context.Context, orgID string, input PreviewInput) (Preview, error) {
	response := Preview{
		Before:  []string{},
		After:   []string{},
		Added:   []string{},
		Removed: []string{},
	}
	tx := s.Store.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return response, err
	}

	defer tx.Rollback()

	p, err := s.resolve(ctx, tx)
	if err != nil {
		return response, err
	}

	for _, action := range []string{contract.PermissionIAMRolesRead, contract.PermissionIAMMembersRead} {
		if _, err = s.authorizeTx(ctx, tx, p, action, orgID); err != nil {
			return response, err
		}
	}

	if input.RoleID != "" && input.Permissions != nil && input.MembershipID == "" && input.RoleIDs == nil {
		if _, err = s.authorizeTx(ctx, tx, p, contract.PermissionIAMRolesManage, orgID); err != nil {
			return response, err
		}

		role, err := repository.RoleGrants(ctx, tx, orgID, input.RoleID)
		if err != nil {
			return response, err
		}

		if role.Kind != "custom" || role.Version != input.Version {
			return response, repository.ErrConflict
		}

		if err = s.delegation(ctx, tx, p, orgID, role.Permissions, *input.Permissions); err != nil {
			return response, err
		}

		response.Before, response.After, response.AffectedMembers = role.Permissions, *input.Permissions, role.MemberCount
	} else if input.MembershipID != "" && input.RoleIDs != nil && input.RoleID == "" && input.Permissions == nil {
		if _, err = s.authorizeTx(ctx, tx, p, contract.PermissionIAMRolesAssign, orgID); err != nil {
			return response, err
		}

		var userID uuid.UUID
		var version int64
		var memberActive bool
		if err = repository.MemberPreviewState(ctx, tx, orgID, input.MembershipID, &userID, &version, &memberActive); err != nil {
			return response, err
		}

		if input.AuthzVersion != version {
			return response, repository.ErrConflict
		}

		org, err := uuid.Parse(orgID)
		if err != nil {
			return response, repository.ErrInvalid
		}

		response.Before, err = repository.NewQueries(tx).GetOrganizationPermissions(ctx, repository.GetOrganizationPermissionsParams{OrgID: org,
			UserID: userID})
		if err != nil {
			return response, err
		}

		previous, err := repository.MemberRoleIDs(ctx, tx, orgID, input.MembershipID)
		if err != nil {
			return response, err
		}

		seen := map[string]bool{}
		for _, id := range *input.RoleIDs {
			if seen[id] {
				return response, repository.ErrInvalid
			}

			seen[id] = true
		}
		// 预览与实际写入使用相同的变更前后委派边界。
		for _, set := range [][]string{previous, *input.RoleIDs} {
			for _, id := range set {
				role, err := repository.RoleGrants(ctx, tx, orgID, id)
				if err != nil {
					return response, err
				}

				included := slices.Contains(*input.RoleIDs, id)
				if role.Kind == "system" && role.Code == "org_admin" && !p.IsSuperAdmin && slices.Contains(previous, id) != included {
					return response, authorization.ErrDenied
				}

				if included && role.Status != "active" {
					return response, repository.ErrInvalid
				}

				if role.Kind == "custom" {
					existing, candidate := role.Permissions, []string{}
					if !slices.Contains(previous, id) {
						existing = nil
					}

					if included {
						candidate = role.Permissions
					}

					if err = s.delegation(ctx, tx, p, orgID, existing, candidate); err != nil {
						return response, err
					}
				}

				if included && memberActive {
					for _, key := range role.Permissions {
						if !slices.Contains(response.After, key) {
							response.After = append(response.After, key)
						}
					}
				}
			}
		}

		response.AffectedMembers = 1
	} else {
		return response, repository.ErrInvalid
	}

	for _, key := range response.After {
		if !slices.Contains(response.Before, key) {
			response.Added = append(response.Added, key)
		}
	}

	for _, key := range response.Before {
		if !slices.Contains(response.After, key) {
			response.Removed = append(response.Removed, key)
		}
	}

	slices.Sort(response.Before)
	slices.Sort(response.After)
	slices.Sort(response.Added)
	slices.Sort(response.Removed)
	if err = tx.Commit().Error; err != nil {
		return response, err
	}

	return response, nil
}
