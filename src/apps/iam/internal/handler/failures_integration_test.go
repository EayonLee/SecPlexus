//go:build integration

package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
)

func TestSessionPersistenceFailureCannotSendSuccessfulLogin(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	_, err := f.server.Service.Store.Pool.Exec(ctx, `CREATE FUNCTION iam.reject_session_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'unavailable'; END $$; CREATE TRIGGER reject_session_write BEFORE INSERT OR UPDATE ON iam.scs_sessions FOR EACH ROW EXECUTE FUNCTION iam.reject_session_write()`)
	if err != nil {
		t.Fatal(err)
	}

	client := newAPIClient(t, f.public.URL, f.public.URL)
	client.context()
	client.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": f.initial}, 503)
	request, _ := http.NewRequest("GET", f.public.URL, nil)
	for _, cookie := range client.client.Jar.Cookies(request.URL) {
		if cookie.Name == "spx_session" {
			t.Fatal("failed storage produced an authentication cookie")
		}
	}

	var active int
	if err = f.server.Service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.user_sessions WHERE state='active'`).Scan(&active); err != nil {
		t.Fatal(err)
	}

	if active != 0 {
		t.Fatal("orphan login remained active")
	}
}

func TestSecurityChangesRollbackWhenAuditCannotCommit(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	client := newAPIClient(t, f.public.URL, f.public.URL)
	client.login(f.admin, f.initial)
	client.login(f.admin, f.password)
	_, err := f.server.Service.Store.Pool.Exec(ctx, `CREATE FUNCTION iam.reject_audit_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'unavailable'; END $$; CREATE TRIGGER reject_audit_write BEFORE INSERT ON iam.audit_events FOR EACH ROW EXECUTE FUNCTION iam.reject_audit_write()`)
	if err != nil {
		t.Fatal(err)
	}

	client.request("POST", "/api/v1/iam/users", map[string]string{"username": "newmember", "display_name": "成员", "password": "temporary account password"}, 500)
	var count int
	if err = f.server.Service.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM iam.users WHERE username_normalized='newmember'`).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatal("account survived a failed audit transaction")
	}
}

func TestAuthenticationLocksRecoverAndDependencyFailuresRemainUnavailable(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) { cfg.Auth.LoginMaxFailures = 2 })
	ctx := context.Background()
	client := newAPIClient(t, f.public.URL, f.public.URL)
	client.context()
	for range 2 {
		client.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": "incorrect password"}, 401)
	}

	client.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": f.initial}, 401)
	if _, err := f.server.Service.Store.Pool.Exec(ctx, `UPDATE iam.password_credentials SET locked_until=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}

	client.login(f.admin, f.initial)
	if err := f.redis.Close(); err != nil {
		t.Fatal(err)
	}

	client.request("POST", "/api/v1/iam/auth/login", map[string]string{"username": f.admin, "password": f.initial}, 503)
	// Redis 不可用不能被误报为密码错误，也不应阻断不依赖 Redis 的身份读取。
	client.request("GET", "/api/v1/iam/me", nil, 200)
	f.server.Service.Store.Pool.Close()
	client.request("GET", "/api/v1/iam/me", nil, 503)
}

// TestPanicBeforeSessionCommitCompensatesLogicalSession 保护逻辑落库后、Cookie提交前的异常路径。
func TestPanicBeforeSessionCommitCompensatesLogicalSession(t *testing.T) {
	f := newFixture(t, nil)
	client := newAPIClient(t, f.public.URL, f.public.URL)
	client.context()
	f.server.ReadyCheck = func(ctx context.Context) error {
		principal, err := f.server.Auth.Login(ctx, authentication.Credentials{Username: f.admin, Password: f.initial}, authentication.RequestMetadata{RequestID: "panic-compensation", ClientIP: "127.0.0.1"})
		if err != nil {
			return err
		}
		authentication.CurrentAttempt(ctx).Principal = &principal
		panic("private panic must not reach browser")
	}
	response, err := client.client.Get(f.public.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 500 {
		t.Fatalf("panic returned %d", response.StatusCode)
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == f.server.Service.Sessions.Manager.Cookie.Name {
			t.Fatal("uncommitted session cookie escaped")
		}
	}
	var active int
	if err := f.server.Service.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM iam.user_sessions WHERE state='active'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatal("panic left an active logical session")
	}
}
