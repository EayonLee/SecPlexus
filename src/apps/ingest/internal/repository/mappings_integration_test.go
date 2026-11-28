//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/service"
)

const validRule = `{"description":"Network activity","contract_version":"v1","parse":{"mode":"json"},"branches":[{"id":"r","name":"r","class_uid":4001,"field_map":{"src":"src_endpoint.ip"}}]}`

// 回归：编译不过的规则不进表——避免运行期才发现。
func TestCreateMappingRejectsUncompilableSpec(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	if _, err := createMapping(ctx, pool, service.SaveRuleInput{
		Name: uniqueName(t, "badrule"), Vendor: "Test", Product: "Edge",
		Definition: []byte(`{"contract_version":"v1","parse":{"mode":"protobuf"},"branches":[{"id":"b","name":"b","class_uid":4001,"field_map":{}}]}`),
	}); err == nil {
		t.Fatal("编译不过的规则被写入")
	}
}

// 回归：换规则内容会让版本自增，事件才能按版本归因。
func TestUpdateMappingBumpsVersion(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "rule")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE name = $1`, name) })

	created, err := createMapping(ctx, pool, service.SaveRuleInput{Name: name, Vendor: "Test", Product: "Edge", Definition: []byte(validRule)})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("初始版本 = %d，期望 1", created.Version)
	}

	updated, err := updateMapping(ctx, pool, created.ID, service.SaveRuleInput{
		Name: name, ExpectedVersion: int32(created.Version),
		Vendor: "Test", Product: "Edge",
		Definition: []byte(`{"description":"Network KV","contract_version":"v1","parse":{"mode":"kv"},"branches":[{"id":"r","name":"r","class_uid":4001,"field_map":{"a":"src_endpoint.hostname"}}]}`),
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("更新后版本 = %d，期望 2", updated.Version)
	}
	if err := repository.DeleteMapping(ctx, pool, created.ID); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"json", "kv"} {
		var mode string
		err := pool.QueryRow(ctx, "SELECT definition#>>'{parse,mode}' FROM ingest.parsing_rule_revisions WHERE parsing_rule_id=$1 AND version=$2", created.ID, i+1).Scan(&mode)
		if err != nil || mode != want {
			t.Fatalf("rule revision %d after deletion = %q, %v", i+1, mode, err)
		}
	}
}

// 回归：删除仍被接入点引用的规则必须返回 ErrMappingInUse（接口转 409）。
func TestDeleteMappingInUseIsRejected(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	ruleName := uniqueName(t, "usedrule")
	sourceName := uniqueName(t, "usedsrc")

	created, err := createMapping(ctx, pool, service.SaveRuleInput{Name: ruleName, Vendor: "Test", Product: "Edge", Definition: []byte(validRule)})
	if err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	if _, _, err = repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: sourceName, Protocol: "http", ParsingRuleID: &created.ID,
	}); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE name = $1`, ruleName)
	})

	if err = repository.DeleteMapping(ctx, pool, created.ID); !errors.Is(err, repository.ErrMappingInUse) {
		t.Fatalf("错误 = %v，期望 ErrMappingInUse", err)
	}
}

func TestBuiltinMappingIsReadOnly(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO ingest.parsing_rules (id, name, vendor, product, definition, builtin)
		VALUES (gen_random_uuid(), 'Read only fixture', 'Example', 'Test', $1, true) RETURNING id
	`, validRule).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := repository.ReadMapping(ctx, pool, id)
	if err != nil || !mapping.Builtin {
		t.Fatalf("could not read builtin rule: %v", err)
	}
	if _, err := updateMapping(ctx, pool, id, service.SaveRuleInput{
		Name: mapping.Name, Vendor: mapping.Vendor, Product: mapping.Product,
		ExpectedVersion: int32(mapping.Version), Definition: []byte(validRule),
	}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("builtin update returned %v", err)
	}
	if err := repository.DeleteMapping(ctx, pool, id); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("builtin delete returned %v", err)
	}
}

func createMapping(ctx context.Context, pool *pgxpool.Pool, input service.SaveRuleInput) (repository.Mapping, error) {
	params, _, err := service.PrepareRuleWrite(input)
	if err != nil {
		return repository.Mapping{}, err
	}
	return repository.CreateMapping(ctx, pool, params)
}

func updateMapping(ctx context.Context, pool *pgxpool.Pool, id string, input service.SaveRuleInput) (repository.Mapping, error) {
	params, _, err := service.PrepareRuleWrite(input)
	if err != nil {
		return repository.Mapping{}, err
	}
	return repository.UpdateMapping(ctx, pool, id, params)
}
