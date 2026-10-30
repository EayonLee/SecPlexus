package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
)

// SyncAdministrator 在已迁移的数据库中应用唯一管理员配置；不创建或修改表结构。
func (s *Store) SyncAdministrator(ctx context.Context, username, password string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockManagement(ctx, tx, ManagementLock); err != nil {
			return err
		}
		_, err := syncAdministrator(ctx, tx, username, password)
		return err
	})
}

// syncAdministrator 在管理锁内同步配置身份，配置不变时不轮换哈希或会话版本。
func syncAdministrator(ctx context.Context, tx *gorm.DB, username, password string) (uuid.UUID, error) {
	username = strings.TrimSpace(username)
	normalized, err := identity.NormalizeUsername(username)
	if err != nil {
		return uuid.Nil, err
	}
	if err = identity.ValidatePassword(password); err != nil {
		return uuid.Nil, err
	}

	reason := pgtype.Text{String: "administrator_configuration", Valid: true}
	q := NewQueries(tx)
	user, err := q.GetLoginUser(ctx, normalized)
	created := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !created {
		return uuid.Nil, err
	}
	if created {
		user.ID = uuid.New()
		if err = InsertUser(ctx, tx, user.ID, username, normalized, username, ""); err != nil {
			return uuid.Nil, err
		}
	}

	// 先更新用户行再撤销会话，保持与登录/会话提交一致的加锁顺序。
	var demoted []struct{ ID uuid.UUID }
	if err = tx.WithContext(ctx).Raw(`UPDATE iam.users SET is_super_admin=false,auth_version=auth_version+1,updated_at=now() WHERE is_super_admin AND id<>? RETURNING id`, user.ID).Scan(&demoted).Error; err != nil {
		return uuid.Nil, err
	}
	for _, previous := range demoted {
		if err = q.RevokeUserSessions(ctx, RevokeUserSessionsParams{UserID: previous.ID, RevokeReason: reason}); err != nil {
			return uuid.Nil, err
		}
	}

	hasher := identity.PasswordHasher{}
	passwordChanged := created || hasher.CompareHashAndPassword(user.PasswordHash, password) != nil
	if !passwordChanged && user.IsSuperAdmin && user.Status == "active" && user.Username == username {
		return user.ID, nil
	}
	if err = tx.WithContext(ctx).Table("iam.users").Where("id = ?", user.ID).Updates(map[string]any{
		"username": username, "display_name": username, "email": nil, "status": "active", "is_super_admin": true,
		"auth_version": gorm.Expr("auth_version+1"), "updated_at": gorm.Expr("now()"),
	}).Error; err != nil {
		return uuid.Nil, err
	}
	if passwordChanged {
		hash, err := hasher.GenerateHash(password)
		if err != nil {
			return uuid.Nil, err
		}
		if created {
			err = InsertPassword(ctx, tx, user.ID, hash)
		} else {
			err = ResetPassword(ctx, tx, user.ID, hash)
		}
		if err != nil {
			return uuid.Nil, err
		}
	}
	if err = q.RevokeUserSessions(ctx, RevokeUserSessionsParams{UserID: user.ID, RevokeReason: reason}); err != nil {
		return uuid.Nil, err
	}
	if err = AppendAudit(ctx, tx, Audit{ActorType: "deployment_operator", Action: "iam.administrator.configure", TargetType: "user", TargetID: &user.ID, RequestID: uuid.NewString()}); err != nil {
		return uuid.Nil, err
	}
	return user.ID, nil
}

// LockMutableUser 取得账号行锁，并拒绝应用及运维密码重置对配置账号的修改。
func (q *Queries) LockMutableUser(ctx context.Context, id uuid.UUID) (LoginUser, error) {
	user, err := q.LockUser(ctx, id)
	if err != nil {
		return user, err
	}
	if user.IsSuperAdmin {
		return user, authorization.ErrDenied
	}
	return user, nil
}
