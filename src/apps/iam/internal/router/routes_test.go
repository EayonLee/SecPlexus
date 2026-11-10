package router_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
)

// 授权只有「用户 → 角色 → 权限」，公开路由必须全部扁平且不含组织段。
func TestUserRoutesAreFlat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.RegisterUserRoutes(engine, &handler.Server{})

	got := make([]string, 0, len(engine.Routes()))
	for _, route := range engine.Routes() {
		if strings.Contains(route.Path, "orgs") || strings.Contains(route.Path, ":org_id") {
			t.Fatalf("organization route still registered: %s %s", route.Method, route.Path)
		}

		got = append(got, route.Method+" "+route.Path)
	}

	sort.Strings(got)
	want := []string{
		"DELETE /api/v1/iam/roles/:id",
		"DELETE /api/v1/iam/users/:id",
		"GET /api/v1/iam/audit-events",
		"GET /api/v1/iam/auth/context",
		"GET /api/v1/iam/me",
		"GET /api/v1/iam/permissions",
		"GET /api/v1/iam/roles",
		"GET /api/v1/iam/roles/:id",
		"GET /api/v1/iam/users",
		"GET /api/v1/iam/users/:id",
		"GET /health/ready",
		"PATCH /api/v1/iam/me",
		"PATCH /api/v1/iam/roles/:id",
		"PATCH /api/v1/iam/users/:id",
		"POST /api/v1/iam/auth/login",
		"POST /api/v1/iam/auth/logout",
		"POST /api/v1/iam/auth/reauthenticate",
		"POST /api/v1/iam/me/password",
		"POST /api/v1/iam/roles",
		"POST /api/v1/iam/users",
		"POST /api/v1/iam/users/:id/reset-password",
		"PUT /api/v1/iam/roles/:id/permissions",
		"PUT /api/v1/iam/users/:id/roles",
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("routes differ\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// 旧组织前缀必须已经没有任何路由。
func TestLegacyOrganizationPathsAreNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.RegisterUserRoutes(engine, &handler.Server{})

	for _, route := range engine.Routes() {
		if strings.HasPrefix(route.Path, "/api/v1/iam/orgs") || strings.HasPrefix(route.Path, "/api/v1/iam/organizations") {
			t.Fatalf("legacy organization route registered: %s %s", route.Method, route.Path)
		}
	}
}
