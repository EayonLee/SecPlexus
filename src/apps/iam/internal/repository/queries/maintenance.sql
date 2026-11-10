-- 保留期清理与操作员恢复：都分批执行，避免长事务与锁等待。
-- 保留期以微秒 bigint 传入并在 SQL 侧换算为区间（区间不是 database/sql 的标准类型）。

-- name: PurgeExpiredStoredSessions :exec
DELETE FROM iam.scs_sessions
WHERE token IN (
  SELECT token FROM iam.scs_sessions WHERE expiry < clock_timestamp() LIMIT 1000
);

-- name: PurgeExpiredSessions :exec
DELETE FROM iam.user_sessions
WHERE id IN (
  SELECT id FROM iam.user_sessions
  WHERE absolute_expires_at < clock_timestamp() - sqlc.arg('retention_micros')::bigint * interval '1 microsecond'
  LIMIT 1000
);

-- AllowAuditPurge 在本次事务内显式开闸：审计默认只允许追加，删除只能走保留期清理。
-- name: AllowAuditPurge :exec
SELECT set_config('iam.audit_purge', 'on', true);

-- name: PurgeExpiredAuditEvents :exec
DELETE FROM iam.audit_events
WHERE id IN (
  SELECT id FROM iam.audit_events
  WHERE occurred_at < clock_timestamp() - sqlc.arg('retention_micros')::bigint * interval '1 microsecond'
  LIMIT 1000
);

-- LockAllUsers 按主键顺序锁定全部账号行，保证恢复时没有并发写入。
-- name: LockAllUsers :exec
SELECT id FROM iam.users ORDER BY id FOR UPDATE;

-- name: BumpAllUserAuthEpochs :exec
UPDATE iam.users SET auth_epoch = auth_epoch + 1, updated_at = now();

-- name: RevokeAllActiveSessions :exec
UPDATE iam.user_sessions
SET state = 'revoked',
    revoked_at = clock_timestamp(),
    revoke_reason = sqlc.arg('revoke_reason')::text
WHERE state = 'active';

-- name: ClearStoredSessions :exec
DELETE FROM iam.scs_sessions;
