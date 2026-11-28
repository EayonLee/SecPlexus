-- name: ReadInput :one
SELECT sqlc.embed(i), coalesce(r.version,0)::integer AS parsing_rule_version
FROM ingest.inputs i LEFT JOIN ingest.parsing_rules r ON r.id=i.parsing_rule_id WHERE i.id=$1;

-- name: ListInputPage :many
SELECT sqlc.embed(i), coalesce(r.version,0)::integer AS parsing_rule_version, count(*) OVER () AS total
FROM ingest.inputs i LEFT JOIN ingest.parsing_rules r ON r.id=i.parsing_rule_id
WHERE (sqlc.arg(q)::text = '' OR strpos(lower(concat_ws(' ', i.name, i.vendor, i.product)), lower(sqlc.arg(q))) > 0)
  AND (sqlc.arg(protocol)::text = '' OR i.protocol = sqlc.arg(protocol))
  AND (sqlc.narg(enabled)::boolean IS NULL OR i.enabled = sqlc.narg(enabled))
  AND (sqlc.narg(parsing_rule_id)::uuid IS NULL OR i.parsing_rule_id = sqlc.narg(parsing_rule_id))
  AND (sqlc.arg(exact_name)::text = '' OR i.name_normalized = lower(btrim(sqlc.arg(exact_name))))
  AND (sqlc.narg(syslog_port)::text IS NULL OR (i.protocol = 'syslog' AND i.configuration->>'port' = sqlc.narg(syslog_port)))
ORDER BY i.name_normalized
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListMappingPage :many
SELECT id, name, vendor, product, version, builtin, created_at, updated_at,
       coalesce(definition->>'description', '') ::text AS description,
       coalesce(definition#>>'{parse,mode}', '') ::text AS parse_type,
       classes.class_uids,
       count(*) OVER () AS total
FROM ingest.parsing_rules
CROSS JOIN LATERAL (
    SELECT coalesce(nullif(ARRAY(
        SELECT DISTINCT (branch->>'class_uid')::integer
        FROM jsonb_array_elements(
            CASE WHEN jsonb_typeof(definition->'branches') = 'array' THEN definition->'branches' ELSE '[]'::jsonb END
        ) AS branch
        WHERE (branch->>'class_uid')::integer > 0
        ORDER BY 1
    ), ARRAY[]::integer[]), ARRAY[0])::integer[] AS class_uids
) AS classes
WHERE (sqlc.arg(q)::text = '' OR strpos(lower(concat_ws(' ', name, vendor, product, definition->>'description')), lower(sqlc.arg(q))) > 0)
  AND (sqlc.arg(parse_type)::text = '' OR definition#>>'{parse,mode}' = sqlc.arg(parse_type))
  AND (sqlc.narg(class_uid)::integer IS NULL OR sqlc.narg(class_uid) = ANY(classes.class_uids))
  AND (sqlc.narg(builtin)::boolean IS NULL OR builtin = sqlc.narg(builtin))
  AND (coalesce(cardinality(sqlc.arg(ids)::uuid[]), 0) = 0 OR id = ANY(sqlc.arg(ids)::uuid[]))
ORDER BY name_normalized
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: MappingInputCounts :many
SELECT parsing_rule_id, count(*) AS input_count
FROM ingest.inputs WHERE parsing_rule_id = ANY(sqlc.arg(ids)::uuid[])
GROUP BY parsing_rule_id;
