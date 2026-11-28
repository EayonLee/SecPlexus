//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
)

// openPool 为当前用例建库，再应用真实迁移。
func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool := unmigratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := repository.Migrate(ctx, pool.Config().ConnConfig.ConnString()); err != nil {
		t.Fatalf("apply ingest migrations: %v", err)
	}

	return pool
}

// unmigratedPool 使用提供的 DSN 创建和清理临时库。
func unmigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("SPX_TEST_POSTGRESQL_URL")
	if dsn == "" {
		t.Skip("SPX_TEST_POSTGRESQL_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect to test PostgreSQL administrator")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	name := "spx_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()

		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("remove test database: %v", err)
		}
	})

	testURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test PostgreSQL URL")
	}
	testURL.Path = "/" + name
	pool, err := pgxpool.New(ctx, testURL.String())
	if err != nil {
		t.Fatal("cannot open owned test database")
	}
	t.Cleanup(pool.Close)

	return pool
}

func uniqueName(t *testing.T, suffix string) string {
	t.Helper()

	return fmt.Sprintf("it-%d-%s", time.Now().UnixNano(), suffix)
}

// 回归：每种传输方式缺字段都必须被数据库拒绝，而不是靠应用层判断。
// 协议必填字段由用例层校验：configuration 是 JSON，数据库无法表达「每种协议必填什么」。
// 协议专属设置按类型校验，而不是给每种类型加固定列：加协议不应改表结构。
func TestInputConfigurationIsValidatedPerProtocol(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	// 端口必须唯一：用当前纳秒推导一个测试专用值，避免与历史行冲突。
	port := int32(15000 + time.Now().UnixNano()%40000)
	topic := "logs"
	brokers := []string{"127.0.0.1:9092"}

	invalid := map[string]repository.CreateInputParams{
		"syslog without port":   {Name: "v-syslog", Protocol: "syslog"},
		"kafka without brokers": {Name: "v-kafka1", Protocol: "kafka", KafkaTopic: &topic, KafkaStartPosition: enum.KafkaStartEarliest},
		"kafka without topic":   {Name: "v-kafka2", Protocol: "kafka", KafkaBrokers: brokers, KafkaStartPosition: enum.KafkaStartEarliest},
		"kafka without start":   {Name: "v-kafka3", Protocol: "kafka", KafkaTopic: &topic, KafkaBrokers: brokers},
		"unknown protocol":      {Name: "v-unknown", Protocol: "smtp"},
	}

	for name, params := range invalid {
		t.Run(name, func(t *testing.T) {
			params.Name = uniqueName(t, params.Name)

			if _, _, err := repository.CreateInput(ctx, pool, params); !errors.Is(err, repository.ErrInvalidCombination) {
				t.Fatalf("错误 = %v，期望 ErrInvalidCombination", err)
			}
		})
	}

	// 合法组合不能被误拒。
	if _, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: uniqueName(t, "v-valid"), Protocol: "syslog", SyslogPort: &port,
	}); err != nil {
		t.Fatalf("合法 syslog 配置被拒绝: %v", err)
	}
	kafka, _, err := repository.CreateInput(ctx, pool, repository.CreateInputParams{
		Name: uniqueName(t, "v-kafka"), Protocol: "kafka", KafkaTopic: &topic,
		KafkaBrokers: brokers, KafkaStartPosition: enum.KafkaStartSinceCreated,
	})
	if err != nil || kafka.KafkaStartPosition != enum.KafkaStartSinceCreated {
		t.Fatalf("Kafka start position was not stored: %q, %v", kafka.KafkaStartPosition, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ingest.inputs SET configuration = configuration - 'start_position' WHERE id = $1`, kafka.ID); err == nil {
		t.Fatal("the database accepted a Kafka input without its initial consumer position")
	}
}
func TestSyslogPortIsUnique(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	// 端口取自时间戳，先按端口清理残留行：抛随机数撞库不是可靠做法。
	port := 15000 + int(time.Now().UnixNano()%40000)

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE configuration->>'port' = $1`, strconv.Itoa(port))
	}

	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, configuration) VALUES (gen_random_uuid(), $1, 'syslog', jsonb_build_object('port', $2::int))`,
		uniqueName(t, "p1"), port); err != nil {
		t.Fatalf("第一条插入失败: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, configuration) VALUES (gen_random_uuid(), $1, 'syslog', jsonb_build_object('port', $2::int))`,
		uniqueName(t, "p2"), port); err == nil {
		t.Fatal("同一端口被两个接入点占用")
	}
}

// 回归：删除仍被引用的解析规则必须失败，接口据此返回 409。
func TestDeleteReferencedRuleIsRejected(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	ruleName := uniqueName(t, "rule")
	sourceName := uniqueName(t, "src")

	var ruleID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO ingest.parsing_rules (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb) RETURNING id`,
		ruleName, validRule).Scan(&ruleID); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, parsing_rule_id) VALUES (gen_random_uuid(), $1, 'http', $2)`,
		sourceName, ruleID); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE name = $1`, ruleName)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE id = $1`, ruleID); err == nil {
		t.Fatal("删除仍被引用的规则没有被拒绝")
	}
}

// intPtr 便于在表驱动用例里表达可空整数。
func intPtr(value int) *int { return &value }
