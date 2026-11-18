//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

const validRule = `{"id":"r","name":"r","parse":{"mode":"json"},"field_map":{"src":"src_endpoint.ip"},"constants":{"class_uid":2004}}`

// 回归：编译不过的规则不进表——避免运行期才发现。
func TestCreateMappingRejectsUncompilableSpec(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	if _, err := repository.CreateMapping(ctx, pool, repository.CreateMappingParams{
		Name:       uniqueName(t, "badrule"),
		Definition: []byte(`{"id":"b","name":"b","parse":{"mode":"protobuf"},"field_map":{},"constants":{"class_uid":2004}}`),
	}); err == nil {
		t.Fatal("编译不过的规则被写入")
	}
}

// 回归：换规则内容会让版本自增，事件才能按版本归因。
func TestUpdateMappingBumpsVersion(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "rule")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE name = $1`, name) })

	created, err := repository.CreateMapping(ctx, pool, repository.CreateMappingParams{Name: name, Definition: []byte(validRule)})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("初始版本 = %d，期望 1", created.Version)
	}

	updated, err := repository.UpdateMapping(ctx, pool, created.ID, repository.CreateMappingParams{
		Name:       name,
		Definition: []byte(`{"id":"r","name":"r","parse":{"mode":"kv"},"field_map":{"a":"actor.user.name"},"constants":{"class_uid":2004}}`),
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("更新后版本 = %d，期望 2", updated.Version)
	}
}

// 回归：删除仍被接入点引用的规则必须返回 ErrMappingReferenced（接口转 409）。
func TestDeleteMappingInUseIsRejected(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	ruleName := uniqueName(t, "usedrule")
	sourceName := uniqueName(t, "usedsrc")

	created, err := repository.CreateMapping(ctx, pool, repository.CreateMappingParams{Name: ruleName, Definition: []byte(validRule)})
	if err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	if _, _, err = repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: sourceName, Protocol: "http", MappingID: &created.ID,
	}); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE name = $1`, ruleName)
	})

	if err = repository.DeleteMapping(ctx, pool, created.ID); !errors.Is(err, repository.ErrMappingReferenced) {
		t.Fatalf("错误 = %v，期望 ErrMappingReferenced", err)
	}
}
