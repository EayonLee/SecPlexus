package service

import (
	"context"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

// MemberInput 是创建组织成员的请求参数。
type MemberInput = model.APIMemberInput

// MemberUpdate 是更新组织成员的请求参数。
type MemberUpdate = model.APIMemberUpdate

// validEmail 校验账号邮件地址的业务约束。
func validEmail(value string) bool {
	if value == "" {
		return true
	}

	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && len(value) <= 320
}

// CreateUser 创建用户并写入审计记录。
func (s *Service) CreateUser(ctx context.Context, input UserInput, requestID string) (string, error) {
	normalized, err := identity.NormalizeUsername(input.Username)
	if err != nil || !validName(input.DisplayName) || !validEmail(input.Email) {
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
			return insertUser(ctx, tx, id, input, normalized, hash)
		},
	)
	return id.String(), err
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
	if !validName(input.DisplayName) || !validEmail(input.Email.GetOrZero()) {
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

			target, err := repository.NewQueries(tx).LockMutableUser(ctx, id)
			if err != nil {
				return err
			}

			metadata["status"] = input.Status
			if target.AuthVersion != input.AuthVersion {
				return repository.ErrConflict
			}

			if err = repository.UpdateUser(ctx, tx, id, input.DisplayName, input.Email, input.Status); err != nil {
				return err
			}

			if err = repository.NewQueries(tx).RevokeUserSessions(ctx, repository.RevokeUserSessionsParams{UserID: id,
				RevokeReason: text("account_change")}); err != nil {
				return err
			}

			return repository.ProtectLastAdministrators(ctx, tx)
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

			if err = repository.BumpUserAuthVersion(ctx, tx, id); err != nil {
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
