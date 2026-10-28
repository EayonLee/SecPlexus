package repository

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm/logger"
)

// TestQueryDiagnosticsNeverRenderSQL 避免开启诊断后通过 ORM 输出密码或查询参数。
func TestQueryDiagnosticsNeverRenderSQL(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	queryLogger{level: logger.Warn}.Trace(context.Background(), time.Now(), func() (string, int64) {
		t.Fatal("SQL renderer must not be invoked")
		return "secret-sql", 0
	}, &pgconn.PgError{Code: "23505", Detail: "secret-detail", ConstraintName: "roles_code_key"})
	if strings.Contains(output.String(), "secret-") || !strings.Contains(output.String(), "23505") || !strings.Contains(output.String(), "roles_code_key") {
		t.Fatal("unsafe or incomplete query diagnosis")
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
	queryLogger{level: logger.Warn}.Trace(ctx, time.Now(), nil, context.Canceled)
	if output.Len() != 0 {
		t.Fatal("正常请求取消不应输出默认级别数据库告警")
	}
}
