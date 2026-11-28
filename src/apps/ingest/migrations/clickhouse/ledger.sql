CREATE TABLE IF NOT EXISTS spx.event_schema_migrations
(
    version UInt32 COMMENT 'Applied Event schema migration version',
    checksum String COMMENT 'SHA-256 of the applied migration source',
    applied_at DateTime64(3, 'UTC') DEFAULT now64(3) COMMENT 'Time the verified migration was recorded, in UTC'
)
ENGINE = ReplacingMergeTree
ORDER BY (version, checksum)
COMMENT 'Verified Event schema migrations';
