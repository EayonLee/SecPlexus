package authorization

import (
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

func TestAuthorizationBoundaries(t *testing.T) {
	p := iamclient.Principal{UserID: "user", AuthenticatedAt: time.Now()}
	permission := Permission{Key: "iam.roles.read", Scope: "organization", Status: "active"}
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
		permission Permission
	}{
		{"different organization", base, "other", permission},
		{"empty grants", Snapshot{Principal: p, OrganizationID: "org", OrganizationActive: true, MembershipActive: true}, "org", permission},
		{"platform requires protected identity", base, "", Permission{Scope: "platform", Status: "active"}},
		{"unknown scope", base, "org", Permission{Scope: "object", Status: "active"}},
		{"retired action", base, "org", Permission{Scope: "organization", Status: "retired"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Evaluate(tc.snapshot, tc.permission, tc.org, time.Now(), 5*time.Minute); err == nil {
				t.Fatal("authorization unexpectedly allowed")
			}
		})
	}
}

func TestSuperAdminStillRequiresOrganizationAndSecurityGates(t *testing.T) {
	p := iamclient.Principal{IsSuperAdmin: true, AuthenticatedAt: time.Now().Add(-time.Hour)}
	s := Snapshot{Principal: p, OrganizationID: "org", OrganizationActive: true}
	permission := Permission{Scope: "organization", Status: "active"}
	if _, err := Evaluate(s, permission, "", time.Now(), 5*time.Minute); err == nil {
		t.Fatal("unscoped business access allowed")
	}

	s.Principal = p
	permission.RequiresRecentAuth = true
	if _, err := Evaluate(s, permission, "org", time.Now(), 5*time.Minute); err == nil {
		t.Fatal("recent authentication bypassed")
	}
}
