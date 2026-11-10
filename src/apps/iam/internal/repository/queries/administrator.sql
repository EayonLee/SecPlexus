-- 配置托管管理员的启动同步。用户名为配置权威来源；
-- 读取走 LEFT JOIN，让还没有密码记录的账号也能用于比较。

-- name: ConfiguredAdministrator :one
SELECT u.id, u.username, u.status, u.auth_epoch, COALESCE(c.password_hash, '') AS password_hash
FROM iam.users u
LEFT JOIN iam.password_credentials c ON c.user_id = u.id
WHERE u.configured;

-- name: AdministratorByUsername :one
SELECT u.id, u.username, u.status, u.auth_epoch, COALESCE(c.password_hash, '') AS password_hash
FROM iam.users u
LEFT JOIN iam.password_credentials c ON c.user_id = u.id
WHERE u.username_normalized = sqlc.arg('username_normalized')::text;

-- DemoteConfiguredAdministrator 收回旧账号的内置标记：配置改名时先降级再接管，
-- 保证任何时刻只有一个配置托管账号（users_single_configured 唯一索引）。
-- name: DemoteConfiguredAdministrator :exec
UPDATE iam.users
SET configured = false,
    auth_epoch = auth_epoch + 1,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- PromoteConfiguredAdministrator 接管已存在的账号：用户名即显示名，清空邮箱。
-- status 由调用方传入 enum.UserStatus，SQL 不重复声明取值集合。
-- name: PromoteConfiguredAdministrator :exec
UPDATE iam.users
SET username = sqlc.arg('username')::text,
    display_name = sqlc.arg('username')::text,
    email = NULL,
    status = sqlc.arg('status')::text,
    configured = true,
    auth_epoch = auth_epoch + 1,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- name: DeleteUserRole :exec
DELETE FROM iam.user_roles
WHERE user_id = sqlc.arg('user_id')::uuid AND role_id = sqlc.arg('role_id')::uuid;

-- name: EnsureUserRole :exec
INSERT INTO iam.user_roles(user_id, role_id, created_by)
VALUES (sqlc.arg('user_id')::uuid, sqlc.arg('role_id')::uuid, sqlc.arg('user_id')::uuid)
ON CONFLICT DO NOTHING;
