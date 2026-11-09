package service

import (
	"context"
	"strings"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// UserInput 是创建用户的请求参数。
type UserInput = model.APIUserInput

// UserUpdate 是更新用户的请求参数。
type UserUpdate = model.APIUserUpdate

// OrganizationInput 是创建组织的请求参数。
type OrganizationInput = model.APIOrganizationInput

// OrganizationUpdate 是更新组织的请求参数。
type OrganizationUpdate = model.APIOrganizationUpdate

// OrganizationUserInput 是创建组织用户的请求参数。
type OrganizationUserInput = model.APIOrganizationUserInput

// OrganizationUserUpdate 是更新组织用户的请求参数。
type OrganizationUserUpdate = model.APIOrganizationUserUpdate

// CreateUser 创建账号、用户关系与角色绑定；三者同一事务，任一失败全部回滚。
// 账号必须属于一个组织：不允许创建无组织用户。
func (s *Service) CreateUser(ctx context.Context, input UserInput, requestID string) (string, error) {
	normalized, err := identity.NormalizeUsername(input.Username)
	if err != nil {
		return "", repository.ErrInvalid
	}

	if err = identity.ValidateUsernameAvailable(normalized); err != nil {
		return "", repository.ErrInvalid
	}

	displayName, err := identity.ValidateDisplayName(input.DisplayName)
	if err != nil {
		return "", repository.ErrInvalid
	}

	if err = identity.ValidateEmail(input.Email); err != nil {
		return "", repository.ErrInvalid
	}

	orgID, err := uuid.Parse(input.OrgID)
	if err != nil {
		return "", repository.ErrInvalid
	}

	hash, err := (identity.PasswordHasher{}).GenerateHash(input.Password)
	if err != nil {
		return "", repository.ErrInvalid
	}

	id := uuid.New()
	err = s.Manage(
		ctx,
		iamclient.PermissionIAMPlatformUsersManage,
		"",
		requestID,
		"user",
		&id,
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			metadata["org_id"] = input.OrgID

			input.DisplayName = displayName
			input.Email = identity.NormalizeEmail(input.Email)
			if err := insertUser(ctx, tx, id, input, normalized, hash); err != nil {
				return err
			}

			organizationUserID := uuid.New()
			if err := repository.InsertOrganizationUser(ctx, tx, organizationUserID, input.OrgID, id); err != nil {
				return err
			}

			// 角色必须属于同一组织，且授予范围不得超过操作者自身权限。
			// 允许空角色：创建组织时先建账号再把它设为组织管理员，
			// 此时该账号在目标组织内尚无角色可授。
			if len(input.RoleIDs) > 0 {
				if err := s.assignRolesWithinScope(ctx, tx, p, orgID, organizationUserID, input.RoleIDs); err != nil {
					return err
				}
			}

			return nil
		},
	)
	return id.String(), err
}

// assignRolesWithinScope 校验角色归属组织并复用委派上限，再写入用户角色关系。
func (s *Service) assignRolesWithinScope(ctx context.Context, tx *gorm.DB, p iamclient.Principal, orgID uuid.UUID, organizationUserID uuid.UUID, roleIDs []string) error {
	catalog, err := repository.Permissions(ctx, tx)
	if err != nil {
		return err
	}

	granted := []string{}
	for _, raw := range roleIDs {
		roleID, err := uuid.Parse(raw)
		if err != nil {
			return repository.ErrInvalid
		}

		role, err := repository.RoleGrants(ctx, tx, orgID.String(), roleID.String())
		if err != nil {
			return repository.ErrInvalid
		}

		granted = append(granted, role.Permissions...)
	}

	effective, err := repository.NewQueries(tx).GetOrganizationPermissions(ctx, repository.GetOrganizationPermissionsParams{OrgID: orgID, UserID: uuid.MustParse(p.UserID)})
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

	if err = authorization.CheckDelegation(effective, nil, granted, catalog); err != nil {
		return err
	}

	for _, raw := range roleIDs {
		roleID, err := uuid.Parse(raw)
		if err != nil {
			return repository.ErrInvalid
		}

		if err = tx.WithContext(ctx).Exec(`INSERT INTO iam.organization_user_roles(org_id,organization_user_id,role_id,created_by) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, orgID, organizationUserID, roleID, p.UserID).Error; err != nil {
			return err
		}
	}

	return nil
}

// insertUser 在当前管理事务内创建资料与密码。
func insertUser(ctx context.Context, tx *gorm.DB, id uuid.UUID, input UserInput, normalized, hash string) error {
	if err := repository.InsertUser(ctx, tx, id, strings.TrimSpace(input.Username), normalized, input.DisplayName, input.Email); err != nil {
		return err
	}

	err := repository.InsertPassword(ctx, tx, id, hash)
	return err
}

// UpdateUser 更新用户信息并检查版本。
func (s *Service) UpdateUser(ctx context.Context, userID string, input UserUpdate, requestID string) error {
	displayName, err := identity.ValidateDisplayName(input.DisplayName)
	if err != nil {
		return repository.ErrInvalid
	}

	if err = identity.ValidateEmail(input.Email.GetOrZero()); err != nil {
		return repository.ErrInvalid
	}

	input.DisplayName = displayName
	input.Email = omit.Map(input.Email, identity.NormalizeEmail)

	return s.Manage(
		ctx,
		iamclient.PermissionIAMPlatformUsersManage,
		"",
		requestID,
		"user",
		uuidPointer(userID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			id, err := uuid.Parse(userID)
			if err != nil {
				return repository.ErrInvalid
			}

			target, err := repository.NewQueries(tx).LockMutableUser(ctx, id)
			if err != nil {
				return err
			}

			metadata["status"] = input.Status
			if target.AuthEpoch != input.AuthEpoch {
				return repository.ErrConflict
			}

			if err = repository.UpdateUser(ctx, tx, id, input.DisplayName, input.Email, input.Status); err != nil {
				return err
			}

			if err = repository.NewQueries(tx).RevokeUserSessions(ctx, repository.RevokeUserSessionsParams{UserID: id,
				RevokeReason: text("account_change")}); err != nil {
				return err
			}

			return repository.ProtectPlatformAdministrator(ctx, tx)
		},
	)
}

// ResetPassword 重置密码并撤销相关会话。
func (s *Service) ResetPassword(ctx context.Context, userID, password, requestID string) error {
	hash, err := (identity.PasswordHasher{}).GenerateHash(password)
	if err != nil {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		iamclient.PermissionIAMPlatformUsersManage,
		"",
		requestID,
		"user",
		uuidPointer(userID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			id, err := uuid.Parse(userID)
			if err != nil {
				return repository.ErrInvalid
			}

			if _, err = repository.NewQueries(tx).LockMutableUser(ctx, id); err != nil {
				return err
			}

			if err = repository.BumpUserAuthEpoch(ctx, tx, id); err != nil {
				return err
			}

			if err = repository.ResetPassword(ctx, tx, id, hash); err != nil {
				return err
			}

			return repository.NewQueries(tx).RevokeUserSessions(ctx, repository.RevokeUserSessionsParams{UserID: id,
				RevokeReason: text("password_reset")})
		},
	)
}
