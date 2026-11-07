-- +goose Up
-- 为审计清理加独立索引。
-- 清理语句是 WHERE occurred_at < now()-interval LIMIT 1000，现有索引 (org_id,occurred_at,id)
-- 以 org_id 打头，无法用于这个查询，导致每次清理都全表扫描。
-- 不采用按 occurred_at 分区：列表查询是 WHERE org_id=? ORDER BY occurred_at DESC 且不带时间条件，
-- 分区后该查询要扫描所有分区，反而变慢。加索引既解决清理又不动查询路径。
CREATE INDEX audit_events_occurred ON iam.audit_events(occurred_at);

-- +goose Down
DROP INDEX iam.audit_events_occurred;
