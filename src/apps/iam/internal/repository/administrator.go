package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// administrator 是配置托管账号在同步时的内部快照。
type administrator struct {
	ID           uuid.UUID       // 账号标识。
	Username     string          // 当前用户名。
	Status       enum.UserStatus // 当前启用状态，取值见 enum.UserStatus。
	AuthEpoch    int64           // 当前账号版本。
	PasswordHash string          // 当前密码哈希，仅用于判断配置是否变化。
}

// syncAdministrator 在管理锁内同步配置的托管账号。
// 用户名为配置权威来源：改名会收回旧账号的内置标记并创建或接管目标账号，
// 配置不变时不轮换哈希或会话版本。
func syncAdministrator(ctx context.Context, tx *sql.Tx, username, password string, builtinRoleID uuid.UUID) (uuid.UUID, error) {
	username = strings.TrimSpace(username)
	normalized, err := account.NormalizeUsername(username)
	if err != nil {
		return uuid.Nil, err
	}
	if err = account.ValidatePassword(password); err != nil {
		return uuid.Nil, err
	}

	reason := "administrator_configuration"
	gen := queries(tx)
	hasher := account.PasswordHasher{}

	current, err := configuredAdministrator(ctx, tx)
	hasCurrent := true
	if errors.Is(err, sql.ErrNoRows) {
		hasCurrent = false
	} else if err != nil {
		return uuid.Nil, err
	}

	target, err := administratorByUsername(ctx, tx, normalized)
	exists := true
	if errors.Is(err, sql.ErrNoRows) {
		exists = false
	} else if err != nil {
		return uuid.Nil, err
	}

	// 配置改名时先收回旧账号的内置标记，保证任何时刻只有一个配置托管账号。
	if hasCurrent && current.ID != target.ID {
		if err = gen.DemoteConfiguredAdministrator(ctx, current.ID); err != nil {
			return uuid.Nil, err
		}

		if err = gen.DeleteUserRole(ctx, sqlcgen.DeleteUserRoleParams{UserID: current.ID, RoleID: builtinRoleID}); err != nil {
			return uuid.Nil, err
		}

		if err = RevokeUserSessions(ctx, tx, RevokeUserSessionsParams{UserID: current.ID, RevokeReason: reason}); err != nil {
			return uuid.Nil, err
		}

		if err = AppendAudit(ctx, tx, Audit{ActorType: enum.ActorTypeDeploymentOperator, Action: "iam.administrator.demote", TargetType: "user", TargetID: &current.ID, RequestID: uuid.NewString()}); err != nil {
			return uuid.Nil, err
		}
	}

	created, changed := false, false
	if !exists {
		target.ID = uuid.New()
		created = true
		if err = InsertUser(ctx, tx, target.ID, username, normalized, username, "", true); err != nil {
			return uuid.Nil, err
		}

		hash, hashErr := hasher.GenerateHash(password)
		if hashErr != nil {
			return uuid.Nil, hashErr
		}

		if err = InsertPassword(ctx, tx, target.ID, hash); err != nil {
			return uuid.Nil, err
		}

		target.Username = username
		target.Status = enum.UserStatusActive
	} else {
		passwordChanged := target.PasswordHash == "" || hasher.CompareHashAndPassword(target.PasswordHash, password) != nil
		changed = passwordChanged || target.Username != username || target.Status != enum.UserStatusActive || !hasCurrent || current.ID != target.ID
		if changed {
			if err = gen.PromoteConfiguredAdministrator(ctx, sqlcgen.PromoteConfiguredAdministratorParams{
				ID:       target.ID,
				Username: username,
				Status:   string(enum.UserStatusActive),
			}); err != nil {
				return uuid.Nil, err
			}

			if passwordChanged {
				hash, hashErr := hasher.GenerateHash(password)
				if hashErr != nil {
					return uuid.Nil, hashErr
				}

				if target.PasswordHash == "" {
					err = InsertPassword(ctx, tx, target.ID, hash)
				} else {
					err = ResetPassword(ctx, tx, target.ID, hash)
				}

				if err != nil {
					return uuid.Nil, err
				}
			}

			if err = RevokeUserSessions(ctx, tx, RevokeUserSessionsParams{UserID: target.ID, RevokeReason: reason}); err != nil {
				return uuid.Nil, err
			}
		}
	}

	if created || changed {
		if err = AppendAudit(ctx, tx, Audit{ActorType: enum.ActorTypeDeploymentOperator, Action: "iam.administrator.configure", TargetType: "user", TargetID: &target.ID, RequestID: uuid.NewString()}); err != nil {
			return uuid.Nil, err
		}
	}

	// 配置账号始终持有内置角色；重复启动只补缺失的绑定。
	if err = gen.EnsureUserRole(ctx, sqlcgen.EnsureUserRoleParams{UserID: target.ID, RoleID: builtinRoleID}); err != nil {
		return uuid.Nil, err
	}

	return target.ID, nil
}

// configuredAdministrator 读取当前配置托管账号及其密码哈希。
func configuredAdministrator(ctx context.Context, tx DB) (administrator, error) {
	row, err := queries(tx).ConfiguredAdministrator(ctx)
	if err != nil {
		return administrator{}, err
	}

	return administrator{
		ID:           row.ID,
		Username:     row.Username,
		Status:       row.Status,
		AuthEpoch:    row.AuthEpoch,
		PasswordHash: row.PasswordHash,
	}, nil
}

// administratorByUsername 按规范化用户名读取账号；不存在时返回 sql.ErrNoRows。
func administratorByUsername(ctx context.Context, tx DB, normalized string) (administrator, error) {
	row, err := queries(tx).AdministratorByUsername(ctx, normalized)
	if err != nil {
		return administrator{}, err
	}

	return administrator{
		ID:           row.ID,
		Username:     row.Username,
		Status:       row.Status,
		AuthEpoch:    row.AuthEpoch,
		PasswordHash: row.PasswordHash,
	}, nil
}
