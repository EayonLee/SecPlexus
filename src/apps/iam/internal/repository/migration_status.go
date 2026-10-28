package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/EayonLee/SecPlexus/src/apps/iam/migrations"
)

// migrationProvider 让迁移、状态查询和启动检查使用同一迁移集合与锁策略。
func migrationProvider(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	store, err := database.NewStore(database.DialectPostgres, "iam_goose_db_version")
	if err != nil {
		return nil, err
	}
	return goose.NewProvider("", db, migrations.Files, goose.WithStore(store),
		goose.WithSessionLocker(locker), goose.WithDisableGlobalRegistry(true))
}

// schemaState 先只读确认版本表已初始化，避免 Provider 查询隐式建表。
func schemaState(ctx context.Context, db *sql.DB) (int64, int64, bool, error) {
	store, err := database.NewStore(database.DialectPostgres, "iam_goose_db_version")
	if err != nil {
		return 0, 0, false, err
	}
	if _, err := store.GetMigration(ctx, db, 0); err != nil {
		return 0, 0, false, errors.New("IAM schema is not initialized; run migrate before serving")
	}
	provider, err := migrationProvider(db)
	if err != nil {
		return 0, 0, false, err
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return 0, 0, false, err
	}
	pending, err := provider.HasPending(ctx)
	return current, target, pending, err
}

// Status 输出只读迁移状态，不初始化数据库、账号或运行服务。
func Status(ctx context.Context, dsn string, output io.Writer) (err error) {
	pool, err := Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := stdlib.OpenDBFromPool(pool)
	defer func() { err = errors.Join(err, db.Close()) }()
	current, target, pending, err := schemaState(ctx, db)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{
		"current_version": current, "target_version": target, "pending": pending,
	})
}
