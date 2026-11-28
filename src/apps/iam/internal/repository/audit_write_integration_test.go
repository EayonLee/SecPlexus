//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// 回归：平台服务写入的审计事实记为 system，且 target_type 闭集生效。
func TestAppendAuditRecordsServiceFacts(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	requestID := "it-audit-" + t.Name()
	audit := repository.Audit{
		EventID:    uuid.New(),
		ActorType:  enum.ActorTypeSystem,
		Action:     "ingest.input.write",
		TargetType: string(enum.TargetTypeInput),
		Outcome:    enum.AuditOutcomeSuccess,
		RequestID:  requestID,
		Metadata:   map[string]any{"source": "integration-test"},
	}

	if err := repository.AppendAudit(ctx, store.DB, audit); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	if err := repository.AppendAudit(ctx, store.DB, audit); err != nil {
		t.Fatalf("审计重投失败: %v", err)
	}
	var count int
	if err := store.DB.QueryRowContext(ctx, "SELECT count(*) FROM iam.audit_events WHERE request_id = $1", requestID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("审计重投创建了重复记录: count=%d err=%v", count, err)
	}

	var actorType, targetType, outcome string
	err := store.DB.QueryRowContext(ctx,
		`SELECT actor_type, target_type, outcome FROM iam.audit_events WHERE request_id = $1`, requestID).
		Scan(&actorType, &targetType, &outcome)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	if actorType != string(enum.ActorTypeSystem) || targetType != string(enum.TargetTypeInput) || outcome != string(enum.AuditOutcomeSuccess) {
		t.Fatalf("落库不一致: %s/%s/%s", actorType, targetType, outcome)
	}

	t.Cleanup(func() {
		_, _ = store.DB.ExecContext(context.Background(), `DELETE FROM iam.audit_events WHERE request_id = $1`, requestID)
	})
}

// 回归：非法 target_type 被数据库拒绝，闭集真的生效。
func TestAppendAuditRejectsUnknownTargetType(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	err := repository.AppendAudit(ctx, store.DB, repository.Audit{
		ActorType:  enum.ActorTypeSystem,
		Action:     "ingest.input.write",
		TargetType: "not_a_real_target",
		Outcome:    enum.AuditOutcomeSuccess,
		RequestID:  "it-audit-bad",
	})
	if err == nil {
		t.Fatal("非法 target_type 被接受")
	}
}
