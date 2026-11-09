package service

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// CreateOrganizationUser 在组织中创建用户关系。
func (s *Service) CreateOrganizationUser(ctx context.Context, orgID string, input OrganizationUserInput, requestID string) (string, error) {
	var hash, normalized string
	var err error
	if input.UserID == "" {
		normalized, err = identity.NormalizeUsername(input.Username)
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

		input.DisplayName = displayName
		input.Email = identity.NormalizeEmail(input.Email)

		hash, err = (identity.PasswordHasher{}).GenerateHash(input.Password)
		if err != nil {
			return "", repository.ErrInvalid
		}
	} else if input.Username != "" || input.Password != "" || input.DisplayName != "" || input.Email != "" {
		return "", repository.ErrInvalid
	}

	existingUser := uuid.Nil
	if input.UserID != "" {
		existingUser, err = uuid.Parse(input.UserID)
		if err != nil {
			return "", repository.ErrInvalid
		}
	}

	organizationUserID := uuid.New()
	err = s.Manage(
		ctx,
		iamclient.PermissionIAMUsersManage,
		orgID,
		requestID,
		"membership",
		&organizationUserID,
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			userID := uuid.New()
			if input.UserID != "" {
				if !p.IsSuperAdmin {
					return authorization.ErrDenied
				}

				var err error
				userID, err = uuid.Parse(input.UserID)
				if err != nil {
					return repository.ErrInvalid
				}

				user, err := repository.NewQueries(tx).LockUser(ctx, userID)
				if err != nil {
					return err
				}

				if user.Status != "active" {
					return repository.ErrInvalid
				}
			} else if err := insertUser(ctx, tx, userID, UserInput{
				Username:    input.Username,
				DisplayName: input.DisplayName,
				Email:       input.Email,
				Password:    input.Password,
			}, normalized, hash); err != nil {
				return err
			}

			if err := repository.InsertOrganizationUser(ctx, tx, organizationUserID, orgID, userID); err != nil {
				return err
			}

			// 新建用户不自动获得任何角色：权限完全由使用者按需授予。
			return nil
		},
		existingUser,
	)
	return organizationUserID.String(), err
}

// UpdateOrganizationUser 更新或移除组织用户。
func (s *Service) UpdateOrganizationUser(ctx context.Context, orgID, organizationUserID string, input OrganizationUserUpdate, remove bool, requestID string) error {

	return s.Manage(
		ctx,
		iamclient.PermissionIAMUsersManage,
		orgID,
		requestID,
		"membership",
		uuidPointer(organizationUserID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			var version int64
			if err := repository.LockOrganizationVersion(ctx, tx, orgID, &version); err != nil {
				return err
			}

			if version != input.AuthzEpoch {
				return repository.ErrConflict
			}

			var userID uuid.UUID
			if err := repository.LockOrganizationUser(ctx, tx, orgID, organizationUserID, &userID); err != nil {
				return err
			}

			if _, err := repository.NewQueries(tx).LockMutableUser(ctx, userID); err != nil {
				return err
			}

			if remove {
				if err := repository.DeleteOrganizationUserRoles(ctx, tx, orgID, organizationUserID); err != nil {
					return err
				}

				if err := repository.DeleteOrganizationUsership(ctx, tx, orgID, organizationUserID); err != nil {
					return err
				}
			} else {
				if err := repository.UpdateOrganizationUsership(ctx, tx, orgID, organizationUserID, input.Status); err != nil {
					return err
				}
			}

			return nil
		},
	)
}
