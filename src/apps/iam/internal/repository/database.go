// Package repository 管理 IAM 数据访问、事务与迁移入口。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/EayonLee/SecPlexus/src/apps/iam/migrations"
)

// Migrate 使用配置的数据库账号执行版本化迁移，普通服务启动不自动改表。
func Migrate(ctx context.Context, dsn string) (err error) {
	started := time.Now()
	completed := false
	defer func() {
		if err != nil || !completed {
			logging.From(ctx).Error("数据库迁移失败", "event", "migration.failed", "duration_ms", time.Since(started).Milliseconds(), "error", logging.Error(err))
		} else {
			logging.From(ctx).Info("数据库迁移完成", "event", "migration.completed", "duration_ms", time.Since(started).Milliseconds())
		}
	}()
	if dsn == "" {
		return errors.New("migration database URL is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("invalid migration database configuration")
	}

	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}

	versionStore, err := database.NewStore(database.DialectPostgres, "iam_goose_db_version")
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider("", db, migrations.Files, goose.WithStore(versionStore), goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}

	if _, err = provider.Up(ctx); err != nil {
		return fmt.Errorf("IAM migration failed: %w", err)
	}

	completed = true
	return nil
}

// Open 创建并验证连接池，不记录包含凭据的连接字符串。
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, errors.New("IAM database URL is required")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid IAM database configuration")
	}

	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("cannot create IAM database pool")
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		logging.From(ctx).Error("PostgreSQL 连接检查失败", "event", "dependency.failed", "dependency", "postgres", "error", logging.Error(err))
		return nil, errors.New("IAM database unavailable")
	}

	return pool, nil
}
