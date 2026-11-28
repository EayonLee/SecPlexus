//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
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
		`INSERT INTO ingest.parsing_rules (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		ruleName, `{"contract_version":"v1","parse":{"mode":"json"},"branches":[{"id":"r1","name":"r1","class_uid":4001,"field_map":{"src":"src_endpoint.ip"}}]}`).Scan(&ruleID); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE name = $1`, ruleName)
	}
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, parsing_rule_id)
		 VALUES (gen_random_uuid(), $1, 'http', $2)`,
		sourceName, ruleID); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	snapshot, err := repository.LoadInputs(ctx, pool)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	for _, target := range snapshot.Targets {
		if target.Problem != "" {
			t.Fatalf("不应有编译问题: %#v", target)
		}
	}

	var found bool
	for _, entry := range snapshot.Entries {
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
		line := pipeline.New(pipeline.Options{}, nil)
		var event struct {
			Metadata struct {
				SPX struct {
					ParsingRuleID      string `json:"parsing_rule_id"`
					ParsingRuleVersion int    `json:"parsing_rule_version"`
				} `json:"spx"`
			} `json:"metadata"`
		}
		body := line.Parse(entry, []byte(`{"src":"192.0.2.1"}`), time.Now(), "").Body
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatal(err)
		}
		if event.Metadata.SPX.ParsingRuleID != ruleID || event.Metadata.SPX.ParsingRuleVersion != 1 {
			t.Fatalf("missing parsing rule provenance: %+v", event.Metadata.SPX)
		}
	}

	if !found {
		t.Fatal("没有加载到新建的接入点")
	}
}

// 非法规则只隔离依赖它的接入，并保留明确的失败阶段。
func TestLoadInputsKeepsOtherSourcesWhenOneRuleIsInvalid(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	goodRule := uniqueName(t, "good")
	badRule := uniqueName(t, "bad")
	goodSource := uniqueName(t, "src-good")
	badSource := uniqueName(t, "src-bad")

	var goodID, badID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO ingest.parsing_rules (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		goodRule, `{"contract_version":"v1","parse":{"mode":"json"},"branches":[{"id":"g","name":"g","class_uid":4001,"field_map":{"src":"src_endpoint.ip"}}]}`).Scan(&goodID); err != nil {
		t.Fatalf("创建好规则失败: %v", err)
	}

	// 未知解析模式，编译必定失败。
	if err := pool.QueryRow(ctx,
		`INSERT INTO ingest.parsing_rules (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		badRule, `{"contract_version":"v1","parse":{"mode":"protobuf"},"branches":[{"id":"b","name":"b","class_uid":4001,"field_map":{}}]}`).Scan(&badID); err != nil {
		t.Fatalf("创建坏规则失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = ANY($1)`, []string{goodSource, badSource})
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE name = ANY($1)`, []string{goodRule, badRule})
	})

	for name, ruleID := range map[string]string{goodSource: goodID, badSource: badID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO ingest.inputs (id, name, protocol, parsing_rule_id)
			 VALUES (gen_random_uuid(), $1, 'http', $2)`,
			name, ruleID); err != nil {
			t.Fatalf("创建接入点失败: %v", err)
		}
	}

	snapshot, err := repository.LoadInputs(ctx, pool)
	if err != nil {
		t.Fatalf("加载意外失败: %v", err)
	}
	foundProblem := false
	for _, target := range snapshot.Targets {
		foundProblem = foundProblem || target.Problem == "rule_invalid"
	}
	if !foundProblem {
		t.Fatal("坏规则没有被报告")
	}
	for _, entry := range snapshot.Entries {
		if entry.Name == badSource {
			t.Fatal("坏规则的接入仍可被启用")
		}
		if entry.Name == goodSource {
			return
		}
	}
	t.Fatal("有效接入被其他规则的编译错误影响")
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

	snapshot, err := repository.LoadInputs(ctx, pool)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	for _, entry := range snapshot.Entries {
		if entry.Name == name {
			t.Fatal("停用的接入点被加载了")
		}
	}
}
