package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// PreviewInput 描述拟预览的角色或用户权限变更。
type PreviewInput = model.APIPreviewInput

// Preview 描述权限变更前后的差异及影响范围。
type Preview = model.APIPreview

// Preview 比较角色权限或用户角色变更前后的权限。
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

	for _, action := range []string{iamclient.PermissionIAMRolesRead, iamclient.PermissionIAMUsersRead} {
		if _, err = s.authorizeTx(ctx, tx, p, action, orgID); err != nil {
			return response, err
		}
	}

	if input.RoleID != "" && input.Permissions != nil && input.OrganizationUserID == "" && input.RoleIDs == nil {
		if _, err = s.authorizeTx(ctx, tx, p, iamclient.PermissionIAMRolesManage, orgID); err != nil {
			return response, err
		}

		role, err := repository.RoleGrants(ctx, tx, orgID, input.RoleID)
		if err != nil {
			return response, err
		}

		if role.Version != input.Version {
			return response, repository.ErrConflict
		}

		if err = s.delegation(ctx, tx, p, orgID, role.Permissions, *input.Permissions); err != nil {
			return response, err
		}

		response.Before, response.After, response.AffectedUsers = role.Permissions, *input.Permissions, role.UserCount
	} else if input.OrganizationUserID != "" && input.RoleIDs != nil && input.RoleID == "" && input.Permissions == nil {
		if _, err = s.authorizeTx(ctx, tx, p, iamclient.PermissionIAMRolesAssign, orgID); err != nil {
			return response, err
		}

		var userID uuid.UUID
		var version int64
		var organizationUserActive, systemAdmin bool
		if err = repository.OrganizationUserPreviewState(ctx, tx, orgID, input.OrganizationUserID, &userID, &version, &organizationUserActive, &systemAdmin); err != nil {
			return response, err
		}

		if systemAdmin {
			return response, authorization.ErrDenied
		}

		if input.AuthzEpoch != version {
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

		previous, err := repository.OrganizationUserRoleIDs(ctx, tx, orgID, input.OrganizationUserID)
		if err != nil {
			return response, err
		}

		// 预览与实际写入使用相同的变更前后委派边界。
		for _, set := range [][]string{previous, *input.RoleIDs} {
			for _, id := range set {
				role, err := repository.RoleGrants(ctx, tx, orgID, id)
				if err != nil {
					return response, err
				}

				included := slices.Contains(*input.RoleIDs, id)
				{
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

				if included && organizationUserActive {
					for _, key := range role.Permissions {
						if !slices.Contains(response.After, key) {
							response.After = append(response.After, key)
						}
					}
				}
			}
		}

		response.AffectedUsers = 1
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
