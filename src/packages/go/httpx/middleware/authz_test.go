package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// newGuard 起一个假 IAM：authorize 返回给定决策，audit 记录收到的事实。
func newGuard(t *testing.T, allowed bool, status int) (*gin.Engine, *[]iamclient.AuditEvent) {
	t.Helper()

	audits := &[]iamclient.AuditEvent{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/iam/authorize":
			w.Header().Set("Content-Type", "application/json")
			if status != 0 {
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": "permission_denied", "message": "denied", "data": nil,
					"request_id": "00000000-0000-4000-8000-000000000001",
				})

				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "ok", "message": "ok",
				"data": map[string]any{
					"allowed": allowed, "decision_id": "00000000-0000-4000-8000-000000000002",
					"principal": map[string]any{"user_id": "user-1", "session_id": "s1", "username": "alice"},
				},
				"request_id": "00000000-0000-4000-8000-000000000003",
			})
		case "/api/v1/iam/audit-events":
			var event iamclient.AuditEvent
			_ = json.NewDecoder(r.Body).Decode(&event)
			*audits = append(*audits, event)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client, err := iamclient.New(server.URL, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	client = client.WithWebOrigin("https://console.example")

	gin.SetMode(gin.TestMode)
	engine := middleware.NewEngine()
	engine.POST("/api/v1/ingest/inputs/:id", middleware.Guard(client, iamclient.PermissionIngestInputWrite, "input"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	engine.Use()

	return engine, audits
}

// 无权限：403，且留下 denied 审计。
func TestGuardDeniesWithoutPermission(t *testing.T) {
	engine, audits := newGuard(t, false, http.StatusForbidden)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, trustedRequest("/api/v1/ingest/inputs/x1"))

	if response.Code != http.StatusForbidden {
		t.Fatalf("状态 = %d，期望 403", response.Code)
	}
	if len(*audits) != 1 || (*audits)[0].Outcome != "denied" {
		t.Fatalf("审计 = %#v", *audits)
	}
	if (*audits)[0].TargetID != "x1" {
		t.Fatalf("target_id = %q，期望路由参数", (*audits)[0].TargetID)
	}
}

// 通过：放行并写 success 审计。
func TestGuardAllowsAndAuditsSuccess(t *testing.T) {
	engine, audits := newGuard(t, true, 0)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, trustedRequest("/api/v1/ingest/inputs/x1"))

	if response.Code != http.StatusOK {
		t.Fatalf("状态 = %d，期望 200", response.Code)
	}
	if len(*audits) != 1 || (*audits)[0].Outcome != "success" {
		t.Fatalf("审计 = %#v", *audits)
	}
	if (*audits)[0].Action != iamclient.PermissionIngestInputWrite {
		t.Fatalf("action = %q，期望来自生成目录", (*audits)[0].Action)
	}
}

// IAM 不可达：503，不放行。
func TestGuardFailsClosedWhenIAMUnavailable(t *testing.T) {
	engine, audits := newGuard(t, true, http.StatusInternalServerError)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, trustedRequest("/api/v1/ingest/inputs/x1"))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态 = %d，期望 503", response.Code)
	}
	if len(*audits) != 0 {
		t.Fatalf("不可达时不应写审计: %#v", *audits)
	}
}

// trustedRequest 构造一个通过 CSRF 防护的同源请求。
func trustedRequest(path string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")

	return request
}

// 回归：缺 CSRF 头的状态变更请求必须被拒绝，且不调用 IAM。
func TestGuardRejectsMissingCSRFToken(t *testing.T) {
	engine, audits := newGuard(t, true, 0)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/inputs/x1", nil)
	request.Header.Set("Origin", "https://console.example")

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("状态 = %d，期望 403", response.Code)
	}
	if len(*audits) != 0 {
		t.Fatal("CSRF 被拒时不应写审计")
	}
}

// 回归：跨站 Origin 必须被拒绝（浏览器无法伪造 Origin）。
func TestGuardRejectsCrossOrigin(t *testing.T) {
	engine, audits := newGuard(t, true, 0)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/inputs/x1", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("X-CSRF-Token", "csrf-token")

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("状态 = %d，期望 403", response.Code)
	}
	if len(*audits) != 0 {
		t.Fatal("跨站请求不应写审计")
	}
}

// 读操作成功不写审计：否则审计被浏览行为淹没，管理留痕反而找不到。
func TestGuardSkipsAuditForSuccessfulRead(t *testing.T) {
	audits := &[]iamclient.AuditEvent{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "audit") {
			var event iamclient.AuditEvent
			_ = json.NewDecoder(r.Body).Decode(&event)
			*audits = append(*audits, event)
			w.WriteHeader(http.StatusNoContent)

			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "ok", "message": "ok",
			"data": map[string]any{
				"allowed": true, "decision_id": "00000000-0000-4000-8000-000000000002",
				"principal": map[string]any{"user_id": "u1", "session_id": "s1", "username": "alice"},
			},
			"request_id": "00000000-0000-4000-8000-000000000003",
		})
	}))
	defer server.Close()

	client, err := iamclient.New(server.URL, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	engine := middleware.NewEngine()
	engine.GET("/api/v1/ingest/inputs", middleware.Guard(client, iamclient.PermissionIngestInputRead, "input"),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ingest/inputs", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("状态 = %d", response.Code)
	}
	if len(*audits) != 0 {
		t.Fatalf("读成功不应写审计: %#v", *audits)
	}
}
