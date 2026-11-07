-- +goose Up
-- 把两个"版本号"列改成不会被看混的名字。
-- auth_version 与 authz_version 只差一个字母，含义却完全不同：
--   auth_epoch  = 账号维度，改动它会让该用户所有会话立即失效（改密、停用、撤超管）。
--   authz_epoch = 组织维度，改动它表示该组织的授权结果发生变化（改角色、改成员绑定）。
-- 沿用旧名时改错一处，结果就是"改个角色把所有人踢下线"或"改密后权限没刷新"。
ALTER TABLE iam.users RENAME COLUMN auth_version TO auth_epoch;
ALTER TABLE iam.user_sessions RENAME COLUMN auth_version TO auth_epoch;
ALTER TABLE iam.organizations RENAME COLUMN authz_version TO authz_epoch;

-- +goose Down
ALTER TABLE iam.organizations RENAME COLUMN authz_epoch TO authz_version;
ALTER TABLE iam.user_sessions RENAME COLUMN auth_epoch TO auth_version;
ALTER TABLE iam.users RENAME COLUMN auth_epoch TO auth_version;
