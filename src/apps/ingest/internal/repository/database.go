// Package repository 负责 Ingest 自身的数据库连接与迁移。
//
// 只放接入点与解析规则两张表：接收路径不写业务库（spec 明确），
// 事件一律走 Kafka，ClickHouse 与关联分析属于下游模块。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/migrations"
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

	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		return errors.New("unsupported migration dialect")
	}

	if err = goose.UpContext(ctx, db, "."); err != nil {
		// 保留底层 SQL 原因：只说"无法应用迁移"会让排障从错误信息重新开始。
		return fmt.Errorf("could not apply migrations: %w", err)
	}

	completed = true

	return nil
}
