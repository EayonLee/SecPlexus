-- 角色是全局对象；角色权限与用户绑定都按集合替换，不做整批重写。
-- 数组参数一律用 jsonb 展开：database/sql 风味的数组绑定会让 sqlc 引入 lib/pq，
-- 与本项目的 pgx 驱动重复；切到 pgx 原生风味后可改回数组。

-- name: CreateRole :exec
INSERT INTO iam.roles(id, name, description, created_by)
VALUES (
  sqlc.arg('id')::uuid,
  sqlc.arg('name')::text,
  sqlc.arg('description')::text,
  NULLIF(sqlc.arg('created_by')::text, '')::uuid
);

-- name: GrantRolePermissions :exec
INSERT INTO iam.role_permissions(role_id, permission_key)
SELECT sqlc.arg('role_id')::uuid, value
FROM jsonb_array_elements_text(sqlc.arg('permissions')::jsonb) AS value
ON CONFLICT DO NOTHING;

-- name: RoleGrants :one
SELECT r.id, r.name, r.description, r.builtin,
       (SELECT count(*) FROM iam.user_roles ur WHERE ur.role_id = r.id) AS user_count
FROM iam.roles r
WHERE r.id = $1
FOR UPDATE;

-- name: RolePermissionKeys :many
SELECT permission_key FROM iam.role_permissions WHERE role_id = $1 ORDER BY permission_key;

-- name: UpdateRole :exec
UPDATE iam.roles SET name = $2, description = $3, updated_at = now() WHERE id = $1;

-- name: DeleteRole :exec
DELETE FROM iam.roles WHERE id = $1;

-- name: DeleteRoleGrantsNotIn :exec
DELETE FROM iam.role_permissions
WHERE role_id = sqlc.arg('role_id')::uuid
  AND permission_key NOT IN (
    SELECT value FROM jsonb_array_elements_text(sqlc.arg('keep')::jsonb) AS value
  );

-- name: TouchRole :exec
UPDATE iam.roles SET updated_at = now() WHERE id = $1;

-- name: UserRoleIDs :many
SELECT role_id FROM iam.user_roles WHERE user_id = $1 ORDER BY role_id;

-- name: DeleteUserRoles :exec
DELETE FROM iam.user_roles WHERE user_id = $1;

-- name: InsertUserRoles :exec
INSERT INTO iam.user_roles(user_id, role_id, created_by)
SELECT sqlc.arg('user_id')::uuid, value::uuid, NULLIF(sqlc.arg('created_by')::text, '')::uuid
FROM jsonb_array_elements_text(sqlc.arg('role_ids')::jsonb) AS value
ON CONFLICT DO NOTHING;
