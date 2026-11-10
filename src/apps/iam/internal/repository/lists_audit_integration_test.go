//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// 键集分页必须覆盖全部审计事件且不重复：游标边界来自上一页最后一行。
func TestListAuditEventsPagesWithoutGaps(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	if err := store.Seed(ctx, "admin", "audit paging password"); err != nil {
		t.Fatal(err)
	}

	const total = 7
	for i := 0; i < total; i++ {
		if err := repository.AppendAudit(ctx, store.DB, repository.Audit{
			Action:     "iam.test.paging",
			TargetType: "test",
			RequestID:  uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := repository.ListAuditEvents(ctx, store.DB, repository.AuditFilter{}, 3, cursor)
		if err != nil {
			t.Fatal(err)
		}

		pages++
		for _, raw := range page.Items {
			if seen[string(raw)] {
				t.Fatalf("event repeated across pages: %s", raw)
			}

			seen[string(raw)] = true
		}

		if page.NextCursor == "" {
			break
		}

		cursor = page.NextCursor
		if pages > 10 {
			t.Fatal("paging did not terminate")
		}
	}

	// Seed 也写审计，因此是「不少于」：关键是本次写入的 7 条全部出现且不重复。
	if len(seen) < total {
		t.Fatalf("visited %d events, want at least %d", len(seen), total)
	}

	if pages != 3 {
		t.Fatalf("expected 3 pages for %d events with limit 3 plus seed rows, got %d", total, pages)
	}
}
