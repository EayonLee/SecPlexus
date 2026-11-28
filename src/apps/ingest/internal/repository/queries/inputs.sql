-- name: CreateInput :one
INSERT INTO ingest.inputs (id,name,vendor,product,protocol,parsing_rule_id,configuration)
VALUES (gen_random_uuid(),$1,$2,$3,$4,$5,$6) RETURNING *;

-- name: LockInputRuleVersion :one
SELECT version FROM ingest.parsing_rules WHERE id = $1 FOR SHARE;

-- name: UpdateInput :one
UPDATE ingest.inputs SET name = coalesce(sqlc.narg(name),name), vendor = coalesce(sqlc.narg(vendor),vendor),
product = coalesce(sqlc.narg(product),product), enabled = coalesce(sqlc.narg(enabled),enabled),
parsing_rule_id = CASE WHEN sqlc.arg(change_mapping)::boolean THEN sqlc.narg(parsing_rule_id)::uuid ELSE parsing_rule_id END,
configuration = CASE WHEN sqlc.narg(digest)::text IS NULL THEN configuration
                     ELSE jsonb_set(configuration, '{token_sha256}', to_jsonb(sqlc.narg(digest)::text)) END,
revision = revision + 1,
credential_generation = credential_generation + CASE WHEN sqlc.narg(digest)::text IS NULL THEN 0 ELSE 1 END,
updated_at = now()
WHERE id = sqlc.arg(id) AND (sqlc.narg(digest)::text IS NULL OR protocol = 'http') RETURNING *;

-- name: LoadSources :many
SELECT s.id,s.name,s.vendor,s.product,s.protocol,s.configuration,s.parsing_rule_id,s.revision,s.credential_generation,s.enabled,s.created_at,coalesce(r.version,0)::integer AS parsing_rule_version,r.definition
FROM ingest.inputs s LEFT JOIN ingest.parsing_rules r ON r.id = s.parsing_rule_id ORDER BY s.name;
