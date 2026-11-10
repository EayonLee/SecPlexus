-- +goose Up
-- 审计列表改为 (occurred_at,id) 键集分页：排序键必须由同一个复合索引支撑，
-- 否则深翻会退化为排序加扫描。单列 occurred_at 索引被复合索引完全覆盖。
CREATE INDEX IF NOT EXISTS audit_events_occurred_id ON iam.audit_events(occurred_at, id);
DROP INDEX IF EXISTS iam.audit_events_occurred;

COMMENT ON INDEX iam.audit_events_occurred_id IS
  '审计列表排序与键集分页的支撑索引：(occurred_at,id) 同时服务时间范围过滤与向前游标。';

-- 常用筛选组合（结果 + 时间）单独建索引，避免只靠排序索引回表过滤。
CREATE INDEX IF NOT EXISTS audit_events_outcome_occurred_id ON iam.audit_events(outcome, occurred_at, id);

-- +goose Down
CREATE INDEX IF NOT EXISTS audit_events_occurred ON iam.audit_events(occurred_at);
DROP INDEX IF EXISTS iam.audit_events_outcome_occurred_id;
DROP INDEX IF EXISTS iam.audit_events_occurred_id;
