//go:build integration

package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// 回归：基线角色是数据，不是 builtin。
// roles_single_builtin 唯一索引只允许一个内置角色，因此基线角色必须是普通角色。
func TestBaselineRoleIsDataAndNotBuiltin(t *testing.T) {
	dsn := os.Getenv("SPX_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("SPX_TEST_POSTGRES_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	store, err := repository.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var builtin bool
	if err = store.DB.QueryRow(`SELECT builtin FROM iam.roles WHERE name_normalized = 'authenticated'`).Scan(&builtin); err != nil {
		t.Fatalf("基线角色不存在: %v", err)
	}
	if builtin {
		t.Fatal("基线角色不能是 builtin：唯一索引只允许一个内置角色")
	}

	permissions, err := repository.BaselinePermissions(context.Background(), store.DB)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, key := range permissions {
		if key == "ingest.input.read" {
			found = true
		}
		// 基线只授予读取类；写权限绝不能出现在这里。
		if key == "ingest.input.write" || key == "ingest.mapping.write" {
			t.Fatalf("基线角色不应授予写权限: %s", key)
		}
	}
	if !found {
		t.Fatalf("基线角色缺少 ingest.input.read: %v", permissions)
	}
}
