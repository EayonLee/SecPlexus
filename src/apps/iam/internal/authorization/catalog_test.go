package authorization_test

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
)

// 权限键是唯一稳定标识：目录必须能把它拆成 module.resource.action。
func TestCatalogDerivesFacetsFromKey(t *testing.T) {
	catalog := authorization.Catalog()
	if len(catalog) == 0 {
		t.Fatal("catalog is empty")
	}

	for _, permission := range catalog {
		module, resource, action, ok := authorization.ParsePermissionKey(permission.Key)
		if !ok {
			t.Fatalf("key %q is not module.resource.action", permission.Key)
		}

		if permission.Module != module || permission.ResourceType != resource || permission.Action != action {
			t.Fatalf("derived facets mismatch for %q", permission.Key)
		}

		if permission.Status != enum.PermissionStatusActive {
			t.Fatalf("catalog key %q must ship as active, got %q", permission.Key, permission.Status)
		}
	}
}

// 非法键在解析阶段就被拒绝，不会悄悄产生空模块或空资源。
func TestParsePermissionKeyRejectsMalformedKeys(t *testing.T) {
	for _, key := range []string{"", "iam", "iam.users", "iam.users.read.extra", "IAM.users.read", "iam..read", "iam.users.Read"} {
		if _, _, _, ok := authorization.ParsePermissionKey(key); ok {
			t.Fatalf("malformed key accepted: %q", key)
		}
	}
}
