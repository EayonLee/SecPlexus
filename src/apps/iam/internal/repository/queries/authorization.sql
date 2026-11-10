-- 授权只有一条路径：user_roles → role_permissions → permissions，没有作用域或继承。

-- name: LockManagement :exec
SELECT pg_advisory_xact_lock($1);

-- name: ListPermissions :many
SELECT permission_key, description, risk_level, delegable, requires_recent_auth, status
FROM iam.permissions
ORDER BY permission_key;

-- name: ReadPermission :one
SELECT permission_key, description, risk_level, delegable, requires_recent_auth, status
FROM iam.permissions
WHERE permission_key = $1;

-- name: EffectivePermissions :many
SELECT DISTINCT p.permission_key
FROM iam.user_roles ur
JOIN iam.role_permissions rp ON rp.role_id = ur.role_id
JOIN iam.permissions p ON p.permission_key = rp.permission_key AND p.status = 'active'
WHERE ur.user_id = $1
ORDER BY p.permission_key;

-- name: UserRoles :many
SELECT r.id, r.name
FROM iam.user_roles ur
JOIN iam.roles r ON r.id = ur.role_id
WHERE ur.user_id = $1
ORDER BY r.name, r.id;

-- name: BuiltinRoleID :one
SELECT id FROM iam.roles WHERE builtin;

-- name: ActiveSuperAdminExistsExcept :one
SELECT EXISTS(
  SELECT 1
  FROM iam.user_roles ur
  JOIN iam.roles r ON r.id = ur.role_id AND r.builtin
  JOIN iam.users u ON u.id = ur.user_id AND u.status = 'active'
  WHERE ur.user_id <> $1
);
