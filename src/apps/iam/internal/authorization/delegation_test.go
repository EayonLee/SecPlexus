package authorization

import (
	"testing"
)

func TestDelegationRequiresBothOldAndNewGrantsWithinAuthority(t *testing.T) {
	catalog := []Permission{{Key: "read", Scope: "organization", Status: "active", Delegable: true}, {Key: "write", Scope: "organization", Status: "active", Delegable: true}, {Key: "platform", Scope: "platform", Status: "active"}, {Key: "legacy", Scope: "organization", Status: "deprecated", Delegable: true}}
	for _, tc := range []struct {
		name      string
		old, next []string
		allowed   bool
	}{{"own action", nil, []string{"read"}, true}, {"empty custom role", []string{"read"}, nil, true}, {"cannot strip stronger grants", []string{"write"}, nil, false}, {"cannot grant outside authority", nil, []string{"write"}, false}, {"cannot grant platform actions", nil, []string{"platform"}, false}, {"can remove deprecated grants", []string{"legacy"}, nil, true}, {"cannot newly grant deprecated actions", nil, []string{"legacy"}, false}} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckDelegation([]string{"read", "legacy"}, tc.old, tc.next, catalog)
			if (err == nil) != tc.allowed {
				t.Fatalf("expected allowed=%v", tc.allowed)
			}
		})
	}
}
