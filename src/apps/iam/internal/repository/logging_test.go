package repository

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestQueryDiagnosticsNeverRenderSQL 避免开启诊断后输出密码或查询参数。
// 追踪器拿不到 SQL 文本，只能看到 SQLSTATE、约束名与失败详情以外的诊断字段。
func TestQueryDiagnosticsNeverRenderSQL(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := QueryTracer{}.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "secret-sql"})
	QueryTracer{}.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: &pgconn.PgError{Code: "23505", Detail: "secret-detail", ConstraintName: "roles_code_key"}})
	if strings.Contains(output.String(), "secret-") || !strings.Contains(output.String(), "23505") || !strings.Contains(output.String(), "roles_code_key") {
		t.Fatalf("unsafe or incomplete query diagnosis: %s", output.String())
	}
}

// TestCanceledQueryIsNotAnIncident 正常请求取消不能产生数据库故障告警。
func TestCanceledQueryIsNotAnIncident(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := QueryTracer{}.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	QueryTracer{}.TraceQueryEnd(started, nil, pgx.TraceQueryEndData{Err: context.Canceled})
	if output.Len() != 0 {
		t.Fatal("正常请求取消不应输出默认级别数据库告警")
	}
}

// TestSlowQueryIsReported 超过一秒的查询必须留下一条慢查询记录。
func TestSlowQueryIsReported(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	started := QueryTracer{}.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{})
	time.Sleep(1100 * time.Millisecond)
	QueryTracer{}.TraceQueryEnd(started, nil, pgx.TraceQueryEndData{})
	if !strings.Contains(output.String(), "db.slow") {
		t.Fatalf("slow query was not reported: %s", output.String())
	}
}
