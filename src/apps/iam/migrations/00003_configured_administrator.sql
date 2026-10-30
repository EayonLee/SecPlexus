-- +goose Up
-- 旧的可分配超管身份转为配置托管；保留账号，撤销其旧身份会话。
UPDATE iam.user_sessions SET state='revoked',revoked_at=clock_timestamp(),revoke_reason='administrator_configuration'
WHERE state='active' AND user_id IN (SELECT id FROM iam.users WHERE is_super_admin);
UPDATE iam.users SET is_super_admin=false,auth_version=auth_version+1,updated_at=now() WHERE is_super_admin;
CREATE UNIQUE INDEX users_single_super_admin ON iam.users (is_super_admin) WHERE is_super_admin;
UPDATE iam.permissions SET status='retired',updated_at=now()
WHERE key IN ('iam.platform.super_admins.manage','iam.platform.security.read');

-- +goose Down
DROP INDEX iam.users_single_super_admin;
-- 回退索引不恢复历史超管授权；管理员身份仍须由运维显式确认。
