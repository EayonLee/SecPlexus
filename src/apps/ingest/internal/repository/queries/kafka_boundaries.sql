-- name: CreateKafkaPartitionHead :exec
INSERT INTO ingest.kafka_partition_heads (input_id, partition, topic_id, first_offset, prepared_through)
VALUES ($1,$2,$3,$4,$4) ON CONFLICT (input_id, partition) DO NOTHING;

-- name: LockKafkaPartitionHead :one
SELECT * FROM ingest.kafka_partition_heads WHERE input_id=$1 AND partition=$2 FOR UPDATE;

-- name: AdvanceKafkaPartitionHead :exec
UPDATE ingest.kafka_partition_heads SET prepared_through = $3
WHERE input_id=$1 AND partition=$2 AND prepared_through < $3;

-- name: CreateKafkaExecutionBoundary :exec
INSERT INTO ingest.kafka_execution_boundaries
(input_id, partition, first_offset, source_revision, source_name, vendor, product, parsing_rule_id, parsing_rule_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: KafkaExecutionBoundaries :many
WITH bounds AS (
    SELECT first_offset FROM ingest.kafka_execution_boundaries
    WHERE input_id=$1 AND partition=$2 AND first_offset <= sqlc.arg(from_offset)::bigint
    ORDER BY first_offset DESC LIMIT 1
)
SELECT sqlc.embed(b), r.definition
FROM ingest.kafka_execution_boundaries b
LEFT JOIN ingest.parsing_rule_revisions r ON r.parsing_rule_id=b.parsing_rule_id AND r.version=b.parsing_rule_version
WHERE b.input_id=$1 AND b.partition=$2 AND b.first_offset BETWEEN (SELECT first_offset FROM bounds) AND sqlc.arg(to_offset)::bigint
ORDER BY b.first_offset;
