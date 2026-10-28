package service

import (
	"context"
	"regexp"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// CreateOrganization 创建组织及其初始管理员。
func (s *Service) CreateOrganization(ctx context.Context, input OrganizationInput, requestID string) (string, error) {
	admin, err := uuid.Parse(input.AdminUserID)
	if err != nil || !validName(input.Name) || !slugPattern.MatchString(input.Slug) {
		return "", repository.ErrInvalid
	}

	id := uuid.New()
	err = s.Manage(
		ctx,
		contract.PermissionIAMPlatformOrganizationsManage,
		"",
		requestID,
		"organization",
		&id,
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			user, err := repository.NewQueries(tx).LockUser(ctx, admin)
			if err != nil {
				return err
			}

			if user.Status != "active" {
				return repository.ErrInvalid
			}

			actor, err := uuid.Parse(p.UserID)
			if err != nil {
				return err
			}

			return repository.CreateOrganization(ctx, tx, id, input.Slug, input.Name, admin, actor)
		},
		admin,
	)
	return id.String(), err
}

// UpdateOrganization 更新组织信息。
func (s *Service) UpdateOrganization(ctx context.Context, orgID string, input OrganizationUpdate, platform bool, requestID string) error {
	if !validName(input.Name) || input.AuthzVersion < 1 {
		return repository.ErrInvalid
	}

	action, targetOrg := contract.PermissionIAMOrganizationsManage, orgID
	if platform {
		action, targetOrg = contract.PermissionIAMPlatformOrganizationsManage, ""
		if input.Status != "active" && input.Status != "suspended" {
			return repository.ErrInvalid
		}
	} else if input.Status != "" {
		return authorization.ErrDenied
	}

	return s.Manage(
		ctx,
		action,
		targetOrg,
		requestID,
		"organization",
		uuidPointer(orgID),
		func(tx *gorm.DB, p contract.Principal, metadata map[string]any) error {
			var version int64
			if err := repository.LockOrganizationVersion(ctx, tx, orgID, &version); err != nil {
				return err
			}

			if version != input.AuthzVersion {
				return repository.ErrConflict
			}

			status := input.Status
			if err := repository.UpdateOrganization(ctx, tx, orgID, input.Name, status, platform); err != nil {
				return err
			}

			return repository.ProtectLastAdministrators(ctx, tx)
		},
	)
}
