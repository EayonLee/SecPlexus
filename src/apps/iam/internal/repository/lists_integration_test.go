//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// 搜索把用户输入当字面量：% 与 _ 不能变成通配符。
func TestListUsersTreatsWildcardsLiterally(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "wildcard search password"); err != nil {
		t.Fatal(err)
	}

	if err := repository.InsertUser(ctx, store.DB, uuid.New(), "alpha", "alpha", "Alpha", "", false); err != nil {
		t.Fatal(err)
	}

	if err := repository.InsertUser(ctx, store.DB, uuid.New(), "bravo", "bravo", "Bravo", "", false); err != nil {
		t.Fatal(err)
	}

	all, err := repository.ListUsers(ctx, store.DB, 50, 0, "")
	if err != nil {
		t.Fatal(err)
	}

	if all.Total != 3 {
		t.Fatalf("empty query total = %v, want 3", all.Total)
	}

	// % 在 ILIKE 里是通配符；作为字面量搜索时只应命中名字里真的带 % 的账号。
	percent, err := repository.ListUsers(ctx, store.DB, 50, 0, "%")
	if err != nil {
		t.Fatal(err)
	}

	if percent.Total != 0 {
		t.Fatalf("query %%=%v matched %v accounts, want 0", "%", percent.Total)
	}

	// 反斜杠也是字面量，不会破坏 ESCAPE 语法。
	if _, err = repository.ListUsers(ctx, store.DB, 50, 0, `\\`); err != nil {
		t.Fatalf("backslash query failed: %v", err)
	}

	named, err := repository.ListUsers(ctx, store.DB, 50, 0, "alp")
	if err != nil {
		t.Fatal(err)
	}

	if named.Total != 1 {
		t.Fatalf("substring query total = %v, want 1", named.Total)
	}
}
