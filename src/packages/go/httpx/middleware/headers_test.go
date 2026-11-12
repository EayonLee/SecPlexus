package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersAppliedToEveryResponse(t *testing.T) {
	router := NewEngine()
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/panic", func(c *gin.Context) { panic("boom") })

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Content-Security-Policy":      "frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Strict-Transport-Security":    "max-age=31536000",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"X-XSS-Protection":             "0",
	}

	// 成功、未找到、panic 三类响应都必须带同一组头：错误响应同样会被浏览器渲染。
	for _, test := range []struct {
		name, path string
		status     int
	}{
		{"success", "/ok", http.StatusNoContent},
		{"not found", "/missing", http.StatusNotFound},
		{"panic", "/panic", http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status = %d; want %d", response.Code, test.status)
			}
			for name, value := range want {
				if got := response.Header().Get(name); got != value {
					t.Errorf("%s = %q; want %q", name, got, value)
				}
			}
			if got := response.Header().Get("Permissions-Policy"); got == "" {
				t.Error("Permissions-Policy must not be empty")
			}
		})
	}
}

func TestSecurityHeadersKeepExistingValues(t *testing.T) {
	router := NewEngine()
	router.GET("/custom", func(c *gin.Context) {
		c.Header("Referrer-Policy", "no-referrer")
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/custom", nil))
	if got := response.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q; want the value set by the handler", got)
	}
}

// ClientIP 的来源决定登录限流的分桶键，因此这里把两条契约钉死：
// 未配置可信代理时忽略转发头；配置了可信代理才采信它。
func TestClientIPIgnoresForwardedHeaderByDefault(t *testing.T) {
	router := NewEngine()
	router.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "10.0.0.9:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if got := response.Body.String(); got != "10.0.0.9" {
		t.Fatalf("ClientIP = %q; want the direct peer 10.0.0.9", got)
	}
}

func TestClientIPHonoursForwardedHeaderFromTrustedProxy(t *testing.T) {
	router := NewEngine()
	router.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := router.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	router.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "10.0.0.9:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if got := response.Body.String(); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q; want the forwarded client 203.0.113.7", got)
	}
}
