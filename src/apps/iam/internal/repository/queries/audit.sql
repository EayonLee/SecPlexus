-- name: InsertAuditEvent :exec
INSERT INTO iam.audit_events (
    id, actor_type, actor_id, actor_name, action, target_type, target_id,
    outcome, reason_code, request_id, decision_id, metadata, occurred_at
) VALUES (
    sqlc.arg('id'), sqlc.arg('actor_type'), sqlc.narg('actor_id'), sqlc.arg('actor_name'),
    sqlc.arg('action'), sqlc.arg('target_type'), sqlc.narg('target_id'), sqlc.arg('outcome'),
    sqlc.arg('reason_code'), sqlc.arg('request_id'), sqlc.narg('decision_id'), sqlc.arg('metadata'),
    COALESCE(sqlc.narg('occurred_at')::timestamptz, now())
)
ON CONFLICT (id) DO NOTHING;
