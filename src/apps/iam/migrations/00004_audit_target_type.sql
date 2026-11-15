-- +goose Up
-- 审计目标类别收紧为闭集。
--
-- 两段式加约束：先 NOT VALID（不扫既有行、不长时间持锁），再 VALIDATE（可并发、可重试）。
-- 审计表持续写入，一次性 ADD CONSTRAINT 会扫全表并阻塞写入。
--
-- 取值依据真实数据（user、session）与生产代码（platform），并预留接入点与解析映射。
-- 权威来源是 internal/enum 的 TargetType；对齐测试保证两边一致。

ALTER TABLE iam.audit_events
    ADD CONSTRAINT audit_events_target_type_check
    CHECK (target_type IN ('user','session','platform','input','mapping')) NOT VALID;

ALTER TABLE iam.audit_events VALIDATE CONSTRAINT audit_events_target_type_check;

COMMENT ON COLUMN iam.audit_events.target_type IS '审计目标类别，闭集：user 账号、session 会话、platform 平台整体、input 数据接入点、mapping 解析映射。';

-- +goose Down
ALTER TABLE iam.audit_events DROP CONSTRAINT IF EXISTS audit_events_target_type_check;
