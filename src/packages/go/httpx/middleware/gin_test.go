package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinBoundaryPreservesErrorsAndMethodSemantics(t *testing.T) {
	router := NewEngine()
	router.GET("/items/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/panic", func(c *gin.Context) {
		panic("private-password-value")
	})

	for _, test := range []struct {
		name, method, path, code string
		status                   int
	}{
		{"missing route", "GET", "/missing", "not_found", 404},
		{"method mismatch", "POST", "/items/123", "method_not_allowed", 405},
		{"private panic", "GET", "/panic", "internal_error", 500},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("X-Request-ID", "untrusted-request-id")
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d; want %d", response.Code, test.status)
			}
			var body struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code || body.RequestID == "" || body.RequestID == "untrusted-request-id" {
				t.Fatalf("unexpected result: %+v", body)
			}
			if body.RequestID != response.Header().Get("X-Request-ID") {
				t.Fatal("response and body request IDs differ")
			}
			if test.status == 405 && response.Header().Get("Allow") != "GET" {
				t.Fatal("405 response omitted allowed method")
			}
		})
	}
}

// TestGinClientIPTrustBoundary 验证实际框架的代理解析，而非另一个手写算法。
func TestGinClientIPTrustBoundary(t *testing.T) {
	engine := NewEngine()
	engine.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := engine.SetTrustedProxies([]string{"127.0.0.0/8", "10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/address", func(c *gin.Context) { c.String(200, c.ClientIP()) })
	for _, tc := range []struct{ remote, forwarded, want string }{
		{"192.0.2.10:1234", "198.51.100.2", "192.0.2.10"},
		{"127.0.0.1:1234", "198.51.100.2,10.0.0.1", "198.51.100.2"},
		{"127.0.0.1:1234", "spoofed", "127.0.0.1"},
		{"[::1]:1234", "198.51.100.2", "::1"},
	} {
		request := httptest.NewRequest("GET", "/address", nil)
		request.RemoteAddr = tc.remote
		request.Header.Set("X-Forwarded-For", tc.forwarded)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Body.String() != tc.want {
			t.Fatalf("want %s, got %s", tc.want, response.Body.String())
		}
	}
}

func TestCommittedResponseIsNotWrappedOnPanic(t *testing.T) {
	engine := NewEngine()
	engine.GET("/stream", func(c *gin.Context) {
		c.Writer.WriteHeader(200)
		_, _ = c.Writer.WriteString("event: started\n\n")
		c.Writer.Flush()
		panic("private-stream-failure")
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/stream", nil))
	if response.Code != 200 || response.Body.String() != "event: started\n\n" || !response.Flushed {
		t.Fatal("recovery appended JSON or lost native flush after commitment")
	}
	for _, method := range []string{"HEAD", "OPTIONS"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(method, "/stream", nil))
		if response.Code != 405 || response.Header().Get("Allow") != "GET" {
			t.Fatalf("%s did not preserve explicit method policy", method)
		}
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/stream/", nil))
	if response.Code != 404 || response.Header().Get("Location") != "" {
		t.Fatal("trailing slash redirected outside the published contract")
	}
}
