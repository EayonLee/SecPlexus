package authentication

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// logAuthFailure 只输出稳定原因和用户标识，不输出账号、密码或原始错误。
func TestLogAuthFailureKeepsStableDiagnosticFields(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	userID := uuid.New()
	logAuthFailure(context.Background(), slog.LevelWarn, "auth.login_failed", "invalid_credentials", &userID)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal("invalid structured log")
	}
	if record["event"] != "auth.login_failed" || record["reason"] != "invalid_credentials" || record["user_id"] != userID.String() {
		t.Fatalf("unexpected diagnostic fields: %v", record)
	}
	if _, ok := record["username"]; ok {
		t.Fatal("username must not be logged")
	}
}
