package authorization

import (
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

func TestEvaluateOnlyChecksStatusAndPermissionKey(t *testing.T) {
	now := time.Now()
	p := iamclient.Principal{UserID: "user", AuthenticatedAt: now}
	permission := Permission{Key: "iam.roles.read", Status: enum.PermissionStatusActive}
	base := Snapshot{Principal: p, Permissions: []string{"iam.roles.read"}}

	if err := Evaluate(base, permission, now, 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	// deprecated 已不属于当前权限目录；任何非 active 状态都必须拒绝。
	deprecated := Permission{Key: "iam.roles.read", Status: enum.PermissionStatus("deprecated")}
	if err := Evaluate(base, deprecated, now, 5*time.Minute); err != ErrDenied {
		t.Fatalf("deprecated status returned %v", err)
	}

	for _, tc := range []struct {
		name       string
		snapshot   Snapshot
		permission Permission
	}{
		{"empty grants", Snapshot{Principal: p}, permission},
		{"other permission", base, Permission{Key: "iam.users.manage", Status: enum.PermissionStatusActive}},
		{"retired action", base, Permission{Key: "iam.roles.read", Status: enum.PermissionStatusRetired}},
		{"unknown status", base, Permission{Key: "iam.roles.read", Status: enum.PermissionStatus("draft")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Evaluate(tc.snapshot, tc.permission, now, 5*time.Minute); err == nil {
				t.Fatal("authorization unexpectedly allowed")
			}
		})
	}
}

// 近期认证只看账号自己的时间戳，没有任何超管旁路。
func TestRecentAuthenticationWindowIsEnforced(t *testing.T) {
	now := time.Now()
	permission := Permission{Key: "iam.users.manage", Status: enum.PermissionStatusActive, RequiresRecentAuth: true}
	snapshot := Snapshot{Principal: iamclient.Principal{UserID: "user", AuthenticatedAt: now}, Permissions: []string{"iam.users.manage"}}
	if err := Evaluate(snapshot, permission, now, 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	stale := now.Add(-6 * time.Minute)
	snapshot.Principal.AuthenticatedAt = stale
	if err := Evaluate(snapshot, permission, now, 5*time.Minute); err != ErrRecentAuthRequired {
		t.Fatalf("stale password confirmation returned %v", err)
	}

	// 二次认证后以新的确认时间重新计算窗口。
	snapshot.Principal.ReauthenticatedAt = &now
	if err := Evaluate(snapshot, permission, now, 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	// 未来时间戳说明时钟或数据异常，不能放宽要求。
	future := now.Add(time.Hour)
	snapshot.Principal.ReauthenticatedAt = &future
	if err := Evaluate(snapshot, permission, now, 5*time.Minute); err != ErrRecentAuthRequired {
		t.Fatalf("future confirmation returned %v", err)
	}

	// 没有认证时间的账号也拿不到敏感动作。
	snapshot.Principal = iamclient.Principal{UserID: "user"}
	if err := Evaluate(snapshot, permission, now, 5*time.Minute); err != ErrRecentAuthRequired {
		t.Fatalf("anonymous timestamp returned %v", err)
	}
}
