package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// InsertUser 创建用户资料，不对外暴露密码记录。
// configured 标记由启动配置写入的内置管理员，数据库只允许一个。
func InsertUser(ctx context.Context, db DB, id uuid.UUID, value string, normalized string, displayName string, email string, configured bool) error {
	err := queries(db).InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:                 id,
		Username:           value,
		UsernameNormalized: normalized,
		DisplayName:        displayName,
		Email:              email,
		Status:             string(enum.UserStatusActive),
		Configured:         configured,
	})

	return mapUniqueViolation(err)
}

// mapUniqueViolation 按约束名区分用户名与邮箱冲突。
// 通用 23505 映射会落到 conflict，用户无法知道该改哪一项。
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}

	switch pgErr.ConstraintName {
	case "users_username_normalized_key":
		return ErrUsernameTaken
	case "users_email_normalized_key":
		return ErrEmailTaken
	}

	return err
}

// InsertPassword 为新账号写入密码哈希。
func InsertPassword(ctx context.Context, db DB, id uuid.UUID, hash string) error {
	return queries(db).InsertPassword(ctx, sqlcgen.InsertPasswordParams{UserID: id, PasswordHash: hash})
}

// UpdateUser 更新资料与状态；只有访问状态变化才递增账号版本。
// auth_epoch 是「账号版本」：改显示名称或邮箱不影响会话，改状态才会让旧会话失效。
// email 未给出时不改写库里已有的邮箱，空串则存为 NULL。
func UpdateUser(ctx context.Context, db DB, id uuid.UUID, displayName string, email omit.Val[string], status enum.UserStatus, accessChanged bool) error {
	params := sqlcgen.UpdateUserParams{
		ID:            id,
		DisplayName:   displayName,
		Status:        string(status),
		UpdateEmail:   email.IsValue(),
		AccessChanged: accessChanged,
	}

	if email.IsValue() {
		params.Email = email.MustGet()
	}

	return queries(db).UpdateUser(ctx, params)
}

// UpdateTimeZone 写入本人展示时区偏好；空串存为 NULL 表示未设置。
// 该偏好只影响渲染，不递增 auth_epoch，也不撤销会话。
func UpdateTimeZone(ctx context.Context, db DB, id uuid.UUID, timeZone string) error {
	return queries(db).UpdateUserTimeZone(ctx, sqlcgen.UpdateUserTimeZoneParams{ID: id, Timezone: timeZone})
}

// BumpUserAuthEpoch 增加账号版本，使旧会话失效。
func BumpUserAuthEpoch(ctx context.Context, db DB, id uuid.UUID) error {
	return queries(db).BumpUserAuthEpoch(ctx, id)
}

// ResetPassword 更新密码哈希，同时清除失败和锁定状态。
func ResetPassword(ctx context.Context, db DB, id uuid.UUID, hash string) error {
	return queries(db).ResetPassword(ctx, sqlcgen.ResetPasswordParams{UserID: id, PasswordHash: hash})
}

// UserTimeZone 读取本人展示时区偏好；未设置时返回空串。
// 账号在会话读取之后被删除时同样按未设置处理，让该项偏好不影响本人资料读取。
func UserTimeZone(ctx context.Context, db DB, id uuid.UUID) (string, error) {
	value, err := queries(db).ReadUserTimeZone(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", err
	}

	if !value.Valid {
		return "", nil
	}

	return value.String, nil
}

// DeleteUser 删除账号。凭据、会话与角色绑定由 users 的级联删除清理，
// 历史创建人引用由 ON DELETE SET NULL 置空，不阻塞删除。
func DeleteUser(ctx context.Context, db DB, id uuid.UUID) error {
	return queries(db).DeleteUser(ctx, id)
}
