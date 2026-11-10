//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// TestGeneratedQueriesShareTheCallerTransaction 守住最关键的不变量：
// 生成的查询绑定在调用方的事务句柄上，因此两者是同一条连接、同一个事务。
// 若各自开连接，回滚之后就仍能读到已写入的行。
func TestGeneratedQueriesShareTheCallerTransaction(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	roleID := uuid.New()

	rollback := errors.New("roll back on purpose")
	err := store.Transaction(ctx, func(tx *sql.Tx) error {
		if createErr := repository.CreateRole(ctx, tx, roleID, "transaction-probe", "probe", ""); createErr != nil {
			return createErr
		}

		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction did not roll back: %v", err)
	}

	var count int64
	if scanErr := store.DB.QueryRowContext(ctx, "SELECT count(*) FROM iam.roles WHERE id = $1", roleID).Scan(&count); scanErr != nil {
		t.Fatal(scanErr)
	}
	if count != 0 {
		t.Fatalf("role written through generated queries survived the rollback: count=%d", count)
	}
}

// TestLockOperationsRejectSessionsWithoutTransaction 守住锁的语义：
// 行锁与 advisory 事务锁在自动提交连接上会立刻释放，静默失效比报错危险得多。
func TestLockOperationsRejectSessionsWithoutTransaction(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	if err := repository.LockUsers(ctx, store.DB, []uuid.UUID{uuid.New()}); err == nil {
		t.Fatal("row lock accepted a session without a transaction")
	}

	if err := repository.LockManagement(ctx, store.DB, repository.ManagementLock); err == nil {
		t.Fatal("management lock accepted a session without a transaction")
	}
}
