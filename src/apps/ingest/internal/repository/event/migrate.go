package event

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/migrations/clickhouse"
)

// Migrate 初始化首版事件表。重试只登记已经核对通过的表结构。
func (s *Store) Migrate(ctx context.Context) error {
	if _, _, err := loadProjection(); err != nil {
		return err
	}
	ddl, err := EventTableMigration()
	if err != nil {
		return err
	}
	if _, err := s.query(ctx, "CREATE DATABASE IF NOT EXISTS "+s.database, nil); err != nil {
		return fmt.Errorf("create Event database: %w", err)
	}
	if _, err = s.query(ctx, clickhouse.LedgerSQL, nil); err != nil {
		return fmt.Errorf("create Event migration ledger: %w", err)
	}

	if err := s.applyMigration(ctx, 1, clickhouse.EventTableTemplate+"\x00"+clickhouse.EventTableColumnsV1,
		strings.Replace(ddl, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1),
		func() error { return s.CheckSchema(ctx) }); err != nil {
		return err
	}
	return s.CheckSchema(ctx)
}

func (s *Store) applyMigration(ctx context.Context, version int, source, ddl string, check func() error) error {
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	prior, err := s.query(ctx, fmt.Sprintf("SELECT DISTINCT checksum FROM spx.event_schema_migrations WHERE version = %d FORMAT TabSeparated", version), nil)
	if err != nil {
		return fmt.Errorf("read Event migration ledger: %w", err)
	}
	if applied := strings.TrimSpace(string(prior)); applied != "" {
		if applied != checksum {
			return fmt.Errorf("event migration %05d checksum differs from the ledger", version)
		}
		return nil
	}
	if _, err := s.query(ctx, ddl, nil); err != nil {
		return fmt.Errorf("apply Event migration %05d: %w", version, err)
	}
	if err := check(); err != nil {
		return err
	}
	// ClickHouse 的 DDL 与账本没有跨语句事务。先核对实表，再登记。
	stmt := fmt.Sprintf("INSERT INTO spx.event_schema_migrations (version, checksum) VALUES (%d, '%s')", version, checksum)
	if _, err := s.query(ctx, stmt, nil); err != nil {
		return fmt.Errorf("record Event migration %05d: %w", version, err)
	}
	return nil
}
