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
	"strings"
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

func (c *apiClient) request(method, path string, input any, status int) map[string]any {
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

		if status < 400 && result.Code != "ok" {
			c.t.Fatalf("expected success Result, got %s", result.Code)
		}

		if status >= 400 && (result.Code == "ok" || result.Data != nil) {
			c.t.Fatal("invalid failure Result")
		}

		data = result.Data
	}

	if response.StatusCode != status {
		c.t.Fatalf("%s %s: expected %d, got %d", method, path, status, response.StatusCode)
	}

	return data
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
	adminUsername := "admin-" + uuid.NewString()
	memberUsername := "auditor-" + uuid.NewString()
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
	cfg.ServiceClients = map[string]config.ServiceClient{"console": {Token: credential, Actions: []string{"iam.organizations.read"}}}
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
	limiter, err := authentication.NewLimiter(redisClient)
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

func TestIdentityAndCustomRoleLifecycle(t *testing.T) {
	f := newFixture(t, nil)
	public, initial, password, credential := f.public, f.initial, f.password, f.credential

	admin := newAPIClient(t, public.URL, public.URL)
	admin.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": initial}, 403)
	admin.login(f.admin, initial)
	admin.request("GET", "/api/v1/iam/users", nil, 200)
	admin.request("POST", "/api/v1/iam/me/password", map[string]string{"old_password": initial, "new_password": "another changed password"}, 403)
	admin.request("GET", "/api/v1/iam/me", nil, 200)
	admin.request("POST", "/api/v1/iam/auth/reauthenticate", map[string]string{"password": "incorrect password"}, 400)
	admin.request("POST", "/api/v1/iam/me/password", map[string]string{"old_password": "incorrect password", "new_password": "another changed password"}, 403)
	me := admin.request("GET", "/api/v1/iam/me", nil, 200)
	org := me["organizations"].([]any)[0].(map[string]any)["id"].(string)
	userID := me["principal"].(map[string]any)["user_id"].(string)
	orgPath := "/api/v1/iam/orgs/" + org
	admin.request("PATCH", "/api/v1/iam/users/"+userID, map[string]any{"display_name": "Changed", "status": "disabled", "auth_version": me["principal"].(map[string]any)["auth_version"]}, 403)
	admin.request("POST", "/api/v1/iam/users/"+userID+"/reset-password", map[string]any{"password": "not configured password"}, 403)
	members := admin.request("GET", orgPath+"/members", nil, 200)["items"].([]any)
	var configuredMembership string
	for _, item := range members {
		row := item.(map[string]any)
		if row["user_id"] == userID {
			configuredMembership = row["id"].(string)
		}
	}
	if configuredMembership == "" {
		t.Fatal("configured administrator membership missing")
	}
	configuredVersion := admin.request("GET", orgPath, nil, 200)["authz_version"].(string)
	protectedPath := orgPath + "/members/" + configuredMembership
	admin.request("PATCH", protectedPath, map[string]any{"status": "disabled", "authz_version": configuredVersion}, 403)
	admin.request("DELETE", protectedPath+"?authz_version="+configuredVersion, nil, 403)
	admin.request("PUT", protectedPath+"/roles", map[string]any{"role_ids": []string{}, "authz_version": configuredVersion}, 403)
	admin.request("POST", orgPath+"/permission-preview", map[string]any{"membership_id": configuredMembership, "role_ids": []string{}, "authz_version": configuredVersion}, 403)

	catalog := admin.request("GET", "/api/v1/iam/permissions?org_id="+org, nil, 200)["items"].([]any)
	foundAudit := false
	for _, item := range catalog {
		entry := item.(map[string]any)
		permission := entry["permission"].(map[string]any)
		if permission["key"] == "iam.audit.read" {
			foundAudit = entry["can_delegate"] == true
			break
		}
	}

	if !foundAudit {
		t.Fatal("超级管理员应能在权限目录中看到并分配组织审计权限")
	}

	admin.request("PUT", "/api/v1/iam/users/"+userID+"/super-admin", map[string]any{"enabled": false, "reason": "identity is configuration-owned"}, 404)
	initialMember := "12345678"
	memberPassword := "isolated member password"
	memberID := admin.request("POST", orgPath+"/members", map[string]string{"username": f.member, "display_name": "审计成员", "password": initialMember}, 201)["id"].(string)
	roleID := admin.request("POST", orgPath+"/roles", map[string]string{"code": "security_auditor", "name": "审计员"}, 201)["id"].(string)
	admin.request("PUT", orgPath+"/roles/"+roleID+"/permissions", map[string]any{"permissions": []string{"iam.audit.read"}, "version": "1"}, 204)
	admin.request("PUT", orgPath+"/roles/"+roleID+"/permissions", map[string]any{"permissions": []string{}, "version": "1"}, 409)
	version := admin.request("GET", orgPath, nil, 200)["authz_version"].(string)
	admin.request("PUT", orgPath+"/members/"+memberID+"/roles", map[string]any{"role_ids": []string{roleID}, "authz_version": version}, 204)
	member := newAPIClient(t, public.URL, public.URL)
	member.login(f.member, initialMember)
	member.request("GET", orgPath+"/audit-events", nil, 200)
	member.change(initialMember, memberPassword)
	member.login(f.member, memberPassword)
	member.request("GET", orgPath+"/audit-events", nil, 200)
	memberUserID := member.request("GET", "/api/v1/iam/me", nil, 200)["principal"].(map[string]any)["user_id"].(string)
	admin.request("POST", "/api/v1/iam/users/"+memberUserID+"/reset-password", map[string]string{"password": "12345678"}, 204)
	member.request("GET", orgPath+"/audit-events", nil, 401)
	member.login(f.member, "12345678")
	member.request("GET", orgPath+"/audit-events", nil, 200)
	member.request("GET", orgPath+"/members", nil, 403)
	member.request("GET", "/api/v1/iam/users", nil, 403)
	otherOrg := admin.request("POST", "/api/v1/iam/organizations", map[string]string{"slug": "isolated-team", "name": "Other organization", "admin_user_id": userID}, 201)["id"].(string)
	otherRole := admin.request("POST", "/api/v1/iam/orgs/"+otherOrg+"/roles", map[string]string{"code": "other_reader", "name": "Other reader"}, 201)["id"].(string)
	member.request("GET", "/api/v1/iam/orgs/"+otherOrg+"/audit-events", nil, 403)
	currentVersion := admin.request("GET", orgPath, nil, 200)["authz_version"].(string)
	admin.request("PUT", orgPath+"/members/"+memberID+"/roles", map[string]any{"role_ids": []string{otherRole}, "authz_version": currentVersion}, 404)
	member.request("GET", orgPath+"/audit-events", nil, 200)
	admin.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": password, "return_to": "https://untrusted.invalid"}, 400)
	// 角色显示名称可以改变，稳定的能力语义不随名称改变。
	admin.request("PATCH", orgPath+"/roles/"+roleID, map[string]any{"name": "只读审计", "description": "", "status": "active", "version": "2"}, 204)
	member.request("GET", orgPath+"/audit-events", nil, 200)
	admin.request("DELETE", orgPath+"/roles/"+roleID+"?version=3", nil, 409)
	admin.request("PUT", orgPath+"/roles/"+roleID+"/permissions", map[string]any{"permissions": []string{}, "version": "3"}, 204)
	member.request("GET", orgPath+"/audit-events", nil, 403)
	// 服务凭证不能代替浏览器原始身份。
	call := func(cookie string, method string, want int, resources ...any) {
		t.Helper()
		input := map[string]any{
			"cookie":     cookie,
			"method":     method,
			"path":       "/api/v1/console/orgs/" + org + "/overview",
			"action":     "iam.organizations.read",
			"org_id":     org,
			"origin":     public.URL,
			"csrf_token": admin.csrf,
		}
		if len(resources) > 0 {
			input["resource"] = resources[0]
		}

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
	request, _ := http.NewRequest("GET", public.URL, nil)
	cookies := admin.client.Jar.Cookies(request.URL)
	cookieHeader := ""
	for _, cookie := range cookies {
		if cookieHeader != "" {
			cookieHeader += "; "
		}

		cookieHeader += cookie.Name + "=" + cookie.Value
	}

	// 同一入口中，仅用户 Cookie 或错误服务凭证不能请求鉴权。
	for _, serviceToken := range []string{"", strings.Repeat("x", 64)} {
		body, _ := json.Marshal(map[string]string{"cookie": cookieHeader, "method": "GET", "path": "/api/v1/console/context", "action": "iam.organizations.read", "org_id": org})
		req, _ := http.NewRequest("POST", public.URL+"/api/v1/iam/authorize", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookieHeader)
		if serviceToken != "" {
			req.Header.Set("Authorization", "Bearer "+serviceToken)
		}
		response, err := public.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 403 || len(response.Cookies()) != 0 {
			t.Fatal("user Cookie or invalid service credential crossed authentication boundary")
		}
	}
	// 服务凭证也不能代替用户会话访问用户 API。
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
	old, err := public.Client().Get(public.URL + "/internal/v1/iam/authorize")
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Body.Close()
	if old.StatusCode != 404 {
		t.Fatal("legacy route still registered")
	}
	call(cookieHeader, "GET", 200)
	call(cookieHeader, "GET", 400, map[string]string{"type": "unsupported", "id": uuid.NewString()})
	call("", "GET", 401)
	// 可信服务转交的写请求仍须通过 Origin 和 CSRF 校验。
	saved := admin.csrf
	admin.csrf = "invalid"
	call(cookieHeader, "POST", 403)
	admin.csrf = saved
	var activeSessions int
	if err := f.server.Service.Store.Pool.QueryRow(context.Background(), "SELECT count(*) FROM iam.user_sessions WHERE user_id=$1 AND state='active'", userID).Scan(&activeSessions); err != nil {
		t.Fatal(err)
	}
	if activeSessions != 1 {
		t.Fatalf("expected one active root session, got %d", activeSessions)
	}
	admin.request("GET", "/api/v1/iam/me/sessions", nil, 404)
	admin.request("POST", "/api/v1/iam/auth/logout-all", nil, 404)
	admin.request("POST", "/api/v1/iam/auth/logout", nil, 204)
	call(cookieHeader, "GET", 401)
	admin.request("GET", "/api/v1/iam/me", nil, 401)
	t.Log(fmt.Sprintf("identity, custom role, version conflicts, last administrator and internal authorization verified for org %s", org))
}

func TestPatchPreservesOmittedFieldsAndClearsExplicitEmptyValues(t *testing.T) {
	f := newFixture(t, nil)
	admin := newAPIClient(t, f.public.URL, f.public.URL)
	admin.login(f.admin, f.initial)
	admin.login(f.admin, f.password)
	user := admin.request("POST", "/api/v1/iam/users", map[string]any{"username": "patch_member", "display_name": "Before", "email": "patch@example.com", "password": f.password}, 201)
	userPath := "/api/v1/iam/users/" + user["id"].(string)
	admin.request("PATCH", userPath, map[string]any{"display_name": "After", "status": "active", "auth_version": "1"}, 204)
	if got := admin.request("GET", userPath, nil, 200)["email"]; got != "patch@example.com" {
		t.Fatalf("omitted email changed stored value: %v", got)
	}
	admin.request("PATCH", userPath, map[string]any{"display_name": "After", "status": "active", "auth_version": "2", "email": ""}, 204)
	if got := admin.request("GET", userPath, nil, 200)["email"]; got != nil {
		t.Fatalf("explicit empty email was not cleared: %v", got)
	}
	admin.request("PATCH", userPath, map[string]any{"display_name": "After", "status": "active", "auth_version": "3", "email": nil}, 400)
	me := admin.request("GET", "/api/v1/iam/me", nil, 200)
	org := me["organizations"].([]any)[0].(map[string]any)["id"].(string)
	role := admin.request("POST", "/api/v1/iam/orgs/"+org+"/roles", map[string]any{"code": "patch_role", "name": "Before", "description": "Preserve me"}, 201)
	rolePath := "/api/v1/iam/orgs/" + org + "/roles/" + role["id"].(string)
	admin.request("PATCH", rolePath, map[string]any{"name": "After", "status": "active", "version": "1"}, 204)
	if got := admin.request("GET", rolePath, nil, 200)["description"]; got != "Preserve me" {
		t.Fatalf("omitted description changed stored value: %v", got)
	}
	admin.request("PATCH", rolePath, map[string]any{"name": "After", "status": "active", "version": "2", "description": ""}, 204)
	if got := admin.request("GET", rolePath, nil, 200)["description"]; got != "" {
		t.Fatalf("explicit empty description was not cleared: %v", got)
	}
	admin.request("PATCH", rolePath, map[string]any{"name": "After", "status": "active", "version": "3", "description": nil}, 400)
}

func TestPreviewAndWriteShareInputConstraints(t *testing.T) {
	f := newFixture(t, nil)
	admin := newAPIClient(t, f.public.URL, f.public.URL)
	admin.login(f.admin, f.initial)
	admin.login(f.admin, f.password)
	me := admin.request("GET", "/api/v1/iam/me", nil, 200)
	org := me["organizations"].([]any)[0].(map[string]any)["id"].(string)
	base := "/api/v1/iam/orgs/" + org
	admin.request("POST", base+"/roles", map[string]any{"name": "Missing code"}, 400)
	role := admin.request("POST", base+"/roles", map[string]any{"code": "review_role", "name": "Review"}, 201)["id"].(string)
	admin.request("PATCH", base+"/roles/"+role, map[string]any{"name": "Missing status"}, 400)
	permissions := []string{"iam.audit.read", "iam.audit.read"}
	admin.request("POST", base+"/permission-preview", map[string]any{"role_id": role, "permissions": permissions, "version": "1"}, 400)
	admin.request("PUT", base+"/roles/"+role+"/permissions", map[string]any{"permissions": permissions, "version": "1"}, 400)
	roles := make([]string, 51)
	for i := range roles {
		roles[i] = uuid.NewString()
	}
	member := uuid.NewString()
	admin.request("POST", base+"/permission-preview", map[string]any{"membership_id": member, "role_ids": roles, "authz_version": "1"}, 400)
	admin.request("PUT", base+"/members/"+member+"/roles", map[string]any{"role_ids": roles, "authz_version": "1"}, 400)

	for _, version := range []string{"0", "9223372036854775808", "invalid"} {
		admin.request("PATCH", base+"/roles/"+role, map[string]any{"name": "Review", "status": "active", "version": version}, 400)
	}
	admin.request("GET", base+"/roles?limit=0", nil, 400)
	admin.request("GET", base+"/roles?offset=-1", nil, 400)
	admin.request("GET", base+"/roles?limit=9223372036854775808", nil, 400)
	admin.request("GET", base+"/roles/invalid", nil, 400)
	if got := admin.request("GET", base+"/roles/"+role, nil, 200)["version"]; got != "1" {
		t.Fatal("rejected input changed the role")
	}
}
