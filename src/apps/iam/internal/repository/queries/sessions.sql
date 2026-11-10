-- 会话与凭据状态：读取按规范化用户名或主键，写入由调用方带入事务。
-- 时间一律取数据库时间（clock_timestamp），避免应用时钟影响期限判断。
-- 时长以微秒 bigint 传入并在 SQL 侧换算为区间：区间不是 database/sql 的标准类型，
-- 传 pgtype.Interval 会让持久化类型泄漏到调用方；Go 侧 Duration 的精度本来就是微秒。

-- name: GetLoginUser :one
SELECT u.id, u.username, u.display_name, u.status, u.configured, u.auth_epoch,
       c.password_hash, c.failed_attempts, c.locked_until
FROM iam.users u
JOIN iam.password_credentials c ON c.user_id = u.id
WHERE u.username_normalized = sqlc.arg('username_normalized')::text;

-- LockedAccount 锁定用户行并返回账号与访问状态；凭据不在 users 表里。
-- name: LockedAccount :one
SELECT id, username, display_name, status, configured, auth_epoch
FROM iam.users
WHERE id = sqlc.arg('id')::uuid
FOR UPDATE;

-- name: LockedSession :one
SELECT id, user_id, token_hash, auth_epoch, state, authenticated_at, reauthenticated_at,
       absolute_expires_at, idle_expires_at
FROM iam.user_sessions
WHERE id = sqlc.arg('id')::uuid
FOR UPDATE;

-- TouchSession 用取得锁后的数据库时间续期；已过期或已撤销的会话不会被重新激活。
-- name: TouchSession :one
UPDATE iam.user_sessions
SET idle_expires_at = LEAST(
      absolute_expires_at,
      GREATEST(idle_expires_at, clock_timestamp() + sqlc.arg('idle_micros')::bigint * interval '1 microsecond')
    ),
    last_seen_at = GREATEST(last_seen_at, clock_timestamp())
WHERE id = sqlc.arg('id')::uuid
  AND token_hash = sqlc.arg('token_hash')::text
  AND state = 'active'
  AND auth_epoch = sqlc.arg('auth_epoch')::bigint
  AND idle_expires_at > clock_timestamp()
  AND absolute_expires_at > clock_timestamp()
RETURNING id, user_id, token_hash, auth_epoch, state, authenticated_at, reauthenticated_at,
          absolute_expires_at, idle_expires_at;

-- name: RevokeUserSessions :exec
UPDATE iam.user_sessions
SET state = 'revoked',
    revoked_at = clock_timestamp(),
    revoke_reason = sqlc.arg('revoke_reason')::text
WHERE user_id = sqlc.arg('user_id')::uuid AND state = 'active';

-- RecordLoginFailure 按数据库时间累计失败次数，达到上限才开锁定期。
-- 锁定期已过时重新从 1 计数，锁定中的账号保持原截止时间不变。
-- name: RecordLoginFailure :exec
UPDATE iam.password_credentials
SET failed_attempts = CASE
      WHEN locked_until <= clock_timestamp() THEN 1
      ELSE failed_attempts + 1
    END,
    locked_until = CASE
      WHEN locked_until > clock_timestamp() THEN locked_until
      WHEN (CASE WHEN locked_until <= clock_timestamp() THEN 1 ELSE failed_attempts + 1 END)
        >= sqlc.arg('max_failures')::integer
        THEN clock_timestamp() + sqlc.arg('lock_micros')::bigint * interval '1 microsecond'
      ELSE NULL
    END,
    updated_at = now()
WHERE user_id = sqlc.arg('user_id')::uuid;

-- name: SessionUserID :one
SELECT user_id FROM iam.user_sessions WHERE id = sqlc.arg('id')::uuid;

-- name: HasStoredSession :one
SELECT EXISTS(
  SELECT 1 FROM iam.scs_sessions
  WHERE token = sqlc.arg('token_hash')::text AND expiry > clock_timestamp()
);

-- VerifyLoginCredential 在用户锁内再次校验凭据快照和锁定状态。
-- name: VerifyLoginCredential :one
SELECT COALESCE(
         password_hash = sqlc.arg('password_hash')::text
         AND (locked_until IS NULL OR locked_until <= clock_timestamp()),
         false
       )::bool
FROM iam.password_credentials
WHERE user_id = sqlc.arg('user_id')::uuid
FOR UPDATE;

-- name: DatabaseTime :one
SELECT clock_timestamp()::timestamptz AS now;

-- name: RevokeRotatedSession :exec
UPDATE iam.user_sessions
SET state = 'revoked',
    revoked_at = clock_timestamp(),
    revoke_reason = 'rotation'
WHERE id = sqlc.arg('id')::uuid
  AND token_hash = sqlc.arg('token_hash')::text
  AND state = 'active';

-- name: InsertSession :exec
INSERT INTO iam.user_sessions(
  id, user_id, token_hash, auth_epoch, state, authenticated_at,
  reauthenticated_at, absolute_expires_at, idle_expires_at, last_seen_at
)
VALUES (
  sqlc.arg('id')::uuid,
  sqlc.arg('user_id')::uuid,
  sqlc.arg('token_hash')::text,
  sqlc.arg('auth_epoch')::bigint,
  'active',
  sqlc.arg('authenticated_at')::timestamptz,
  sqlc.narg('reauthenticated_at')::timestamptz,
  sqlc.arg('absolute_expires_at')::timestamptz,
  sqlc.arg('idle_expires_at')::timestamptz,
  sqlc.arg('last_seen_at')::timestamptz
);

-- name: ResetLoginFailures :exec
UPDATE iam.password_credentials
SET failed_attempts = 0,
    locked_until = NULL,
    updated_at = now()
WHERE user_id = sqlc.arg('user_id')::uuid;

-- CompensateSession 撤销 SCS 提交失败的逻辑会话，禁止产生孤立登录。
-- session_id 由会话存储给出（字符串），非法时由数据库的 uuid 转换报错。
-- name: CompensateSession :exec
UPDATE iam.user_sessions
SET state = 'revoked',
    revoked_at = clock_timestamp(),
    revoke_reason = 'storage_failure'
WHERE id = sqlc.arg('session_id')::text::uuid;
