//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/testutil"
	cachekey "github.com/EayonLee/SecPlexus/src/packages/go/cache/key"
)

// apiClient 为单个测试保存独立 Cookie 和 CSRF 状态。
type apiClient struct {
	t                  *testing.T
	client             *http.Client
	base, origin, csrf string
	sourceIP           string
}

// exchange 发送请求并返回成功结果的 data 与结果码。
func (c *apiClient) exchange(method, path string, input any) (map[string]any, string, int) {
	c.t.Helper()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			c.t.Fatal(err)
		}

		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		c.t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	req.Header.Set("X-Forwarded-For", c.sourceIP)
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}

	response, err := c.client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}

	defer response.Body.Close()

	data := map[string]any{}
	code := ""
	if response.StatusCode != 204 {
		var result struct {
			Code      string         `json:"code"`
			Message   string         `json:"message"`
			Data      map[string]any `json:"data"`
			RequestID string         `json:"request_id"`
		}
		if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
			c.t.Fatal(err)
		}

		if result.RequestID == "" || result.Code == "" || result.Message == "" {
			c.t.Fatal("incomplete Result")
		}

		if response.StatusCode < 400 && result.Code != "ok" {
			c.t.Fatalf("expected success Result, got %s", result.Code)
		}

		if response.StatusCode >= 400 && (result.Code == "ok" || result.Data != nil) {
			c.t.Fatal("invalid failure Result")
		}

		data, code = result.Data, result.Code
	}

	return data, code, response.StatusCode
}

func (c *apiClient) request(method, path string, input any, status int) map[string]any {
	c.t.Helper()
	data, _, got := c.exchange(method, path, input)
	if got != status {
		c.t.Fatalf("%s %s: expected %d, got %d", method, path, status, got)
	}

	return data
}

// code 断言失败响应使用稳定的业务结果码，而不只是 HTTP 状态。
func (c *apiClient) code(method, path string, input any, status int, want string) {
	c.t.Helper()
	_, got, actual := c.exchange(method, path, input)
	if actual != status || got != want {
		c.t.Fatalf("%s %s: expected %d %s, got %d %s", method, path, status, want, actual, got)
	}
}

func (c *apiClient) context() {
	c.csrf = c.request("GET", "/api/v1/iam/auth/context", nil, 200)["csrf_token"].(string)
}

func (c *apiClient) login(username, password string) {
	c.context()
	c.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": username, "password": password}, 200)
	c.context()
}

func (c *apiClient) change(old, next string) {
	c.request("POST", "/api/v1/iam/me/password", map[string]string{"old_password": old, "new_password": next}, 204)
	c.context()
}

func newAPIClient(t *testing.T, base, origin string) *apiClient {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	parsed, _ := url.Parse(base)
	port, _ := strconv.ParseUint(parsed.Port(), 10, 16)
	return &apiClient{t: t, base: base, origin: origin, sourceIP: "2001:db8::" + strconv.FormatUint(port, 16), client: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// builtinRole 从角色列表读取内置角色行。
func (c *apiClient) builtinRole() map[string]any {
	c.t.Helper()
	items := c.request("GET", "/api/v1/iam/roles?limit=100", nil, 200)["items"].([]any)
	for _, item := range items {
		row := item.(map[string]any)
		if row["builtin"] == true {
			return row
		}
	}

	c.t.Fatal("builtin role missing from role list")
	return nil
}

// httpFixture 持有本用例独占的服务、数据和调用凭证。
type httpFixture struct {
	redis                         *redis.Client
	server                        *handler.Server
	public                        *httptest.Server
	initial, password, credential string
	admin, member                 string
}

func newFixture(t *testing.T, configure func(*config.Config)) *httpFixture {
	store := testutil.Database(t)
	ctx := context.Background()
	adminUsername := "admin" + uuid.NewString()[:8]
	memberUsername := "auditor" + uuid.NewString()[:8]
	initial := "isolated initial password"
	password := initial
	if err := store.Seed(ctx, adminUsername, initial); err != nil {
		t.Fatal(err)
	}

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	public := httptest.NewUnstartedServer(nil)
	cfg.Auth.WebOrigin = "http://" + public.Listener.Addr().String()
	cfg.Auth.CSRFKey = hex.EncodeToString(key)
	cfg.Bootstrap.Username = adminUsername
	cfg.Bootstrap.Password = initial
	cfg.App.TrustedProxyCIDRs = []string{"127.0.0.1/32"}
	credential := hex.EncodeToString(key) + hex.EncodeToString(key)
	cfg.ServiceClients = map[string]config.ServiceClient{"console": {Token: credential, Actions: []string{"iam.users.read"}}}
	options, err := redis.ParseURL(os.Getenv("SPX_TEST_REDIS_URL"))
	if err != nil {
		t.Fatal(err)
	}

	redisClient := redis.NewClient(options)
	t.Cleanup(func() { redisClient.Close() })
	// 测试使用唯一账号和来源地址；只删除本用例会创建的精确 key。
	_, portText, _ := net.SplitHostPort(public.Listener.Addr().String())
	portNumber, _ := strconv.ParseUint(portText, 10, 16)
	sourceIP := "2001:db8::" + strconv.FormatUint(portNumber, 16)
	t.Cleanup(func() {
		cleanup := redis.NewClient(options)
		defer cleanup.Close()
		keys := []string{}
		digest := sha256.Sum256([]byte(sourceIP))
		keys = append(keys, cachekey.IAMLoginIP.Format(hex.EncodeToString(digest[:])))
		accounts := []string{adminUsername, memberUsername}
		connection, err := pgx.Connect(context.Background(), store.Pool.Config().ConnConfig.ConnString())
		if err != nil {
			t.Error("cannot connect for owned key cleanup")
			return
		}
		rows, err := connection.Query(context.Background(), "SELECT id::text FROM iam.user_sessions")
		if err != nil {
			connection.Close(context.Background())
			t.Error("cannot read owned sessions for cleanup")
			return
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Error(err)
				break
			}
			accounts = append(accounts, "password:"+id)
		}
		rows.Close()
		connection.Close(context.Background())
		for _, account := range accounts {
			digest := sha256.Sum256([]byte(account))
			keys = append(keys, cachekey.IAMLoginAccount.Format(hex.EncodeToString(digest[:])))
		}
		if err := cleanup.Del(context.Background(), keys...).Err(); err != nil {
			t.Error("cannot clean owned rate-limit keys")
		}
	})

	if configure != nil {
		configure(&cfg)
	}

	security, err := iammiddleware.NewSecurity(cfg)
	if err != nil {
		t.Fatal(err)
	}

	service := &service.Service{Store: store, Sessions: authentication.NewSessions(store, cfg), Config: cfg}
	limiter, err := authentication.NewLimiter(redisClient, 60, 20)
	if err != nil {
		t.Fatal(err)
	}
	server, err := handler.New(service, security, limiter, func(ctx context.Context) error {
		if err := store.Pool.Ping(ctx); err != nil {
			return err
		}

		return redisClient.Ping(ctx).Err()
	})
	if err != nil {
		t.Fatal(err)
	}

	public.Config.Handler = router.Handler(server)
	public.Start()
	t.Cleanup(public.Close)
	return &httpFixture{redis: redisClient, admin: adminUsername, member: memberUsername,
		server:     server,
		public:     public,
		initial:    initial,
		password:   password,
		credential: credential,
	}
}

func TestFlatIdentityRoleAndUserLifecycle(t *testing.T) {
	f := newFixture(t, nil)
	public, initial, credential := f.public, f.initial, f.credential

	admin := newAPIClient(t, public.URL, public.URL)
	admin.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": initial}, 403)
	admin.login(f.admin, initial)
	me := admin.request("GET", "/api/v1/iam/me", nil, 200)
	principal := me["principal"].(map[string]any)
	userID := principal["user_id"].(string)
	for _, gone := range []string{"organizations", "org_id", "capabilities", "platform_capabilities"} {
		if _, exists := me[gone]; exists {
			t.Fatalf("/me still exposes %q", gone)
		}
	}

	if principal["configured"] != true || principal["username"] != f.admin || principal["session_id"] == "" || principal["auth_epoch"] == "" {
		t.Fatalf("unexpected principal %v", principal)
	}

	roles := me["roles"].([]any)
	if len(roles) != 1 || roles[0].(map[string]any)["name"] != "超级管理员" {
		t.Fatalf("configured administrator roles = %v", roles)
	}

	permissions := me["permissions"].([]any)
	if len(permissions) != 7 {
		t.Fatalf("configured administrator permissions = %v", permissions)
	}

	// 内置角色由启动同步写入：可查看、不可改、不可删。
	builtin := admin.builtinRole()
	if builtin["name"] != "超级管理员" || builtin["description"] != "" || len(builtin["permissions"].([]any)) != 7 || builtin["version"] != nil {
		t.Fatalf("unexpected builtin role %v", builtin)
	}

	builtinID := builtin["id"].(string)
	admin.code("PATCH", "/api/v1/iam/roles/"+builtinID, map[string]any{"name": "改名"}, 409, "iam.builtin_role_immutable")
	admin.code("PUT", "/api/v1/iam/roles/"+builtinID+"/permissions", map[string]any{"permissions": []string{"iam.audit.read"}}, 409, "iam.builtin_role_immutable")
	admin.code("DELETE", "/api/v1/iam/roles/"+builtinID, nil, 409, "iam.builtin_role_immutable")

	// 配置托管账号不可删除、停用、重置密码；自身改密也被拒绝。
	admin.code("PATCH", "/api/v1/iam/users/"+userID, map[string]any{"display_name": f.admin, "status": "disabled"}, 409, "iam.configured_user_immutable")
	admin.code("DELETE", "/api/v1/iam/users/"+userID, nil, 409, "iam.configured_user_immutable")
	admin.code("POST", "/api/v1/iam/users/"+userID+"/reset-password", map[string]any{"password": "another reset password"}, 409, "iam.configured_user_immutable")
	admin.code("POST", "/api/v1/iam/me/password", map[string]any{"old_password": initial, "new_password": "another changed password"}, 409, "iam.configured_user_immutable")

	// 权限目录来自数据库投影，超级管理员可委派全部 7 项。
	catalog := admin.request("GET", "/api/v1/iam/permissions", nil, 200)["items"].([]any)
	if len(catalog) != 7 {
		t.Fatalf("permission catalog has %d items", len(catalog))
	}

	for _, item := range catalog {
		entry := item.(map[string]any)
		permission := entry["permission"].(map[string]any)
		if permission["scope"] != nil {
			t.Fatal("permission catalog still exposes a scope field")
		}

		if entry["can_delegate"] != true {
			t.Fatalf("configured administrator cannot delegate %v", permission["key"])
		}
	}

	// 全局角色名唯一：与内置角色同名、与已有角色同名都必须冲突。
	admin.code("POST", "/api/v1/iam/roles", map[string]any{"name": "超级管理员", "permissions": []string{}}, 409, "iam.role_name_taken")
	roleID := admin.request("POST", "/api/v1/iam/roles", map[string]any{"name": "审计员", "permissions": []string{"iam.audit.read"}}, 201)["id"].(string)
	admin.code("POST", "/api/v1/iam/roles", map[string]any{"name": "审计员", "permissions": []string{}}, 409, "iam.role_name_taken")

	// 创建用户并在同一事务内授予角色。
	memberPassword := "isolated member password"
	memberID := admin.request("POST", "/api/v1/iam/users", map[string]any{"username": f.member, "display_name": "审计用户", "password": memberPassword, "role_ids": []string{roleID}}, 201)["id"].(string)
	user := admin.request("GET", "/api/v1/iam/users/"+memberID, nil, 200)
	if user["username"] != f.member || user["configured"] != false || len(user["roles"].([]any)) != 1 {
		t.Fatalf("unexpected user projection %v", user)
	}

	// 角色仍被持有：删除必须返回专门的冲突码。
	admin.code("DELETE", "/api/v1/iam/roles/"+roleID, nil, 409, "iam.role_in_use")

	// PUT /users/:id/roles 生效后，成员自身 /me 的权限随之变化。
	member := newAPIClient(t, public.URL, public.URL)
	member.login(f.member, memberPassword)
	if got := member.request("GET", "/api/v1/iam/me", nil, 200)["permissions"].([]any); len(got) != 1 || got[0] != "iam.audit.read" {
		t.Fatalf("member permissions = %v", got)
	}
	member.request("GET", "/api/v1/iam/audit-events", nil, 200)
	member.request("GET", "/api/v1/iam/users", nil, 403)

	admin.request("PUT", "/api/v1/iam/users/"+memberID+"/roles", map[string]any{"role_ids": []string{}}, 204)
	if got := member.request("GET", "/api/v1/iam/me", nil, 200); len(got["permissions"].([]any)) != 0 || len(got["roles"].([]any)) != 0 {
		t.Fatalf("member grants survived role removal: %v", got)
	}
	member.request("GET", "/api/v1/iam/audit-events", nil, 403)

	// 角色不再被持有后可以删除，且版本正确时才允许。
	admin.request("DELETE", "/api/v1/iam/roles/"+roleID, nil, 204)

	// 列表投影包含角色与总数，搜索按用户名、显示名或邮箱模糊匹配。
	list := admin.request("GET", "/api/v1/iam/users?q="+f.member, nil, 200)
	if list["total"] != float64(1) {
		t.Fatalf("user search total = %v", list["total"])
	}

	row := list["items"].([]any)[0].(map[string]any)
	if row["id"] != memberID || row["roles"] == nil || row["auth_epoch"] == "" {
		t.Fatalf("unexpected user list row %v", row)
	}

	// 删除普通用户不影响调用者会话。
	admin.request("DELETE", "/api/v1/iam/users/"+memberID, nil, 204)
	admin.request("GET", "/api/v1/iam/users/"+memberID, nil, 404)
	admin.request("GET", "/api/v1/iam/me", nil, 200)

	// 服务间授权只接收 action；返回固定 platform 范围。
	request, _ := http.NewRequest("GET", public.URL, nil)
	cookieHeader := ""
	for _, cookie := range admin.client.Jar.Cookies(request.URL) {
		if cookieHeader != "" {
			cookieHeader += "; "
		}

		cookieHeader += cookie.Name + "=" + cookie.Value
	}

	call := func(input map[string]any, want int) {
		t.Helper()
		body, _ := json.Marshal(input)
		req, _ := http.NewRequest("POST", public.URL+"/api/v1/iam/authorize", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+credential)
		req.Header.Set("Content-Type", "application/json")
		response, err := public.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer response.Body.Close()

		if len(response.Cookies()) != 0 {
			t.Fatal("service authorization must not set browser cookies")
		}

		if response.StatusCode != want {
			t.Fatalf("service authorization expected %d, got %d", want, response.StatusCode)
		}
	}

	base := map[string]any{"cookie": cookieHeader, "method": "GET", "path": "/api/v1/console/context", "action": "iam.users.read", "origin": public.URL, "csrf_token": admin.csrf}
	call(base, 200)
	withScope := map[string]any{}
	for key, value := range base {
		withScope[key] = value
	}

	withScope["org_id"] = uuid.NewString()
	call(withScope, 400)
	withoutCookie := map[string]any{}
	for key, value := range base {
		withoutCookie[key] = value
	}

	withoutCookie["cookie"] = ""
	call(withoutCookie, 401)

	// 可信服务转交的写请求仍须通过 Origin 和 CSRF 校验。
	write := map[string]any{}
	for key, value := range base {
		write[key] = value
	}

	write["method"], write["csrf_token"] = "POST", "invalid"
	call(write, 403)

	// 服务凭证不能代替用户会话访问用户 API，也不能以用户 Cookie 代替服务凭证。
	req, _ := http.NewRequest("GET", public.URL+"/api/v1/iam/me", nil)
	req.Header.Set("Authorization", "Bearer "+credential)
	response, err := public.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("service credential authenticated a user route")
	}

	req, _ = http.NewRequest("POST", public.URL+"/api/v1/iam/authorize", bytes.NewReader([]byte(`{"action":"iam.users.read","method":"GET","path":"/","cookie":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookieHeader)
	response, err = public.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("user cookie crossed the service authentication boundary")
	}

	// 组织路由与旧的授权入口都不再存在。
	for _, gone := range []string{"/api/v1/iam/organizations", "/api/v1/iam/orgs/" + uuid.NewString() + "/users", "/api/v1/iam/me/sessions", "/internal/v1/iam/authorize"} {
		admin.request("GET", gone, nil, 404)
	}

	// 审计列表的字段是前端消费的契约：时间字段必须叫 occurred_at，且没有范围维度。
	audit := admin.request("GET", "/api/v1/iam/audit-events?limit=50", nil, 200)
	items := audit["items"].([]any)
	if len(items) == 0 {
		t.Fatal("audit list is empty; the contract assertions below would not run")
	}

	for _, item := range items {
		row := item.(map[string]any)
		if _, exists := row["org_id"]; exists {
			t.Fatal("audit event still exposes org_id")
		}

		if _, exists := row["time"]; exists {
			t.Fatal("audit event renamed occurred_at to time")
		}

		if _, err := time.Parse(time.RFC3339Nano, row["occurred_at"].(string)); err != nil {
			t.Fatalf("audit occurred_at is not RFC3339: %v", err)
		}
	}

	var orgColumns int
	if err := f.server.Service.Store.Pool.QueryRow(context.Background(), "SELECT count(*) FROM information_schema.columns WHERE table_schema='iam' AND table_name='audit_events' AND column_name LIKE '%org%'").Scan(&orgColumns); err != nil {
		t.Fatal(err)
	}

	if orgColumns != 0 {
		t.Fatalf("audit_events still has %d organization columns", orgColumns)
	}

	admin.request("POST", "/api/v1/iam/auth/logout", nil, 204)
	admin.request("GET", "/api/v1/iam/me", nil, 401)
	t.Log(fmt.Sprintf("flat accounts, builtin protections, role conflicts and internal authorization verified for %s", f.admin))
}

func TestPatchPreservesOmittedFieldsAndClearsExplicitEmptyValues(t *testing.T) {
	f := newFixture(t, nil)
	admin := newAPIClient(t, f.public.URL, f.public.URL)
	admin.login(f.admin, f.initial)
	admin.login(f.admin, f.password)
	roleID := admin.request("POST", "/api/v1/iam/roles",
		map[string]any{"name": "只读用户", "permissions": []string{"iam.users.read"}}, 201)["id"].(string)

	user := admin.request("POST", "/api/v1/iam/users", map[string]any{"username": "patch_member", "display_name": "Before", "email": "patch@example.com", "password": f.password, "role_ids": []string{roleID}}, 201)
	userPath := "/api/v1/iam/users/" + user["id"].(string)
	// 改显示名称不改变账号版本，因此不打断该用户已有的会话。
	beforeEpoch := admin.request("GET", userPath, nil, 200)["auth_epoch"]
	admin.request("PATCH", userPath, map[string]any{"display_name": "After", "status": "active"}, 204)
	if got := admin.request("GET", userPath, nil, 200)["auth_epoch"]; got != beforeEpoch {
		t.Fatalf("profile change bumped auth_epoch: %v -> %v", beforeEpoch, got)
	}
	if got := admin.request("GET", userPath, nil, 200)["email"]; got != "patch@example.com" {
		t.Fatalf("omitted email changed stored value: %v", got)
	}
	admin.request("PATCH", userPath, map[string]any{"display_name": "After", "status": "active", "email": ""}, 204)
	if got := admin.request("GET", userPath, nil, 200)["email"]; got != nil {
		t.Fatalf("explicit empty email was not cleared: %v", got)
	}
	admin.request("PATCH", userPath, map[string]any{"display_name": "After", "status": "active", "email": nil}, 400)
	role := admin.request("POST", "/api/v1/iam/roles", map[string]any{"name": "Before", "description": "Preserve me", "permissions": []string{"iam.roles.read"}}, 201)
	rolePath := "/api/v1/iam/roles/" + role["id"].(string)
	admin.request("PATCH", rolePath, map[string]any{"name": "After"}, 204)
	if got := admin.request("GET", rolePath, nil, 200)["description"]; got != "Preserve me" {
		t.Fatalf("omitted description changed stored value: %v", got)
	}
	admin.request("PATCH", rolePath, map[string]any{"name": "After", "description": ""}, 204)
	if got := admin.request("GET", rolePath, nil, 200)["description"]; got != "" {
		t.Fatalf("explicit empty description was not cleared: %v", got)
	}
	admin.request("PATCH", rolePath, map[string]any{"name": "After", "description": nil}, 400)
}

// TestDelegationLimitsAndPreview 固定委派上限：只能授予自己持有且 delegable 的权限。
func TestDelegationLimitsAndPreview(t *testing.T) {
	f := newFixture(t, nil)
	admin := newAPIClient(t, f.public.URL, f.public.URL)
	admin.login(f.admin, f.initial)
	admin.login(f.admin, f.password)

	readRole := admin.request("POST", "/api/v1/iam/roles", map[string]any{"name": "用户只读", "permissions": []string{"iam.users.read"}}, 201)["id"].(string)
	auditRole := admin.request("POST", "/api/v1/iam/roles", map[string]any{"name": "审计员", "permissions": []string{"iam.audit.read"}}, 201)["id"].(string)
	delegatorRole := admin.request("POST", "/api/v1/iam/roles", map[string]any{"name": "用户管理员", "permissions": []string{"iam.users.read", "iam.users.manage", "iam.users.roles"}}, 201)["id"].(string)

	delegatorPassword := "delegated manager password"
	delegatorUsername := "delegate" + uuid.NewString()[:6]
	delegatorID := admin.request("POST", "/api/v1/iam/users", map[string]any{"username": delegatorUsername, "display_name": "委派管理员", "password": delegatorPassword, "role_ids": []string{delegatorRole}}, 201)["id"].(string)

	delegator := newAPIClient(t, f.public.URL, f.public.URL)
	delegator.login(delegatorUsername, delegatorPassword)
	// 没有 iam.roles.manage，不能创建或修改角色。
	delegator.request("POST", "/api/v1/iam/roles", map[string]any{"name": "偷建角色", "permissions": []string{"iam.users.read"}}, 403)
	delegator.request("PUT", "/api/v1/iam/roles/"+delegatorRole+"/permissions", map[string]any{"permissions": []string{"iam.users.read"}}, 403)

	// 可以授予自己持有的只读角色，但不能授予未持有的审计角色。
	memberPassword := "delegated member password"
	delegator.request("POST", "/api/v1/iam/users", map[string]any{"username": "sub" + uuid.NewString()[:8], "display_name": "下属用户", "password": memberPassword, "role_ids": []string{auditRole}}, 403)
	memberUsername := "sub" + uuid.NewString()[:8]
	delegator.request("POST", "/api/v1/iam/users", map[string]any{"username": memberUsername, "display_name": "下属用户", "password": memberPassword, "role_ids": []string{readRole}}, 201)

	// 不能把自己的账号提升到内置超级管理员角色。
	builtin := admin.builtinRole()
	delegator.request("PUT", "/api/v1/iam/users/"+delegatorID+"/roles", map[string]any{"role_ids": []string{builtin["id"].(string)}}, 403)

	// 预览与写入共享委派边界，且不落库。
	if got := delegator.request("GET", "/api/v1/iam/me", nil, 200)["permissions"].([]any); len(got) != 3 {
		t.Fatalf("delegator permissions = %v", got)
	}

	sub := newAPIClient(t, f.public.URL, f.public.URL)
	sub.login(memberUsername, memberPassword)
	if got := sub.request("GET", "/api/v1/iam/me", nil, 200)["permissions"].([]any); len(got) != 1 || got[0] != "iam.users.read" {
		t.Fatalf("subordinate permissions = %v", got)
	}
	sub.request("GET", "/api/v1/iam/users", nil, 200)
	sub.request("GET", "/api/v1/iam/audit-events", nil, 403)
}

// TestManagementBindingRejectsInvalidInputBeforeService 固定管理接口的输入边界：
// 非法 body 与越界分页必须在触达业务用例之前被拒绝，返回 invalid_request 而不是 500。
func TestManagementBindingRejectsInvalidInputBeforeService(t *testing.T) {
	f := newFixture(t, nil)
	admin := newAPIClient(t, f.public.URL, f.public.URL)
	admin.login(f.admin, f.initial)
	admin.login(f.admin, f.password)

	admin.request("POST", "/api/v1/iam/roles", map[string]any{"name": ""}, 400)
	admin.request("PATCH", "/api/v1/iam/roles/00000000-0000-4000-8000-000000000000", map[string]any{"description": "缺名称"}, 400)
	admin.request("PUT", "/api/v1/iam/roles/00000000-0000-4000-8000-000000000000/permissions", map[string]any{"permissions": []string{}, "version": "abc"}, 400)
	admin.request("GET", "/api/v1/iam/users?limit=1000", nil, 400)
	admin.request("POST", "/api/v1/iam/users/00000000-0000-4000-8000-000000000000/reset-password", map[string]any{"password": "short"}, 400)
	// 审计列表使用游标分页，offset 不是它的参数；改断言真实的绑定边界。
	admin.request("GET", "/api/v1/iam/audit-events?limit=1000", nil, 400)
	admin.request("GET", "/api/v1/iam/audit-events?from=2026-13-01", nil, 400)
	admin.request("GET", "/api/v1/iam/roles/not-a-uuid", nil, 400)
}

// 密码确认失败不累计登录失败：否则一次输错的 sudo 确认会把账号锁在门外。
func TestReauthenticateFailureDoesNotLockAccount(t *testing.T) {
	f := newFixture(t, nil)
	admin := newAPIClient(t, f.public.URL, f.public.URL)
	admin.login(f.admin, f.initial)
	admin.login(f.admin, f.password)

	for range 3 {
		admin.request("POST", "/api/v1/iam/auth/reauthenticate", map[string]any{"password": "wrong confirmation"}, 400)
	}

	var failed int32
	if err := f.server.Service.Store.Pool.QueryRow(context.Background(),
		`SELECT c.failed_attempts FROM iam.password_credentials c JOIN iam.users u ON u.id=c.user_id WHERE u.configured`).Scan(&failed); err != nil {
		t.Fatal(err)
	}

	if failed != 0 {
		t.Fatalf("failed_attempts = %d after password confirmation, want 0", failed)
	}

	// 正确密码仍可完成确认，说明账号没有被锁。
	admin.request("POST", "/api/v1/iam/auth/reauthenticate", map[string]any{"password": f.password}, 200)
}
