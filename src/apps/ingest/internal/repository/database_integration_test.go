//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

func TestFirstSchemaIsEmptyAndMigrationPreservesCurrentData(t *testing.T) {
	pool := unmigratedPool(t)
	ctx := context.Background()
	dsn := pool.Config().ConnConfig.ConnString()
	if err := repository.Migrate(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	var rules, version int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ingest.parsing_rules").Scan(&rules); err != nil || rules != 0 {
		t.Fatalf("fresh schema unexpectedly contains rules: %d, %v", rules, err)
	}
	if err := pool.QueryRow(ctx, "SELECT max(version_id) FROM ingest.goose_db_version WHERE is_applied").Scan(&version); err != nil || version != 1 {
		t.Fatalf("expected first schema version: %d, %v", version, err)
	}
	created, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{Name: "new-input", Protocol: "http", Vendor: "Example", Product: "Edge"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := repository.VerifySchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var vendor, product string
	if err := pool.QueryRow(ctx, "SELECT vendor, product FROM ingest.inputs WHERE id=$1", created.ID).Scan(&vendor, &product); err != nil || vendor != "Example" || product != "Edge" {
		t.Fatalf("repeat migration altered input: %q, %q, %v", vendor, product, err)
	}
}

func TestFirstSchemaEnforcesPublishedRuleShape(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	const insert = `INSERT INTO ingest.parsing_rules(id,name,vendor,definition) VALUES (gen_random_uuid(),$1,$2,$3::jsonb)`
	for _, test := range []struct{ name, vendor, rule string }{
		{"prototype", "Example", `{"contract_version":"1.1.0","parse":{"mode":"json"},"description":"fixture"}`},
		{"non-text description", "Example", `{"contract_version":"v1","parse":{"mode":"json"},"description":42}`},
		{"oversized vendor", strings.Repeat("厂", 121), validRule},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, insert, test.name, test.vendor, test.rule); err == nil {
				t.Fatalf("invalid rule was saved: %s", test.name)
			}
		})
	}
}
