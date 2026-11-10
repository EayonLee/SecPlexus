package authorization

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// 委派只有一条规则：操作者当前持有且目录标记 delegable 的权限。
func TestDelegationRequiresBothOldAndNewGrantsWithinAuthority(t *testing.T) {
	catalog := []Permission{
		{Key: "read", Status: enum.PermissionStatusActive, Delegable: true},
		{Key: "write", Status: enum.PermissionStatusActive, Delegable: true},
		{Key: "secret", Status: enum.PermissionStatusActive},
		{Key: "retired", Status: enum.PermissionStatusRetired, Delegable: true},
	}
	for _, tc := range []struct {
		name      string
		old, next []string
		allowed   bool
	}{
		{"own action", nil, []string{"read"}, true},
		{"empty custom role", []string{"read"}, nil, true},
		{"cannot strip stronger grants", []string{"write"}, nil, false},
		{"cannot grant outside authority", nil, []string{"write"}, false},
		{"cannot grant non-delegable action", nil, []string{"secret"}, false},
		{"cannot grant retired action", nil, []string{"retired"}, false},
		{"can remove retired grants", []string{"retired"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckDelegation([]string{"read", "retired"}, tc.old, tc.next, catalog)
			if (err == nil) != tc.allowed {
				t.Fatalf("expected allowed=%v", tc.allowed)
			}
		})
	}
}
