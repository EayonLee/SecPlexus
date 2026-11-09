-- +goose Up
-- 组织内的人一律叫「用户」，不再叫「成员」。
-- 表、列、约束、索引、权限键同步改名，不保留旧名与视图映射。

ALTER TABLE iam.membership_roles RENAME TO organization_user_roles;
ALTER TABLE iam.organization_user_roles RENAME COLUMN membership_id TO organization_user_id;
ALTER TABLE iam.memberships RENAME TO organization_users;

-- 约束随表改名保留原名，这里逐个纠正，避免 schema 里残留 membership 字样。
ALTER TABLE iam.organization_user_roles
  RENAME CONSTRAINT membership_roles_pkey TO organization_user_roles_pkey;
ALTER TABLE iam.organization_user_roles
  RENAME CONSTRAINT membership_roles_created_by_fkey TO organization_user_roles_created_by_fkey;
ALTER TABLE iam.organization_user_roles
  RENAME CONSTRAINT membership_roles_org_id_membership_id_fkey TO organization_user_roles_org_id_organization_user_id_fkey;
ALTER TABLE iam.organization_user_roles
  RENAME CONSTRAINT membership_roles_org_id_role_id_fkey TO organization_user_roles_org_id_role_id_fkey;
ALTER INDEX iam.membership_roles_role RENAME TO organization_user_roles_role;

ALTER TABLE iam.organization_users
  RENAME CONSTRAINT memberships_pkey TO organization_users_pkey;
ALTER TABLE iam.organization_users
  RENAME CONSTRAINT memberships_org_id_user_id_key TO organization_users_org_id_user_id_key;
ALTER TABLE iam.organization_users
  RENAME CONSTRAINT memberships_org_id_id_key TO organization_users_org_id_id_key;
ALTER TABLE iam.organization_users
  RENAME CONSTRAINT memberships_org_id_fkey TO organization_users_org_id_fkey;
ALTER TABLE iam.organization_users
  RENAME CONSTRAINT memberships_user_id_fkey TO organization_users_user_id_fkey;
ALTER TABLE iam.organization_users
  RENAME CONSTRAINT memberships_status_check TO organization_users_status_check;
ALTER INDEX iam.memberships_user_status RENAME TO organization_users_user_status;

-- 权限键：组织级 members 改名为 users，与平台级 platform.users 靠 scope 区分。
-- role_permissions 上有触发器校验 permission_key 必须指向一个 active 的组织权限，
-- 因此必须先把新键插进目录，再迁移授权，最后删旧键；直接 UPDATE 键名会被触发器拒绝。
INSERT INTO iam.permissions (permission_key, module, resource_type, action, scope, description, risk_level, delegable, requires_recent_auth, status, created_at, updated_at)
SELECT 'iam.users.read', module, 'users', action, scope, '查看组织用户', risk_level, delegable, requires_recent_auth, status, created_at, updated_at
FROM iam.permissions WHERE permission_key = 'iam.members.read'
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO iam.permissions (permission_key, module, resource_type, action, scope, description, risk_level, delegable, requires_recent_auth, status, created_at, updated_at)
SELECT 'iam.users.manage', module, 'users', action, scope, '管理组织用户', risk_level, delegable, requires_recent_auth, status, created_at, updated_at
FROM iam.permissions WHERE permission_key = 'iam.members.manage'
ON CONFLICT (permission_key) DO NOTHING;

UPDATE iam.role_permissions SET permission_key = 'iam.users.read' WHERE permission_key = 'iam.members.read';
UPDATE iam.role_permissions SET permission_key = 'iam.users.manage' WHERE permission_key = 'iam.members.manage';

DELETE FROM iam.permissions WHERE permission_key IN ('iam.members.read', 'iam.members.manage');

COMMENT ON TABLE iam.organization_users IS
  '组织内的用户关系。一个用户在一个组织内最多一行；授权经由 organization_user_roles。';

-- +goose Down
-- 改名已改变外键与约束标识，回滚会造成与既有部署不一致的状态；
-- 显式拒绝，要求从备份恢复。
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'cannot roll back member-to-user rename: restore from backup instead';
END $$;
-- +goose StatementEnd
