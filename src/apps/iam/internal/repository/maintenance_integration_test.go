//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// 保留期窗口必须只删除超出窗口的记录，窗口内的审计与会话不能被误删。
func TestMaintainKeepsRecordsInsideRetentionWindow(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, userID, "retain-member", "retain-member", "保留成员", ""); err != nil {
		t.Fatal(err)
	}

	insertAudit := func(id uuid.UUID, age time.Duration) {
		t.Helper()
		if err := store.DB.Exec(
			`INSERT INTO iam.audit_events(id,occurred_at,actor_type,action,target_type,outcome,request_id) VALUES($1,clock_timestamp()-$2::interval,'user','iam.auth.login','user','success',$3)`,
			id, pgtype.Interval{Microseconds: age.Microseconds(), Valid: true}, id.String(),
		).Error; err != nil {
			t.Fatal(err)
		}
	}

	freshAudit, staleAudit := uuid.New(), uuid.New()
	insertAudit(freshAudit, 24*time.Hour)
	insertAudit(staleAudit, 400*24*time.Hour)

	if err := store.Maintain(ctx, 90, 30); err != nil {
		t.Fatal(err)
	}

	var freshRemaining, staleRemaining int64
	if err := store.DB.Raw(`SELECT count(*) FROM iam.audit_events WHERE id=?`, freshAudit).Scan(&freshRemaining).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.DB.Raw(`SELECT count(*) FROM iam.audit_events WHERE id=?`, staleAudit).Scan(&staleRemaining).Error; err != nil {
		t.Fatal(err)
	}

	if freshRemaining != 1 {
		t.Fatalf("audit inside retention window was removed (%d remaining)", freshRemaining)
	}

	if staleRemaining != 0 {
		t.Fatalf("audit outside retention window survived (%d remaining)", staleRemaining)
	}
}

// 非正数保留期是配置错误，必须拒绝而不是把所有记录当过期清空。
func TestMaintainRejectsNonPositiveRetention(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	for _, retention := range [][2]int{{0, 30}, {90, 0}, {-1, 30}, {90, -1}} {
		if err := store.Maintain(ctx, retention[0], retention[1]); err != repository.ErrInvalid {
			t.Fatalf("Maintain(%d,%d) err = %v, want ErrInvalid", retention[0], retention[1], err)
		}
	}
}

// 恢复备份后必须撤销全部会话，否则备份里仍有效的旧会话可以继续访问。
func TestRevokeAfterRestoreRevokesEverySession(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, userID, "restore-member", "restore-member", "恢复成员", ""); err != nil {
		t.Fatal(err)
	}

	sessionID := uuid.New()
	if err := store.DB.Exec(
		`INSERT INTO iam.user_sessions(id,user_id,token_hash,auth_epoch,state,authenticated_at,absolute_expires_at,idle_expires_at,last_seen_at,user_agent) VALUES(?,?,?,1,'active',clock_timestamp(),clock_timestamp()+interval '8 hours',clock_timestamp()+interval '30 minutes',clock_timestamp(),'agent')`,
		sessionID, userID, sessionID.String(),
	).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.RevokeAfterRestore(ctx); err != nil {
		t.Fatal(err)
	}

	var state string
	if err := store.DB.Raw(`SELECT state FROM iam.user_sessions WHERE id=?`, sessionID).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}

	if state != "revoked" {
		t.Fatalf("session state = %q, want revoked", state)
	}
}
