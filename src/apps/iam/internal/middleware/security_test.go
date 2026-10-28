package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/csrf"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/contracts/common"
)

// TestOriginAndCSRFBoundary 用真实 CSRF Cookie 和令牌验证来源边界，不跳过令牌校验。
func TestOriginAndCSRFBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		public string
		origin string
		status int
	}{
		{"configured", "http://127.0.0.1:5173", "http://127.0.0.1:5173", 204},
		{"localhost alias", "http://127.0.0.1:5173", "http://localhost:5173", 204},
		{"IPv4 alias", "http://localhost:5173", "http://127.0.0.1:5173", 204},
		{"IPv6 alias", "http://localhost:5173", "http://[::1]:5173", 204},
		{"different port", "http://127.0.0.1:5173", "http://localhost:5174", 403},
		{"different scheme", "http://127.0.0.1:5173", "https://localhost:5173", 403},
		{"external site", "http://127.0.0.1:5173", "http://example.com:5173", 403},
		{"hostname suffix", "http://127.0.0.1:5173", "http://localhost.example.com:5173", 403},
		{"absent origin", "http://127.0.0.1:5173", "", 403},
		{"public domain", "https://console.example.com", "https://console.example.com", 204},
		{"public domain has no local alias", "https://console.example.com", "https://localhost", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{}
			cfg.App.PublicURL = tc.public
			cfg.Auth.CSRFKey = strings.Repeat("k", 32)
			cfg.Internal.Clients = []config.InternalClient{{ID: "console", Token: strings.Repeat("t", 32), Actions: []string{"iam.organizations.read"}}}
			security, err := NewSecurity(cfg)
			if err != nil {
				t.Fatal(err)
			}

			handler := security.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-CSRF-Token", csrf.Token(r))
				w.WriteHeader(http.StatusNoContent)
			}))
			get := httptest.NewRecorder()
			handler.ServeHTTP(get, httptest.NewRequest("GET", tc.public+"/auth/context", nil))
			cookies := get.Result().Cookies()
			if get.Code != http.StatusNoContent || len(cookies) == 0 {
				t.Fatal("没有获得真实 CSRF Cookie")
			}

			post := httptest.NewRequest("POST", tc.public+"/auth/login", nil)
			post.Header.Set("Origin", tc.origin)
			post.Header.Set("X-CSRF-Token", get.Header().Get("X-CSRF-Token"))
			for _, cookie := range cookies {
				post.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, post)
			if response.Code != tc.status {
				t.Fatalf("预期 %d，实际 %d", tc.status, response.Code)
			}
			if tc.status == http.StatusForbidden && !strings.Contains(response.Body.String(), string(common.CodeOriginInvalid)) {
				t.Fatal("不允许的来源应返回统一的来源错误码")
			}

			// 允许来源也必须提供与 Cookie 对应的 CSRF 令牌。
			post.Header.Del("X-CSRF-Token")
			withoutToken := httptest.NewRecorder()
			handler.ServeHTTP(withoutToken, post)
			if withoutToken.Code != http.StatusForbidden {
				t.Fatal("来源兼容不能绕过 CSRF 令牌校验")
			}
			if tc.status == http.StatusNoContent && !strings.Contains(withoutToken.Body.String(), string(common.CodeCSRFInvalid)) {
				t.Fatal("允许来源缺少令牌时应返回统一的 CSRF 错误码")
			}
		})
	}
}
