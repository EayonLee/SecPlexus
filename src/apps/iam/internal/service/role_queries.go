package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// Roles 返回角色和当前用户可执行的操作；实际写入时仍须在 Manage 中重新鉴权。
func (s *Service) Roles(ctx context.Context, orgID, roleID string, limit, offset int) ([]contract.APIRole, int64, error) {
	tx := s.Store.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return nil, 0, err
	}

	defer tx.Rollback()

	p, err := s.resolve(ctx, tx)
	if err != nil {
		return nil, 0, err
	}

	if _, err = s.authorizeTx(ctx, tx, p, contract.PermissionIAMRolesRead, orgID); err != nil {
		return nil, 0, err
	}

	canManage := true
	if _, err = s.authorizeTx(ctx, tx, p, contract.PermissionIAMRolesManage, orgID); err != nil {
		if errors.Is(err, authorization.ErrDenied) {
			canManage = false
		} else {
			return nil, 0, err
		}
	}

	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, 0, repository.ErrInvalid
	}

	user, err := uuid.Parse(p.UserID)
	if err != nil {
		return nil, 0, err
	}

	effective, err := repository.NewQueries(tx).GetOrganizationPermissions(ctx, repository.GetOrganizationPermissionsParams{OrgID: org,
		UserID: user})
	if err != nil {
		return nil, 0, err
	}

	catalog, err := repository.Permissions(ctx, tx)
	if err != nil {
		return nil, 0, err
	}

	if p.IsSuperAdmin {
		for _, permission := range catalog {
			if permission.Scope == "organization" {
				effective = append(effective, permission.Key)
			}
		}
	}

	var target any
	if roleID != "" {
		id, err := uuid.Parse(roleID)
		if err != nil {
			return nil, 0, repository.ErrInvalid
		}

		target = id
	}

	items, total, err := repository.ListRoles(ctx, tx, org, target, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	for i := range items {
		role := &items[i]
		role.AllowedActions = []string{contract.PermissionIAMRolesRead}
		if canManage && role.Kind == "custom" && authorization.CheckDelegation(effective, role.Permissions, role.Permissions, catalog) == nil {
			role.AllowedActions = append(role.AllowedActions, contract.PermissionIAMRolesManage)
		}
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
