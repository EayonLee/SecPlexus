//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
	"github.com/EayonLee/SecPlexus/src/apps/iam/migrations"
)

func TestFailedMigrationCanBeRetriedAndConcurrentMigrationsConverge(t *testing.T) {
	store := testutil.UnmigratedDatabase(t)
	ctx := context.Background()
	dsn := store.Pool.Config().ConnConfig.ConnString()
	if _, err := store.Pool.Exec(ctx, `CREATE SCHEMA iam; CREATE TABLE iam.users(sentinel boolean)`); err != nil {
		t.Fatal(err)
	}

	if err := repository.Migrate(ctx, dsn); err == nil {
		t.Fatal("conflicting schema did not fail migration")
	}

	var partiallyApplied bool
	if err := store.Pool.QueryRow(ctx, `SELECT to_regclass('iam.password_credentials') IS NOT NULL`).Scan(&partiallyApplied); err != nil {
		t.Fatal(err)
	}

	if partiallyApplied {
		t.Fatal("failed migration left partially applied tables")
	}

	if _, err := store.Pool.Exec(ctx, `DROP TABLE iam.users`); err != nil {
		t.Fatal(err)
	}

	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- repository.Migrate(ctx, dsn)
		}()
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	var count int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='iam' AND table_type='BASE TABLE'`).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 11 {
		t.Fatalf("expected 11 IAM tables, got %d", count)
	}
}

// TestPasswordPolicyUpgradePreservesCredentials 验证已有账号升级后保留凭据和管理员身份。
func TestPasswordPolicyUpgradePreservesCredentials(t *testing.T) {
	store := testutil.UnmigratedDatabase(t)
	ctx := context.Background()
	db := stdlib.OpenDBFromPool(store.Pool)
	defer db.Close()

	versionStore, err := database.NewStore(database.DialectPostgres, "iam_goose_db_version")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := goose.NewProvider("", db, migrations.Files, goose.WithStore(versionStore))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = provider.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}

	if err = store.Seed(ctx, "admin", "secplexus"); err != nil {
		t.Fatal(err)
	}

	before, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	if err = repository.Migrate(ctx, store.Pool.Config().ConnConfig.ConnString()); err != nil {
		t.Fatal(err)
	}

	// 重复初始化不覆盖用户已有密码，配置变化也不能暗中改凭据。
	if err = store.Seed(ctx, "admin", "another configured password"); err != nil {
		t.Fatal(err)
	}

	after, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}

	if after.ID != before.ID || after.PasswordHash != before.PasswordHash || !after.IsSuperAdmin {
		t.Fatal("升级或重复初始化改变了现有管理员")
	}

	if err = (identity.PasswordHasher{}).CompareHashAndPassword(after.PasswordHash, "secplexus"); err != nil {
		t.Fatal("默认管理员凭据无法验证")
	}

	if err = store.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}

	var obsolete int
	if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='iam' AND column_name IN ('must_change_password','temporary_expires_at')`).Scan(&obsolete); err != nil {
		t.Fatal(err)
	}

	if obsolete != 0 {
		t.Fatal("过时密码状态列未删除")
	}
}
