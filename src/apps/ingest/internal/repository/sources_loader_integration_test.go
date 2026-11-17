//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

// 回归：解析规则能编译时，接入点带着规则进入运行期。
func TestLoadInputsCompilesRule(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	ruleName := uniqueName(t, "rule")
	sourceName := uniqueName(t, "src")

	var ruleID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO ingest.mappings (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		ruleName, `{"id":"r1","name":"r1","parse":{"mode":"json"},"field_map":{"src":"src_endpoint.ip"},"constants":{"class_uid":2004}}`).Scan(&ruleID); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE name = $1`, ruleName)
	}
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, mapping_id)
		 VALUES (gen_random_uuid(), $1, 'http', $2)`,
		sourceName, ruleID); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	entries, problems, err := repository.LoadInputs(ctx, pool)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("不应有编译问题: %#v", problems)
	}

	var found bool
	for _, entry := range entries {
		if entry.Name != sourceName {
			continue
		}

		found = true
		if entry.Mapping == nil {
			t.Fatal("接入点没有编译出解析规则")
		}
		if entry.Protocol != "http" {
			t.Fatalf("protocol = %q", entry.Protocol)
		}
	}

	if !found {
		t.Fatal("没有加载到新建的接入点")
	}
}

// 回归：非法规则让整次加载失败（Problems 非空），调用方据此保留旧配置。
// 这是 nginx fail-safe reload 的语义：新配置整体校验通过才切换。
func TestLoadInputsRejectsWholeSetOnBadRule(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	goodRule := uniqueName(t, "good")
	badRule := uniqueName(t, "bad")
	goodSource := uniqueName(t, "src-good")
	badSource := uniqueName(t, "src-bad")

	var goodID, badID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO ingest.mappings (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		goodRule, `{"id":"g","name":"g","parse":{"mode":"json"},"field_map":{"src":"src_endpoint.ip"},"constants":{"class_uid":2004}}`).Scan(&goodID); err != nil {
		t.Fatalf("创建好规则失败: %v", err)
	}

	// 未知解析模式，编译必定失败。
	if err := pool.QueryRow(ctx,
		`INSERT INTO ingest.mappings (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		badRule, `{"id":"b","name":"b","parse":{"mode":"protobuf"},"field_map":{},"constants":{"class_uid":2004}}`).Scan(&badID); err != nil {
		t.Fatalf("创建坏规则失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = ANY($1)`, []string{goodSource, badSource})
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE name = ANY($1)`, []string{goodRule, badRule})
	})

	for name, ruleID := range map[string]string{goodSource: goodID, badSource: badID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO ingest.inputs (id, name, protocol, mapping_id)
			 VALUES (gen_random_uuid(), $1, 'http', $2)`,
			name, ruleID); err != nil {
			t.Fatalf("创建接入点失败: %v", err)
		}
	}

	_, problems, err := repository.LoadInputs(ctx, pool)
	if err != nil {
		t.Fatalf("加载意外失败: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("坏规则没有被报告，整份配置会被误判为可用")
	}

	for _, problem := range problems {
		if problem.SourceID == "" || problem.Reason == "" {
			t.Fatalf("问题记录不完整: %#v", problem)
		}
	}
}

// 停用的接入点不进入运行期：它不该占用端口或消费组。
func TestLoadInputsSkipsDisabled(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	name := uniqueName(t, "off")
	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, enabled)
		 VALUES (gen_random_uuid(), $1, 'http', false)`,
		name); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, name)
	})

	entries, _, err := repository.LoadInputs(ctx, pool)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	for _, entry := range entries {
		if entry.Name == name {
			t.Fatal("停用的接入点被加载了")
		}
	}
}
