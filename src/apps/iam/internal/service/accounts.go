package service

import (
	"context"
	"database/sql"
	"strings"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// CreateUser 创建账号、密码与可选角色绑定；任一失败全部回滚。
func (s *Service) CreateUser(ctx context.Context, input UserCreate, requestID string) (string, error) {
	normalized, err := account.NormalizeUsername(input.Username)
	if err != nil {
		return "", repository.ErrInvalid
	}

	if err = account.ValidateUsernameAvailable(normalized); err != nil {
		return "", repository.ErrInvalid
	}

	displayName, err := account.ValidateDisplayName(input.DisplayName)
	if err != nil {
		return "", repository.ErrInvalid
	}

	if err = account.ValidateEmail(input.Email); err != nil {
		return "", repository.ErrInvalid
	}

	hash, err := (account.PasswordHasher{}).GenerateHash(input.Password)
	if err != nil {
		return "", repository.ErrInvalid
	}

	input.DisplayName = displayName
	input.Email = account.NormalizeEmail(input.Email)
	id := uuid.New()
	err = s.Manage(
		ctx,
		iamclient.PermissionIAMUsersManage,
		requestID,
		"user",
		&id,
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			metadata["username"] = input.Username
			if err := insertUser(ctx, tx, id, input, normalized, hash); err != nil {
				return err
			}

			if len(input.RoleIDs) > 0 {
				return s.assignRoles(ctx, tx, p, id, input.RoleIDs)
			}

			return nil
		},
	)
	return id.String(), err
}

// insertUser 在当前管理事务内创建资料与密码。
func insertUser(ctx context.Context, tx *sql.Tx, id uuid.UUID, input UserCreate, normalized, hash string) error {
	if err := repository.InsertUser(ctx, tx, id, strings.TrimSpace(input.Username), normalized, input.DisplayName, input.Email, false); err != nil {
		return err
	}

	return repository.InsertPassword(ctx, tx, id, hash)
}

// assignRoles 复用委派上限校验并写入用户角色绑定。
func (s *Service) assignRoles(ctx context.Context, tx *sql.Tx, p iamclient.Principal, userID uuid.UUID, roleIDs []string) error {
	granted, err := grantsFor(ctx, tx, roleIDs)
	if err != nil {
		return err
	}

	if err = s.delegation(ctx, tx, p, nil, granted); err != nil {
		return err
	}

	return repository.ReplaceUserRoles(ctx, tx, userID.String(), roleIDs, p.UserID)
}

// UpdateUser 更新用户信息；配置托管账号不能被停用。
func (s *Service) UpdateUser(ctx context.Context, userID string, input UserUpdate, requestID string) error {
	displayName, err := account.ValidateDisplayName(input.DisplayName)
	if err != nil {
		return repository.ErrInvalid
	}

	if err = account.ValidateEmail(input.Email.GetOrZero()); err != nil {
		return repository.ErrInvalid
	}

	input.DisplayName = displayName
	input.Email = omit.Map(input.Email, account.NormalizeEmail)

	return s.Manage(
		ctx,
		iamclient.PermissionIAMUsersManage,
		requestID,
		"user",
		uuidPointer(userID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			id, err := uuid.Parse(userID)
			if err != nil {
				return repository.ErrInvalid
			}

			target, err := repository.LockedAccount(ctx, tx, id)
			if err != nil {
				return err
			}

			status := input.Status
			// 配置托管管理员由部署配置决定是否启用，页面停用会与下次启动的配置同步冲突。
			if target.Configured && status == enum.UserStatusDisabled {
				return repository.ErrConfiguredUserImmutable
			}

			if status == enum.UserStatusDisabled && target.Status != enum.UserStatusDisabled && p.UserID == userID {
				return repository.ErrSelfTarget
			}

			if status == enum.UserStatusDisabled && target.Status != enum.UserStatusDisabled {
				if err = s.requireOtherSuperAdmin(ctx, tx, id); err != nil {
					return err
				}
			}

			statusChanged := target.Status != status
			metadata["status"] = string(status)
			if err = repository.UpdateUser(ctx, tx, id, input.DisplayName, input.Email, status, statusChanged); err != nil {
				return err
			}

			// 状态变化已通过账号版本让旧会话失效；这里再写明撤销原因，便于审计与清理。
			if statusChanged {
				return repository.RevokeUserSessions(ctx, tx, repository.RevokeUserSessionsParams{UserID: id,
					RevokeReason: "account_status"})
			}

			return nil
		},
	)
}

// DeleteUser 删除用户资料、角色绑定与会话；配置托管账号不可删除。
func (s *Service) DeleteUser(ctx context.Context, userID, requestID string) error {
	return s.Manage(
		ctx,
		iamclient.PermissionIAMUsersManage,
		requestID,
		"user",
		uuidPointer(userID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			id, err := uuid.Parse(userID)
			if err != nil {
				return repository.ErrInvalid
			}

			target, err := repository.LockedAccount(ctx, tx, id)
			if err != nil {
				return err
			}

			if target.Configured {
				return repository.ErrConfiguredUserImmutable
			}

			if p.UserID == userID {
				return repository.ErrSelfTarget
			}

			if err = s.requireOtherSuperAdmin(ctx, tx, id); err != nil {
				return err
			}

			metadata["username"] = target.Username
			return repository.DeleteUser(ctx, tx, id)
		},
	)
}

// ResetPassword 管理员重置密码并撤销目标用户的所有会话。
func (s *Service) ResetPassword(ctx context.Context, userID, password, requestID string) error {
	hash, err := (account.PasswordHasher{}).GenerateHash(password)
	if err != nil {
		return repository.ErrInvalid
	}

	return s.Manage(
		ctx,
		iamclient.PermissionIAMUsersReset,
		requestID,
		"user",
		uuidPointer(userID),
		func(tx *sql.Tx, p iamclient.Principal, metadata map[string]any) error {
			id, err := uuid.Parse(userID)
			if err != nil {
				return repository.ErrInvalid
			}

			target, err := repository.LockedAccount(ctx, tx, id)
			if err != nil {
				return err
			}

			// 配置托管账号的密码只能由配置更新，页面重置会在下次启动被覆盖。
			if target.Configured {
				return repository.ErrConfiguredUserImmutable
			}

			if err = repository.BumpUserAuthEpoch(ctx, tx, id); err != nil {
				return err
			}

			if err = repository.ResetPassword(ctx, tx, id, hash); err != nil {
				return err
			}

			return repository.RevokeUserSessions(ctx, tx, repository.RevokeUserSessionsParams{UserID: id,
				RevokeReason: "password_reset"})
		},
	)
}
