//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
)

// 时区偏好只在渲染层生效，写入不能连带改变账号版本或撤销会话。
func TestTimeZoneRoundTripDoesNotTouchIdentityVersion(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, userID, "tz-member", "tz-member", "时区用户", "", false); err != nil {
		t.Fatal(err)
	}

	if value, err := repository.UserTimeZone(ctx, store.DB, userID); err != nil || value != "" {
		t.Fatalf("initial timezone = %q, err = %v; want empty", value, err)
	}

	var before int64
	if err := store.DB.QueryRowContext(ctx, `SELECT auth_epoch FROM iam.users WHERE id = $1`, userID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	if err := repository.UpdateTimeZone(ctx, store.DB, userID, "Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}

	if value, err := repository.UserTimeZone(ctx, store.DB, userID); err != nil || value != "Asia/Shanghai" {
		t.Fatalf("timezone = %q, err = %v; want Asia/Shanghai", value, err)
	}

	var after int64
	if err := store.DB.QueryRowContext(ctx, `SELECT auth_epoch FROM iam.users WHERE id = $1`, userID).Scan(&after); err != nil {
		t.Fatal(err)
	}

	if before != after {
		t.Fatalf("auth epoch changed from %d to %d; time zone must not bump the account version", before, after)
	}
}

// 空字符串必须存成 NULL，让读取端把它当作「未设置」而不是一个非法时区名。
func TestClearingTimeZoneStoresNull(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, userID, "tz-clear", "tz-clear", "清除时区", "", false); err != nil {
		t.Fatal(err)
	}

	if err := repository.UpdateTimeZone(ctx, store.DB, userID, "Europe/Berlin"); err != nil {
		t.Fatal(err)
	}

	if err := repository.UpdateTimeZone(ctx, store.DB, userID, ""); err != nil {
		t.Fatal(err)
	}

	var isNull bool
	if err := store.DB.QueryRowContext(ctx, `SELECT timezone IS NULL FROM iam.users WHERE id = $1`, userID).Scan(&isNull); err != nil {
		t.Fatal(err)
	}

	if !isNull {
		t.Fatal("cleared time zone is not NULL")
	}
}

// 迁移必须给 users 加上可空时区列，且默认无值。
func TestUserTimeZoneColumnIsNullable(t *testing.T) {
	store := testutil.Database(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := repository.InsertUser(ctx, store.DB, userID, "tz-default", "tz-default", "默认时区", "", false); err != nil {
		t.Fatal(err)
	}

	var nullable string
	if err := store.DB.QueryRowContext(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_schema='iam' AND table_name='users' AND column_name='timezone'`).Scan(&nullable); err != nil {
		t.Fatal(err)
	}

	if nullable != "YES" {
		t.Fatalf("users.timezone is_nullable = %q, want YES", nullable)
	}
}
