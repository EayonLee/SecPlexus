//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// TestGeneratedQueriesShareTheCallerTransaction 守住迁移期最关键的不变量：
// sqlc 生成查询绑定在 GORM 的 ConnPool 上，因此与调用方是同一个事务。
// 若两者各自开连接，回滚之后就仍能读到已写入的行。
func TestGeneratedQueriesShareTheCallerTransaction(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()
	roleID := uuid.New()

	rollback := errors.New("roll back on purpose")
	err := store.DB.Transaction(func(tx *gorm.DB) error {
		if createErr := repository.CreateRole(ctx, tx, roleID, "transaction-probe", "probe", ""); createErr != nil {
			return createErr
		}

		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction did not roll back: %v", err)
	}

	var count int64
	if scanErr := store.DB.WithContext(ctx).Raw("SELECT count(*) FROM iam.roles WHERE id = $1", roleID).Row().Scan(&count); scanErr != nil {
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
