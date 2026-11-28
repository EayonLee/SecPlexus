// Package repository 负责 Ingest 自身的数据库连接与迁移。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/migrations/postgresql"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Open 建立连接池；调用方负责关闭。
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, errors.New("database URL is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, errors.New("database unavailable")
	}

	return pool, nil
}

// VerifySchema 在常驻进程启动时只读核验当前迁移，避免不同版本同时解释配置。
func VerifySchema(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	ledger, err := database.NewStore(database.DialectPostgres, "ingest.goose_db_version")
	if err != nil {
		return err
	}
	if _, err := ledger.GetMigration(ctx, db, 0); err != nil {
		return errors.New("Ingest schema is not initialized; run the matching migration before serving")
	}
	provider, err := goose.NewProvider("", db, postgresql.Files, goose.WithStore(ledger), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return err
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil || current != target {
		return errors.New("Ingest schema version is incompatible; run the matching migration before serving")
	}
	pending, err := provider.HasPending(ctx)
	if err != nil || pending {
		return errors.New("Ingest schema has missing migrations")
	}
	return nil
}

// Migrate 执行未应用的迁移，并把结果记录成一条迁移事件。
func Migrate(ctx context.Context, dsn string) (err error) {
	started := time.Now()
	completed := false

	defer func() {
		if err != nil || !completed {
			logging.From(ctx).Error("Database migration failed", "event", "migration.failed",
				"duration_ms", time.Since(started).Milliseconds(), "error", logging.Error(err))

			return
		}

		logging.From(ctx).Info("Database migration completed", "event", "migration.completed",
			"duration_ms", time.Since(started).Milliseconds())
	}()

	pool, err := Open(ctx, dsn)
	if err != nil {
		return err
	}

	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	defer func() { err = errors.Join(err, db.Close()) }()

	if _, err = db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS ingest"); err != nil {
		return err
	}
	store, err := database.NewStore(database.DialectPostgres, "ingest.goose_db_version")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider("", db, postgresql.Files, goose.WithStore(store),
		goose.WithSessionLocker(locker), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return err
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return err
	}
	if current > target {
		return errors.New("Ingest schema is newer than this source baseline; initialize a new database")
	}
	if _, err = provider.Up(ctx); err != nil {
		// 保留底层 SQL 原因：只说"无法应用迁移"会让排障从错误信息重新开始。
		return fmt.Errorf("could not apply migrations: %w", err)
	}

	completed = true

	return nil
}
