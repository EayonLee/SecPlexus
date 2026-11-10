//go:build integration

package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

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

	// 版本账本与业务表同处 iam schema：它随 schema 一起创建、一起销毁。
	var count, ledger int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='iam' AND table_type='BASE TABLE' AND table_name <> 'goose_db_version'`).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 9 {
		t.Fatalf("expected 9 IAM business tables, got %d", count)
	}

	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='iam' AND table_name='goose_db_version'`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}

	if ledger != 1 {
		t.Fatal("migration ledger must live inside the iam schema")
	}
}

// TestBaselineHasNoOrganizationSurface 固定最终 schema：没有组织表、组织列或超管标记。
// 授权只由 users → user_roles → roles → role_permissions → permissions 表达。
func TestBaselineHasNoOrganizationSurface(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	var tables int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='iam' AND table_name LIKE '%organi%'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}

	if tables != 0 {
		t.Fatalf("baseline still has %d organization tables", tables)
	}

	var columns int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='iam' AND (column_name LIKE '%org%' OR column_name IN ('is_super_admin','authz_epoch','scope','slug','membership_id'))`).Scan(&columns); err != nil {
		t.Fatal(err)
	}

	if columns != 0 {
		t.Fatalf("baseline still has %d organization/scope columns", columns)
	}

	var legacy int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='iam' AND table_name='audit_events' AND column_name LIKE '%org%'`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}

	if legacy != 0 {
		t.Fatal("audit_events still has organization columns")
	}
}

// TestSchemaInspectionNeverInitializesVersionTable 验证 serve/status 在空库中只读失败。
func TestSchemaInspectionNeverInitializesVersionTable(t *testing.T) {
	store := testutil.UnmigratedDatabase(t)
	ctx := context.Background()
	if err := store.VerifySchema(ctx); err == nil {
		t.Fatal("empty schema reported ready")
	}
	var output bytes.Buffer
	if err := repository.Status(ctx, store.Pool.Config().ConnConfig.ConnString(), &output); err == nil {
		t.Fatal("empty schema reported initialized")
	}
	var exists bool
	if err := store.Pool.QueryRow(ctx, `SELECT to_regclass('iam.goose_db_version') IS NOT NULL OR to_regnamespace('iam') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("schema inspection issued implicit initialization DDL")
	}
}

// TestMigrationStatusDistinguishesPendingCurrentAndAhead 验证版本来自 Provider 而非硬编码常量，
// 并区分 schema 版本与启动同步的内置数据。
func TestMigrationStatusDistinguishesPendingCurrentAndAhead(t *testing.T) {
	store := testutil.UnmigratedDatabase(t)
	ctx := context.Background()
	db := stdlib.OpenDBFromPool(store.Pool)
	defer func() { _ = db.Close() }()
	versionStore, err := database.NewStore(database.DialectPostgres, "iam.goose_db_version")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := goose.NewProvider("", db, migrations.Files, goose.WithStore(versionStore), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	// 账本在 iam schema 内，因此必须先建 schema（与 Migrate 的 ensureSchema 一致）。
	if _, err = db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS iam"); err != nil {
		t.Fatal(err)
	}

	// 只应用第一条迁移：schema 落后于目标版本，必须既不能服务、也要如实报告 pending。
	if _, err = provider.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifySchema(ctx); err == nil {
		t.Fatal("schema behind the target version reported ready")
	}
	readStatus := func() (current, target int64, pending bool) {
		t.Helper()
		var output bytes.Buffer
		if err = repository.Status(ctx, store.Pool.Config().ConnConfig.ConnString(), &output); err != nil {
			t.Fatal(err)
		}
		var state struct {
			Current int64 `json:"current_version"`
			Target  int64 `json:"target_version"`
			Pending bool  `json:"pending"`
		}
		if err = json.Unmarshal(output.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		return state.Current, state.Target, state.Pending
	}
	if current, target, pending := readStatus(); !pending || current == target {
		t.Fatalf("behind schema reported current=%d target=%d pending=%v", current, target, pending)
	}
	// 应用到目标版本后 schema 可服务；内置数据仍未同步。
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifySchema(ctx); err != nil {
		t.Fatal(err)
	}
	current, target, pending := readStatus()
	if pending || target != current {
		t.Fatalf("unexpected migration state current=%d target=%d pending=%v", current, target, pending)
	}
	state := struct{ Target int64 }{Target: target}

	var configured int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.users WHERE configured`).Scan(&configured); err != nil {
		t.Fatal(err)
	}
	if configured != 0 {
		t.Fatal("migration created a configured administrator")
	}

	if err := store.Seed(ctx, "status-admin", "isolated migration status password"); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.users WHERE configured AND status='active'`).Scan(&configured); err != nil {
		t.Fatal(err)
	}
	if configured != 1 {
		t.Fatalf("seed created %d configured administrators", configured)
	}
	var builtin int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.roles WHERE builtin`).Scan(&builtin); err != nil {
		t.Fatal(err)
	}
	if builtin != 1 {
		t.Fatalf("seed created %d builtin roles", builtin)
	}
	if err := versionStore.Insert(ctx, db, database.InsertRequest{Version: state.Target + 100}); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifySchema(ctx); err == nil {
		t.Fatal("future schema accepted by older application")
	}
}
