-- name: CreateMapping :one
INSERT INTO ingest.parsing_rules (id,name,vendor,product,definition,verification,author_id)
VALUES (gen_random_uuid(),$1,$2,$3,$4,sqlc.narg(verification),sqlc.narg(author_id)) RETURNING *;

-- name: UpdateMapping :one
UPDATE ingest.parsing_rules SET name=$2, vendor=$3, product=$4, definition=$5, verification=sqlc.narg(verification),
author_id=sqlc.narg(author_id), version=version+1, updated_at=now()
WHERE id=$1 AND NOT builtin AND version = sqlc.arg(expected_version)::integer RETURNING *;

-- name: ReadMapping :one
SELECT * FROM ingest.parsing_rules WHERE id=$1;

-- name: ListMappingRevisions :many
SELECT parsing_rule_id, version, name, vendor, product, recorded_at, verification, author_id
FROM ingest.parsing_rule_revisions
WHERE parsing_rule_id=$1 AND (sqlc.narg(before_version)::integer IS NULL OR version < sqlc.narg(before_version))
ORDER BY version DESC
LIMIT sqlc.arg(page_limit);

-- name: ReadMappingRevision :one
SELECT * FROM ingest.parsing_rule_revisions WHERE parsing_rule_id=$1 AND version=$2;
