-- +goose Up
-- 补全审计目标类别：角色操作此前漏在闭集之外。
--
-- 00004 的闭集来自"数据库已有值 + 部分代码路径"，漏掉了 roles 服务写入的 role，
-- 结果是角色创建在 CI 上被 CHECK 拒绝。这说明闭集必须枚举**全部生产写入路径**，
-- 而不只是历史数据里出现过的值。
--
-- 这里用前向迁移而不是回改 00004：已应用迁移不可修改。

ALTER TABLE iam.audit_events DROP CONSTRAINT IF EXISTS audit_events_target_type_check;

ALTER TABLE iam.audit_events
    ADD CONSTRAINT audit_events_target_type_check
    CHECK (target_type IN ('user','session','role','platform','input','mapping')) NOT VALID;

ALTER TABLE iam.audit_events VALIDATE CONSTRAINT audit_events_target_type_check;

COMMENT ON COLUMN iam.audit_events.target_type IS '审计目标类别，闭集：user 账号、session 会话、role 角色、platform 平台整体、input 数据接入点、mapping 解析映射。';

-- +goose Down
ALTER TABLE iam.audit_events DROP CONSTRAINT IF EXISTS audit_events_target_type_check;
-- 回滚到 00004 定义的较小闭集（仅在无 role 数据时安全）。
ALTER TABLE iam.audit_events
    ADD CONSTRAINT audit_events_target_type_check
    CHECK (target_type IN ('user','session','platform','input','mapping')) NOT VALID;
