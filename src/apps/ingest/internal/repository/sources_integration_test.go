//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

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
func TestSourceTransportFieldsAreEnforcedByDatabase(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	tests := map[string]struct {
		transport string
		token     []byte
		port      *int
		topic     *string
		brokers   []string
	}{
		"http without token":    {transport: "http"},
		"syslog without port":   {transport: "syslog"},
		"kafka without topic":   {transport: "kafka", brokers: []string{"127.0.0.1:9092"}},
		"http with syslog port": {transport: "http", token: make([]byte, 32), port: intPtr(5514)},
		"unknown transport":     {transport: "smtp", token: make([]byte, 32)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(ctx,
				`INSERT INTO ingest.sources (id, name, transport, token_digest, syslog_port, kafka_topic, kafka_brokers)
				 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)`,
				uniqueName(t, "bad"), test.transport, test.token, test.port, test.topic, test.brokers)
			if err == nil {
				t.Fatal("数据库接受了非法组合")
			}
		})
	}
}

// 回归：两个 syslog 接入点不能抢同一个端口。
func TestSyslogPortIsUnique(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	port := 5900 + int(time.Now().UnixNano()%300)

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.sources WHERE syslog_port = $1`, port)
	}
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.sources (id, name, transport, syslog_port) VALUES (gen_random_uuid(), $1, 'syslog', $2)`,
		uniqueName(t, "p1"), port); err != nil {
		t.Fatalf("第一条插入失败: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.sources (id, name, transport, syslog_port) VALUES (gen_random_uuid(), $1, 'syslog', $2)`,
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
		`INSERT INTO ingest.parse_rules (id, name, spec) VALUES (gen_random_uuid(), $1, '{}'::jsonb) RETURNING id`,
		ruleName).Scan(&ruleID); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest.sources (id, name, transport, token_digest, parse_rule_id)
		 VALUES (gen_random_uuid(), $1, 'http', $2, $3)`,
		sourceName, make([]byte, 32), ruleID); err != nil {
		t.Fatalf("创建接入点失败: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.sources WHERE name = $1`, sourceName)
		_, _ = pool.Exec(ctx, `DELETE FROM ingest.parse_rules WHERE name = $1`, ruleName)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM ingest.parse_rules WHERE id = $1`, ruleID); err == nil {
		t.Fatal("删除仍被引用的规则没有被拒绝")
	}
}

// intPtr 便于在表驱动用例里表达可空整数。
func intPtr(value int) *int { return &value }
