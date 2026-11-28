-- name: InsertAuditOutbox :exec
INSERT INTO ingest.audit_outbox (id, event) VALUES ($1, $2);

-- name: ListAuditOutbox :many
SELECT id, event, queued_at FROM ingest.audit_outbox WHERE rejected_at IS NULL
ORDER BY queued_at, id LIMIT 50;

-- name: DeleteAuditOutbox :exec
DELETE FROM ingest.audit_outbox WHERE id = $1;

-- name: AuditOutboxHealth :one
SELECT count(*) AS depth,
       coalesce(extract(epoch FROM now() - min(queued_at))::bigint, 0)::bigint AS oldest_seconds
FROM (SELECT queued_at FROM ingest.audit_outbox WHERE rejected_at IS NULL ORDER BY queued_at LIMIT 10001) AS sample;

-- name: RejectAuditOutbox :exec
UPDATE ingest.audit_outbox SET rejected_at = now() WHERE id = $1 AND rejected_at IS NULL;
