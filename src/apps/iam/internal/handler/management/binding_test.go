package management_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler/management"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// management 包只负责绑定参数并调用用例；错误响应由 handler.Server 的 Errors 中间件统一写出。
// 这些用例断言该包自己的契约：非法输入必须收集 ErrInvalid，且绝不触达业务用例。
// Service 为 nil，一旦请求越过绑定层就会 panic，因此「没有 panic」即为提前返回。
func requireInvalidBinding(t *testing.T, pattern, method string, handler gin.HandlerFunc, target, body string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var collected []error
	engine := gin.New()
	engine.Handle(method, pattern, func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("%s %s reached the service layer with a nil Service", method, target)
			}
		}()
		handler(c)
		for _, entry := range c.Errors {
			collected = append(collected, entry.Err)
		}
	})

	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), request)

	if len(collected) != 1 {
		t.Fatalf("%s %s collected %d errors, want exactly 1", method, target, len(collected))
	}

	if !errors.Is(collected[0], repository.ErrInvalid) {
		t.Fatalf("%s %s error = %v, want ErrInvalid", method, target, collected[0])
	}
}

func TestCreateRoleRejectsInvalidBodyBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/orgs/:org_id/roles", http.MethodPost, s.CreateRole, "/orgs/o/roles", `{"code":"","name":""}`)
}

func TestCreateRoleRejectsMalformedJSONBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/orgs/:org_id/roles", http.MethodPost, s.CreateRole, "/orgs/o/roles", `{`)
}

func TestCreateRoleRejectsUnknownFieldBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/orgs/:org_id/roles", http.MethodPost, s.CreateRole, "/orgs/o/roles", `{"code":"x1","name":"名称","unexpected":"field"}`)
}

func TestCreateOrganizationRejectsInvalidBodyBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/organizations", http.MethodPost, s.CreateOrganization, "/organizations", `{"name":"","slug":"","admin_user_id":""}`)
}

func TestCreateOrganizationRejectsInvalidSlugBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/organizations", http.MethodPost, s.CreateOrganization, "/organizations", `{"name":"组织","slug":"Bad Slug","admin_user_id":"u"}`)
}

func TestUpdateRoleRejectsMissingVersionBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/orgs/:org_id/roles/:id", http.MethodPatch, s.UpdateRole, "/orgs/o/roles/r", `{"name":"Role","status":"active"}`)
}

func TestUpdateRoleRejectsUnknownStatusBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/orgs/:org_id/roles/:id", http.MethodPatch, s.UpdateRole, "/orgs/o/roles/r", `{"name":"Role","status":"archived","authz_epoch":"1"}`)
}

func TestSetRolePermissionsRejectsNonNumericVersionBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/orgs/:org_id/roles/:id/permissions", http.MethodPut, s.SetRolePermissions, "/orgs/o/roles/r/permissions", `{"permissions":[],"version":"abc"}`)
}

func TestListOrganizationsRejectsOutOfRangeLimitBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/organizations", http.MethodGet, s.ListOrganizations, "/organizations?limit=1000", "")
}

func TestListOrganizationsRejectsNegativeOffsetBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/organizations", http.MethodGet, s.ListOrganizations, "/organizations?offset=-1", "")
}
