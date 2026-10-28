package authorization

import (
	"testing"
	"time"

	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

func TestAuthorizationBoundaries(t *testing.T) {
	p := contract.Principal{UserID: "user", AuthenticatedAt: time.Now()}
	permission := contract.Permission{Key: "iam.roles.read", Scope: "organization", Status: "active"}
	base := Snapshot{
		Principal:          p,
		OrganizationID:     "org",
		OrganizationActive: true,
		MembershipActive:   true,
		Permissions:        []string{"iam.roles.read"},
	}
	if _, err := Evaluate(base, permission, "org", time.Now(), 5*time.Minute); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		snapshot   Snapshot
		org        string
		permission contract.Permission
	}{
		{"different organization", base, "other", permission},
		{"empty grants", Snapshot{Principal: p, OrganizationID: "org", OrganizationActive: true, MembershipActive: true}, "org", permission},
		{"platform requires protected identity", base, "", contract.Permission{Scope: "platform", Status: "active"}},
		{"unknown scope", base, "org", contract.Permission{Scope: "object", Status: "active"}},
		{"retired action", base, "org", contract.Permission{Scope: "organization", Status: "retired"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Evaluate(tc.snapshot, tc.permission, tc.org, time.Now(), 5*time.Minute); err == nil {
				t.Fatal("authorization unexpectedly allowed")
			}
		})
	}
}

func TestSuperAdminStillRequiresOrganizationAndSecurityGates(t *testing.T) {
	p := contract.Principal{IsSuperAdmin: true, AuthenticatedAt: time.Now().Add(-time.Hour)}
	s := Snapshot{Principal: p, OrganizationID: "org", OrganizationActive: true}
	permission := contract.Permission{Scope: "organization", Status: "active"}
	if _, err := Evaluate(s, permission, "", time.Now(), 5*time.Minute); err == nil {
		t.Fatal("unscoped business access allowed")
	}

	s.Principal = p
	permission.RequiresRecentAuth = true
	if _, err := Evaluate(s, permission, "org", time.Now(), 5*time.Minute); err == nil {
		t.Fatal("recent authentication bypassed")
	}
}
