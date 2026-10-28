package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestContextAndSafeError 保留定位问题所需字段，但不能输出数据库 Detail 或连接凭据。
func TestContextAndSafeError(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := With(context.Background(), "request_id", "request-one", "operation", "iam.roles.assign")
	err := fmt.Errorf("secret connection password: %w", &pgconn.PgError{Code: "23505", Message: "secret row password", Detail: "secret token", ConstraintName: "roles_org_id_code_key"})
	From(ctx).Error("数据库操作失败", "error", Error(err))
	var record map[string]any
	if json.Unmarshal(output.Bytes(), &record) != nil {
		t.Fatal("invalid structured log")
	}
	if record["request_id"] != "request-one" || record["operation"] != "iam.roles.assign" {
		t.Fatal("request context lost")
	}
	diagnosis := record["error"].(map[string]any)
	if diagnosis["sqlstate"] != "23505" || diagnosis["constraint"] != "roles_org_id_code_key" {
		t.Fatal("database diagnostic lost")
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatal("raw error leaked into diagnostic log")
	}
}
