//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
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
	if err = store.Seed(ctx, "admin", original); err != nil {
		t.Fatal(err)
	}
	repeated, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.PasswordHash != before.PasswordHash || repeated.AuthVersion != before.AuthVersion {
		t.Fatal("unchanged configuration invalidated credentials")
	}
	if err = store.Seed(ctx, "admin", changed); err != nil {
		t.Fatal(err)
	}
	after, err := repository.NewQueries(store.DB).GetLoginUser(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = (identity.PasswordHasher{}).CompareHashAndPassword(after.PasswordHash, changed); err != nil {
		t.Fatal("configured password was not applied")
	}
	if after.ID != before.ID || after.AuthVersion <= before.AuthVersion {
		t.Fatal("credential rotation did not preserve identity and revoke old identity versions")
	}
	if err = store.Seed(ctx, "admin", ""); err == nil {
		t.Fatal("missing authoritative password was accepted")
	}
}

func TestConfiguredAdministratorIsTheOnlySuperAdministrator(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "first configured password"); err != nil {
		t.Fatal(err)
	}
	if err := store.Seed(ctx, "configured", "second configured password"); err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := store.DB.WithContext(ctx).Table("iam.users").Where("is_super_admin").Pluck("username_normalized", &names).Error; err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "configured" {
		t.Fatalf("unexpected administrator identities: %v", names)
	}
	id := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, id, "regular", "regular", "Regular", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.WithContext(ctx).Table("iam.users").Where("id = ?", id).Update("is_super_admin", true).Error; err == nil {
		t.Fatal("database permitted a second super administrator")
	}
}
