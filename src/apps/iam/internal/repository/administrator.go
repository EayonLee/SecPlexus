package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
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
func syncAdministrator(ctx context.Context, tx *gorm.DB, username, password string, builtinRoleID uuid.UUID) (uuid.UUID, error) {
	username = strings.TrimSpace(username)
	normalized, err := account.NormalizeUsername(username)
	if err != nil {
		return uuid.Nil, err
	}
	if err = account.ValidatePassword(password); err != nil {
		return uuid.Nil, err
	}

	reason := pgtype.Text{String: "administrator_configuration", Valid: true}
	q := NewQueries(tx)
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
		if err = tx.WithContext(ctx).Table("iam.users").Where("id = ?", current.ID).Updates(map[string]any{
			"configured": false,
			"auth_epoch": gorm.Expr("auth_epoch+1"),
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return uuid.Nil, err
		}

		if err = tx.WithContext(ctx).Table("iam.user_roles").Where("user_id = ? AND role_id = ?", current.ID, builtinRoleID).Delete(nil).Error; err != nil {
			return uuid.Nil, err
		}

		if err = q.RevokeUserSessions(ctx, RevokeUserSessionsParams{UserID: current.ID, RevokeReason: reason}); err != nil {
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
			if err = tx.WithContext(ctx).Table("iam.users").Where("id = ?", target.ID).Updates(map[string]any{
				"username":     username,
				"display_name": username,
				"email":        nil,
				"status":       string(enum.UserStatusActive),
				"configured":   true,
				"auth_epoch":   gorm.Expr("auth_epoch+1"),
				"updated_at":   gorm.Expr("now()"),
			}).Error; err != nil {
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

			if err = q.RevokeUserSessions(ctx, RevokeUserSessionsParams{UserID: target.ID, RevokeReason: reason}); err != nil {
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
	if err = tx.WithContext(ctx).Exec(`INSERT INTO iam.user_roles(user_id,role_id,created_by) VALUES($1,$2,$1) ON CONFLICT DO NOTHING`, target.ID, builtinRoleID).Error; err != nil {
		return uuid.Nil, err
	}

	return target.ID, nil
}

// configuredAdministrator 读取当前配置托管账号及其密码哈希。
func configuredAdministrator(ctx context.Context, tx *gorm.DB) (administrator, error) {
	var row administrator
	err := tx.WithContext(ctx).Raw(`
		SELECT u.id,u.username,u.status,u.auth_epoch,COALESCE(c.password_hash,'')
		FROM iam.users u LEFT JOIN iam.password_credentials c ON c.user_id=u.id
		WHERE u.configured`).Row().Scan(&row.ID, &row.Username, &row.Status, &row.AuthEpoch, &row.PasswordHash)
	return row, err
}

// administratorByUsername 按规范化用户名读取账号；不存在时返回 sql.ErrNoRows。
func administratorByUsername(ctx context.Context, tx *gorm.DB, normalized string) (administrator, error) {
	var row administrator
	err := tx.WithContext(ctx).Raw(`
		SELECT u.id,u.username,u.status,u.auth_epoch,COALESCE(c.password_hash,'')
		FROM iam.users u LEFT JOIN iam.password_credentials c ON c.user_id=u.id
		WHERE u.username_normalized=$1`, normalized).Row().Scan(&row.ID, &row.Username, &row.Status, &row.AuthEpoch, &row.PasswordHash)
	return row, err
}
