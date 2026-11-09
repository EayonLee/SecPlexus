-- +goose Up
-- 角色只保留 id、名称、说明、权限。删除 code 与 kind：
-- code 对使用者没有价值（关联一律用 UUID），kind 只是内置/自定义的二分，
-- 而内置角色本身随「组织必须有管理员」这条不变量一并移除。

ALTER TABLE iam.roles DROP CONSTRAINT IF EXISTS roles_kind_check;
ALTER TABLE iam.roles DROP CONSTRAINT IF EXISTS roles_check;
ALTER TABLE iam.roles DROP CONSTRAINT IF EXISTS roles_code_check;
ALTER TABLE iam.roles DROP CONSTRAINT IF EXISTS roles_org_id_code_key;

ALTER TABLE iam.roles DROP COLUMN code;
ALTER TABLE iam.roles DROP COLUMN kind;

COMMENT ON TABLE iam.roles IS
  '组织角色。只有 id、name、description 与权限；不再区分内置/自定义，也不再有业务代码。';

-- +goose Down
-- code 与 kind 的取值已随本次迁移丢弃，无法还原；重建列只会得到空值并让
-- 依赖 code 的授权逻辑失去意义。这里显式拒绝回滚，要求从备份恢复。
-- +goose StatementBegin
DO $$
BEGIN
  RAISE EXCEPTION 'cannot roll back roles code/kind removal: original values are lost; restore from backup instead';
END $$;
-- +goose StatementEnd
