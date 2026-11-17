//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDSN 优先用显式测试地址，其次按与线上相同的字段拼装。
func testDSN(t *testing.T) string {
	t.Helper()

	if dsn := os.Getenv("SPX_TEST_POSTGRES_URL"); dsn != "" {
		return dsn
	}

	host := os.Getenv("SPX_POSTGRES_HOST")
	if host == "" {
		t.Skip("没有可用的 PostgreSQL 测试地址")
	}

	port, _ := strconv.Atoi(os.Getenv("SPX_POSTGRES_PORT"))
	query := url.Values{"sslmode": []string{os.Getenv("SPX_POSTGRES_SSLMODE")}}

	built := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("SPX_POSTGRES_USER"), os.Getenv("SPX_POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + os.Getenv("SPX_POSTGRES_DATABASE"),
		RawQuery: query.Encode(),
	}).String()

	return built
}

// openPool 建立连接池并在测试结束后关闭。
func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDSN(t))
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("数据库不可用: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

// uniqueName 让每次运行的测试数据互不干扰。
func uniqueName(t *testing.T, suffix string) string {
	t.Helper()

	return fmt.Sprintf("it-%d-%s", time.Now().UnixNano(), suffix)
}

// 回归：每种传输方式缺字段都必须被数据库拒绝，而不是靠应用层判断。
// 协议必填字段由用例层校验：configuration 是 JSON，数据库无法表达「每种协议必填什么」。
// 依据：Graylog 也是按输入类型校验 configuration，而不是给每种类型加固定列。
func TestInputConfigurationIsValidatedPerProtocol(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	// 端口必须唯一：用当前纳秒推导一个测试专用值，避免与历史行冲突。
	port := int32(15000 + time.Now().UnixNano()%40000)
	topic := "logs"

	invalid := map[string]repository.CreateInputParams{
		"syslog without port":   {Name: "v-syslog", Protocol: "syslog"},
		"kafka without brokers": {Name: "v-kafka1", Protocol: "kafka", KafkaTopic: &topic},
		"kafka without topic":   {Name: "v-kafka2", Protocol: "kafka", KafkaBrokers: []string{"127.0.0.1:9092"}},
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
		`INSERT INTO ingest.mappings (id, name, definition) VALUES (gen_random_uuid(), $1, '{}'::jsonb) RETURNING id`,
		ruleName).Scan(&ruleID); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.inputs (id, name, protocol, mapping_id) VALUES (gen_random_uuid(), $1, 'http', $2)`,
		sourceName, ruleID); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE name = $1`, ruleName)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE id = $1`, ruleID); err == nil {
		t.Fatal("删除仍被引用的规则没有被拒绝")
	}
}

// intPtr 便于在表驱动用例里表达可空整数。
func intPtr(value int) *int { return &value }
