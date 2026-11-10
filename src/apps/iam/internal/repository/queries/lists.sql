-- 列表查询只返回类型化行：HTTP JSON 由 Go 的投影类型生成，SQL 不再拼 jsonb。
-- 页内角色与权限用第二条查询按标识批量读取，避免关联行重复参与分页与计数。
-- 搜索把输入当字面量：% 与 _ 通过 ESCAPE 转义，不会变成通配符。

-- name: ListUsers :many
SELECT u.id, u.username, u.display_name, u.email, u.status, u.configured, u.auth_epoch,
       count(*) OVER() AS total
FROM iam.users u
WHERE (sqlc.arg('query')::text = ''
    OR u.username ILIKE sqlc.arg('query')::text ESCAPE E'\\'
    OR u.display_name ILIKE sqlc.arg('query')::text ESCAPE E'\\'
    OR COALESCE(u.email, '') ILIKE sqlc.arg('query')::text ESCAPE E'\\')
ORDER BY u.username, u.id
LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: UserRolesByUserIDs :many
SELECT ur.user_id, r.id AS role_id, r.name AS role_name
FROM iam.user_roles ur
JOIN iam.roles r ON r.id = ur.role_id
WHERE ur.user_id IN (SELECT value::uuid FROM jsonb_array_elements_text(sqlc.arg('user_ids')::jsonb))
ORDER BY r.name, r.id;

-- name: GetUserSummary :one
SELECT id, username, display_name, email, status, configured, auth_epoch
FROM iam.users
WHERE id = sqlc.arg('id')::uuid;

-- name: ListRoles :many
SELECT r.id, r.name, r.description, r.builtin,
       (SELECT count(*) FROM iam.user_roles ur WHERE ur.role_id = r.id) AS user_count,
       count(*) OVER() AS total
FROM iam.roles r
ORDER BY r.builtin DESC, r.name, r.id
LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: RolePermissionsByRoleIDs :many
SELECT role_id, permission_key
FROM iam.role_permissions
WHERE role_id IN (SELECT value::uuid FROM jsonb_array_elements_text(sqlc.arg('role_ids')::jsonb))
ORDER BY permission_key;

-- name: GetRoleSummary :one
SELECT r.id, r.name, r.description, r.builtin,
       (SELECT count(*) FROM iam.user_roles ur WHERE ur.role_id = r.id) AS user_count
FROM iam.roles r
WHERE r.id = sqlc.arg('id')::uuid;

-- ListAuditEvents 用 (occurred_at,id) 键集分页：审计行数随时间增长，只提供向前游标。
-- 空串参数表示不筛选；日期先经 NULLIF 转 NULL，避免空串被当作非法日期。
-- name: ListAuditEvents :many
SELECT id, occurred_at, actor_type, actor_name, action, target_type, target_id,
       outcome, reason_code, request_id, metadata
FROM iam.audit_events
WHERE (sqlc.arg('actor')::text = '' OR actor_name ILIKE sqlc.arg('actor')::text ESCAPE E'\\')
  AND (sqlc.arg('outcome')::text = '' OR outcome = sqlc.arg('outcome')::text)
  AND occurred_at >= COALESCE(NULLIF(sqlc.arg('from_date')::text, '')::date, '-infinity'::date)
  AND occurred_at < COALESCE(NULLIF(sqlc.arg('to_date')::text, '')::date + 1, 'infinity'::date)
  AND (sqlc.arg('cursor_id')::text = ''
       OR (occurred_at, id) < (sqlc.arg('cursor_at')::timestamptz, sqlc.arg('cursor_id')::text::uuid))
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::integer;
