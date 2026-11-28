//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

func TestAdminWriteCommitsAuditWithMutation(t *testing.T) {
	pool := openPool(t)
	store := repository.NewAdminStore(pool)
	ctx := context.Background()

	event := iamclient.AuditEvent{Action: "ingest.input.write", Outcome: "success", TargetType: "input", RequestID: "test-request"}
	ownedCtx := iamclient.ContextWithAudit(ctx, event)
	created, _, err := store.CreateInput(ownedCtx, repository.CreateInputParams{Name: "edge", Protocol: "http"})
	if err != nil {
		t.Fatal(err)
	}
	occurredAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := pool.Exec(ctx, "UPDATE ingest.audit_outbox SET queued_at = $1", occurredAt); err != nil {
		t.Fatal(err)
	}
	checkTime := func(fact iamclient.AuditEvent) {
		payload, err := json.Marshal(fact)
		if err != nil {
			t.Fatal(err)
		}
		var delivered struct {
			OccurredAt time.Time `json:"occurred_at"`
		}
		if err := json.Unmarshal(payload, &delivered); err != nil || !delivered.OccurredAt.Equal(occurredAt) {
			t.Fatalf("delivered audit time = %v, want %v (%v)", delivered.OccurredAt, occurredAt, err)
		}
	}
	var deliveredID string
	err = store.DeliverAudit(ctx, func(_ context.Context, fact iamclient.AuditEvent) error {
		checkTime(fact)
		if fact.TargetID != created.ID || fact.RequestID != event.RequestID || fact.EventID == "" {
			t.Fatalf("wrong audit fact: %+v", fact)
		}
		deliveredID = fact.EventID
		return errors.New("IAM unavailable")
	})
	if err == nil || deliveredID == "" {
		t.Fatal("failed delivery must leave the committed fact available")
	}
	err = store.DeliverAudit(ctx, func(_ context.Context, fact iamclient.AuditEvent) error {
		checkTime(fact)
		if fact.EventID != deliveredID {
			t.Fatal("retry changed audit ID")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = store.DeliverAudit(ctx, func(context.Context, iamclient.AuditEvent) error {
		t.Fatal("acknowledged fact was delivered again")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	event.RequestID = ""
	if _, _, err = store.CreateInput(iamclient.ContextWithAudit(ctx, event), repository.CreateInputParams{Name: "rollback", Protocol: "http"}); err == nil {
		t.Fatal("business write succeeded without a valid audit fact")
	}
	page, err := store.ListInputPage(ctx, repository.InputListFilter{Limit: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("business change was not rolled back: %+v, %v", page.Items, err)
	}
}

func TestAuditRejectionPreservesFactAndDeliversLaterEvents(t *testing.T) {
	pool := openPool(t)
	store := repository.NewAdminStore(pool)
	ctx := context.Background()

	for _, name := range []string{"first", "second"} {
		event := iamclient.AuditEvent{Action: "ingest.input.write", TargetType: "input", Outcome: "success", RequestID: name}
		if _, _, err := store.CreateInput(iamclient.ContextWithAudit(ctx, event), repository.CreateInputParams{Name: name, Protocol: "http"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE ingest.audit_outbox SET queued_at = now() - interval '1 day' WHERE event->>'request_id' = 'first'"); err != nil {
		t.Fatal(err)
	}

	var delivered int
	if err := store.DeliverAudit(ctx, func(_ context.Context, event iamclient.AuditEvent) error {
		if event.RequestID == "first" {
			return iamclient.ErrInvalidAuditEvent
		}
		delivered++
		return nil
	}); err != nil || delivered != 1 {
		t.Fatalf("delivery stopped behind an invalid fact: delivered=%d, err=%v", delivered, err)
	}
	var rejected int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ingest.audit_outbox WHERE rejected_at IS NOT NULL AND event->>'request_id' = 'first'").Scan(&rejected); err != nil || rejected != 1 {
		t.Fatalf("rejected audit fact not preserved: %d, %v", rejected, err)
	}
}
