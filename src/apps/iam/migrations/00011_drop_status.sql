-- +goose Up
-- 角色与组织不再有启用/停用状态：未删除即可用。成员仍保留 active/disabled。
-- 项目未发版，按清空重建处理：直接删列，不做兼容。
ALTER TABLE iam.roles DROP CONSTRAINT roles_status_check;
ALTER TABLE iam.roles DROP COLUMN status;

ALTER TABLE iam.organizations DROP CONSTRAINT organizations_status_check;
ALTER TABLE iam.organizations DROP COLUMN status;

-- kind 的约束原本同时引用 status，重建为只校验 kind 与系统角色编码。
ALTER TABLE iam.roles DROP CONSTRAINT roles_kind_check;
ALTER TABLE iam.roles ADD CONSTRAINT roles_kind_check CHECK(
  (kind = 'system' AND code IN ('org_admin','member'))
  OR (kind = 'custom' AND code NOT IN ('org_admin','member','super_admin'))
);

-- +goose Down
ALTER TABLE iam.roles DROP CONSTRAINT roles_kind_check;
ALTER TABLE iam.roles ADD COLUMN status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled'));
ALTER TABLE iam.roles ADD CONSTRAINT roles_kind_check CHECK(
  (kind = 'system' AND code IN ('org_admin','member'))
  OR (kind = 'custom' AND code NOT IN ('org_admin','member','super_admin'))
);

ALTER TABLE iam.organizations ADD COLUMN status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended'));
