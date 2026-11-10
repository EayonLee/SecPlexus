-- 账号与凭据的写入路径。email 的空串在 SQL 侧转 NULL：空串不是合法邮箱，也不是合法唯一键。
-- status 一律由调用方从 internal/enum 传入，SQL 不重复声明取值集合。

-- name: InsertUser :exec
INSERT INTO iam.users(id, username, username_normalized, display_name, email, status, configured)
VALUES (
  sqlc.arg('id')::uuid,
  sqlc.arg('username')::text,
  sqlc.arg('username_normalized')::text,
  sqlc.arg('display_name')::text,
  NULLIF(sqlc.arg('email')::text, ''),
  sqlc.arg('status')::text,
  sqlc.arg('configured')::bool
);

-- name: InsertPassword :exec
INSERT INTO iam.password_credentials(user_id, password_hash)
VALUES (sqlc.arg('user_id')::uuid, sqlc.arg('password_hash')::text);

-- UpdateUser 更新资料与状态。email 只在调用方给出时改写（空串转 NULL）；
-- auth_epoch 只在访问状态变化时递增：改显示名或邮箱不影响会话，改状态才让旧会话失效。
-- name: UpdateUser :exec
UPDATE iam.users
SET display_name = sqlc.arg('display_name')::text,
    status = sqlc.arg('status')::text,
    email = CASE
      WHEN sqlc.arg('update_email')::bool THEN NULLIF(sqlc.arg('email')::text, '')
      ELSE email
    END,
    auth_epoch = CASE
      WHEN sqlc.arg('access_changed')::bool THEN auth_epoch + 1
      ELSE auth_epoch
    END,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- name: UpdateUserTimeZone :exec
UPDATE iam.users
SET timezone = NULLIF(sqlc.arg('timezone')::text, ''),
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- name: ReadUserTimeZone :one
SELECT timezone FROM iam.users WHERE id = sqlc.arg('id')::uuid;

-- name: BumpUserAuthEpoch :exec
UPDATE iam.users
SET auth_epoch = auth_epoch + 1,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid;

-- name: ResetPassword :exec
UPDATE iam.password_credentials
SET password_hash = sqlc.arg('password_hash')::text,
    changed_at = now(),
    failed_attempts = 0,
    locked_until = NULL,
    updated_at = now()
WHERE user_id = sqlc.arg('user_id')::uuid;

-- name: DeleteUser :exec
DELETE FROM iam.users WHERE id = sqlc.arg('id')::uuid;
