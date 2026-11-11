//go:build integration

package repository_test

import (
	"context"
	"database/sql"
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

// 用户列表的 roles 是前端渲染列的直接来源：有角色要带上，没有角色必须是空数组而不是 null。
func TestListUsersProjectsAssignedRoles(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "role projection password"); err != nil {
		t.Fatal(err)
	}

	memberID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, memberID, "role-holder", "role-holder", "Role Holder", "", false); err != nil {
		t.Fatal(err)
	}

	roleID := uuid.New()
	if err := store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := repository.CreateRole(ctx, tx, roleID, "projection-role", "投影用例", ""); err != nil {
			return err
		}

		return repository.ReplaceUserRoles(ctx, tx, memberID.String(), []string{roleID.String()}, "")
	}); err != nil {
		t.Fatal(err)
	}

	page, err := repository.ListUsers(ctx, store.DB, 50, 0, "role-holder")
	if err != nil {
		t.Fatal(err)
	}

	if len(page.Items) != 1 {
		t.Fatalf("matched %d users, want 1", len(page.Items))
	}

	roles := page.Items[0].Roles
	if len(roles) != 1 || roles[0].ID != roleID.String() || roles[0].Name != "projection-role" {
		t.Fatalf("user roles = %v, want the assigned role", roles)
	}

	// 没有角色的账号必须拿到 nil 切片：DTO 层会把 nil 折成 []，HTTP 里不能出现 null。
	if err := repository.InsertUser(ctx, store.DB, uuid.New(), "no-role", "no-role", "No Role", "", false); err != nil {
		t.Fatal(err)
	}

	plain, err := repository.ListUsers(ctx, store.DB, 50, 0, "no-role")
	if err != nil {
		t.Fatal(err)
	}

	if len(plain.Items) != 1 || plain.Items[0].Roles != nil {
		t.Fatalf("user without roles = %v, want no role refs", plain.Items)
	}
}
