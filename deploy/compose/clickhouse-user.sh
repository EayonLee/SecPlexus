#!/bin/sh
set -eu

# The service accounts are created with access scoped to the event table.
for identifier in "$CLICKHOUSE_DB" "$SPX_INGEST_CLICKHOUSE_USERNAME" "$SPX_INGEST_CLICKHOUSE_READER_USERNAME"; do
  case "$identifier" in
    ''|[0-9]*|*[!a-zA-Z0-9_]*) printf '%s\n' 'Invalid ClickHouse database or writer name' >&2; exit 1 ;;
  esac
done

if [ "$CLICKHOUSE_USER" = "$SPX_INGEST_CLICKHOUSE_USERNAME" ] ||
   [ "$CLICKHOUSE_USER" = "$SPX_INGEST_CLICKHOUSE_READER_USERNAME" ] ||
   [ "$SPX_INGEST_CLICKHOUSE_USERNAME" = "$SPX_INGEST_CLICKHOUSE_READER_USERNAME" ]; then
  printf '%s\n' 'ClickHouse migration, writer and reader require distinct accounts' >&2
  exit 1
fi

writer_hash=$(printf '%s' "$SPX_INGEST_CLICKHOUSE_PASSWORD" | sha256sum | cut -d ' ' -f 1)
reader_hash=$(printf '%s' "$SPX_INGEST_CLICKHOUSE_READER_PASSWORD" | sha256sum | cut -d ' ' -f 1)
clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --multiquery \
  --param_writer="$SPX_INGEST_CLICKHOUSE_USERNAME" --param_writer_hash="$writer_hash" \
  --param_reader="$SPX_INGEST_CLICKHOUSE_READER_USERNAME" --param_reader_hash="$reader_hash" <<SQL
CREATE USER IF NOT EXISTS {writer:Identifier} IDENTIFIED WITH sha256_hash BY {writer_hash:String};
CREATE USER IF NOT EXISTS {reader:Identifier} IDENTIFIED WITH sha256_hash BY {reader_hash:String};
GRANT SELECT, INSERT ON "$CLICKHOUSE_DB".events TO "$SPX_INGEST_CLICKHOUSE_USERNAME";
GRANT SELECT ON "$CLICKHOUSE_DB".events TO "$SPX_INGEST_CLICKHOUSE_READER_USERNAME";
SQL
