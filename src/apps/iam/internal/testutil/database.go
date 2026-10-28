//go:build integration

// Package testutil 仅创建和清理当前测试拥有的独立数据库。
package testutil

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// UnmigratedDatabase 创建独立测试数据库，并仅注册本次资源的清理。
func UnmigratedDatabase(t *testing.T) *repository.Store {
	t.Helper()
	dsn := os.Getenv("SPX_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Fatal("SPX_TEST_POSTGRES_URL is required; use the isolated integration runner")
	}

	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to test database administrator")
	}

	t.Cleanup(func() { admin.Close(ctx) })
	name := "spx_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("clean test database: %v", err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test database URL")
	}

	parsed.Path = "/" + name
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)
	store, err := repository.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })
	return store
}

// Database 在独立测试数据库中应用真实版本化迁移。
func Database(t *testing.T) *repository.Store {
	t.Helper()
	store := UnmigratedDatabase(t)
	if err := repository.Migrate(context.Background(), store.Pool.Config().ConnConfig.ConnString()); err != nil {
		t.Fatal(err)
	}

	return store
}
