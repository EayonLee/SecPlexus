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

// CreateMember 在组织中创建成员关系。
func (s *Service) CreateMember(ctx context.Context, orgID string, input MemberInput, requestID string) (string, error) {
	var hash, normalized string
	var err error
	if input.UserID == "" {
		normalized, err = identity.NormalizeUsername(input.Username)
		if err != nil || !validName(input.DisplayName) || !validEmail(input.Email) {
			return "", repository.ErrInvalid
		}

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

	membershipID := uuid.New()
	err = s.Manage(
		ctx,
		iamclient.PermissionIAMMembersManage,
		orgID,
		requestID,
		"membership",
		&membershipID,
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

			if err := repository.InsertMembership(ctx, tx, membershipID, orgID, userID); err != nil {
				return err
			}

			err := repository.AssignMemberRole(ctx, tx, orgID, membershipID, p.UserID)
			return err
		},
		existingUser,
	)
	return membershipID.String(), err
}

// UpdateMember 更新或移除组织成员。
func (s *Service) UpdateMember(ctx context.Context, orgID, memberID string, input MemberUpdate, remove bool, requestID string) error {

	return s.Manage(
		ctx,
		iamclient.PermissionIAMMembersManage,
		orgID,
		requestID,
		"membership",
		uuidPointer(memberID),
		func(tx *gorm.DB, p iamclient.Principal, metadata map[string]any) error {
			var version int64
			if err := repository.LockOrganizationVersion(ctx, tx, orgID, &version); err != nil {
				return err
			}

			if version != input.AuthzVersion {
				return repository.ErrConflict
			}

			var admin bool
			var userID uuid.UUID
			if err := repository.LockMembershipAdministrator(ctx, tx, orgID, memberID, &userID, &admin); err != nil {
				return err
			}

			if admin && !p.IsSuperAdmin {
				return authorization.ErrDenied
			}

			if remove {
				if err := repository.DeleteMemberRoles(ctx, tx, orgID, memberID); err != nil {
					return err
				}

				if err := repository.DeleteMembership(ctx, tx, orgID, memberID); err != nil {
					return err
				}
			} else {
				if err := repository.UpdateMembership(ctx, tx, orgID, memberID, input.Status); err != nil {
					return err
				}
			}

			return repository.ProtectLastAdministrators(ctx, tx)
		},
	)
}
