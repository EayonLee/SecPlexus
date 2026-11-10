package management_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler/management"
	// 绑定测试必须加载校验别名注册（iam_permissions/iam_role_ids）。
	_ "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
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
	requireInvalidBinding(t, "/roles", http.MethodPost, s.CreateRole, "/roles", `{"name":""}`)
}

func TestCreateRoleRejectsMalformedJSONBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/roles", http.MethodPost, s.CreateRole, "/roles", `{`)
}

func TestCreateRoleRejectsUnknownFieldBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/roles", http.MethodPost, s.CreateRole, "/roles", `{"name":"名称","unexpected":"field"}`)
}

func TestUpdateRoleRejectsMissingNameBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/roles/:id", http.MethodPatch, s.UpdateRole, "/roles/r", `{"description":"说明"}`)
}

// 角色没有并发版本字段，旧客户端回传必须按未知字段拒绝而不是静默忽略。
func TestSetRolePermissionsRejectsVersionFieldBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/roles/:id/permissions", http.MethodPut, s.SetRolePermissions, "/roles/r/permissions", `{"permissions":[],"version":"abc"}`)
}

func TestSetUserRolesRejectsNonUUIDBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/users/:id/roles", http.MethodPut, s.SetUserRoles, "/users/u/roles", `{"role_ids":["not-a-uuid"]}`)
}

func TestUpdateUserRejectsMissingStatusBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/users/:id", http.MethodPatch, s.UpdateUser, "/users/u", `{"display_name":"用户"}`)
}

func TestUpdateUserRejectsUnknownStatusBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/users/:id", http.MethodPatch, s.UpdateUser, "/users/u", `{"display_name":"用户","status":"archived"}`)
}

// 账号版本由服务端维护，客户端回传 auth_epoch 必须按未知字段拒绝。
func TestUpdateUserRejectsEpochFieldBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/users/:id", http.MethodPatch, s.UpdateUser, "/users/u", `{"display_name":"用户","status":"active","auth_epoch":"1"}`)
}

func TestListUsersRejectsOutOfRangeLimitBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/users", http.MethodGet, s.ListUsers, "/users?limit=1000", "")
}

func TestListUsersRejectsNegativeOffsetBeforeService(t *testing.T) {
	s := &management.Handler{}
	requireInvalidBinding(t, "/users", http.MethodGet, s.ListUsers, "/users?offset=-1", "")
}
