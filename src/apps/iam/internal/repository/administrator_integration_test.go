//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

func TestConfiguredAdministratorCredentialsAreAuthoritativeAndIdempotent(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	original, changed := "configured administrator password", "rotated configuration password"
	if err := store.Seed(ctx, "admin", original); err != nil {
		t.Fatal(err)
	}
	before, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !before.Configured {
		t.Fatal("seeded administrator is not marked as configured")
	}
	if err = store.Seed(ctx, "admin", original); err != nil {
		t.Fatal(err)
	}
	repeated, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.PasswordHash != before.PasswordHash || repeated.AuthEpoch != before.AuthEpoch {
		t.Fatal("unchanged configuration invalidated credentials")
	}
	if err = store.Seed(ctx, "admin", changed); err != nil {
		t.Fatal(err)
	}
	after, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = (account.PasswordHasher{}).CompareHashAndPassword(after.PasswordHash, changed); err != nil {
		t.Fatal("configured password was not applied")
	}
	if after.ID != before.ID || after.AuthEpoch <= before.AuthEpoch {
		t.Fatal("credential rotation did not preserve the account or bump its version")
	}
	if err = store.Seed(ctx, "admin", ""); err == nil {
		t.Fatal("missing authoritative password was accepted")
	}
}

// 数据库只允许一个配置托管账号；内置角色也必须是唯一的。
func TestConfiguredAdministratorAndBuiltinRoleAreUnique(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "first configured password"); err != nil {
		t.Fatal(err)
	}
	if err := repository.InsertUser(ctx, store.DB, uuid.New(), "regular", "regular", "Regular", "", true); err == nil {
		t.Fatal("database permitted a second configured administrator")
	}
	var configured []string
	rows, err := store.Pool.Query(ctx, `SELECT username_normalized FROM iam.users WHERE configured`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var username string
		if err = rows.Scan(&username); err != nil {
			t.Fatal(err)
		}
		configured = append(configured, username)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(configured) != 1 || configured[0] != "admin" {
		t.Fatalf("unexpected configured identities: %v", configured)
	}

	var builtin int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.roles WHERE builtin`).Scan(&builtin); err != nil {
		t.Fatal(err)
	}
	if builtin != 1 {
		t.Fatalf("builtin roles = %d, want 1", builtin)
	}
	if _, err := store.Pool.Exec(ctx, `INSERT INTO iam.roles(id,name,builtin) VALUES($1,'第二个内置角色',true)`, uuid.New()); err == nil {
		t.Fatal("database permitted a second builtin role")
	}
}

// 配置改名后，旧账号保留为普通账号但失去内置标记与内置角色。
func TestRenamingConfiguredAdministratorMovesBuiltinIdentity(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "isolated rename password"); err != nil {
		t.Fatal(err)
	}
	var oldID uuid.UUID
	if err := store.Pool.QueryRow(ctx, `SELECT id FROM iam.users WHERE configured`).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	if err := store.Seed(ctx, "renamed", "isolated rename password"); err != nil {
		t.Fatal(err)
	}
	var newID uuid.UUID
	if err := store.Pool.QueryRow(ctx, `SELECT id FROM iam.users WHERE configured`).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	if newID == oldID {
		t.Fatal("rename reused the previous configured account")
	}
	var oldConfigured bool
	if err := store.Pool.QueryRow(ctx, `SELECT configured FROM iam.users WHERE id=$1`, oldID).Scan(&oldConfigured); err != nil {
		t.Fatal(err)
	}
	if oldConfigured {
		t.Fatal("previous configured account kept its builtin mark")
	}
	var oldBuiltin int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.user_roles ur JOIN iam.roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.builtin`, oldID).Scan(&oldBuiltin); err != nil {
		t.Fatal(err)
	}
	if oldBuiltin != 0 {
		t.Fatal("previous configured account kept the builtin role")
	}
}
