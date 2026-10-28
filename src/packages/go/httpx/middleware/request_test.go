package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

func TestBoundaryKeepsRequestIDAndRecoversBeforeCommit(t *testing.T) {
	for _, panics := range []bool{false, true} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", "untrusted")
		router := NewEngine()
		router.GET("/", func(c *gin.Context) {
			w, r := c.Writer, c.Request
			if RequestID(r) == "untrusted" || RequestID(r) == "" {
				t.Fatal("request ID was not generated at the boundary")
			}

			if panics {
				panic("internal detail")
			}

			result.Success(w, []string{})
		})
		router.ServeHTTP(w, r)
		var value common.Result[any]
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}

		if value.RequestID != w.Header().Get("X-Request-ID") {
			t.Fatal("body and header request IDs differ")
		}

		if panics && w.Code != 500 {
			t.Fatalf("panic returned %d", w.Code)
		}
	}
}

// TestBoundaryLogsSafeRoute 验证完整请求链可关联到路由，同时不记录原始 URL、凭据或 panic 值。
func TestBoundaryLogsSafeRoute(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	router := NewEngine()
	router.GET("/items/:id", func(c *gin.Context) {
		r := c.Request
		logging.From(r.Context()).Info("核心步骤", "event", "test.step")
		panic("secret-panic")
	})
	request := httptest.NewRequest("GET", "/items/secret-id?token=secret-query", nil)
	request.Header.Set("Authorization", "secret-header")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 500 {
		t.Fatal("panic did not return uniform failure")
	}
	if strings.Contains(output.String(), "secret-") {
		t.Fatal("request or panic value leaked")
	}
	if !strings.Contains(output.String(), `"route":"/items/:id"`) {
		t.Fatal("matched route missing")
	}
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record["request_id"] != response.Header().Get("X-Request-ID") {
			t.Fatal("core event lost request correlation")
		}
	}
}

// TestRejectedBeforeRouterStillLogsPattern 保证安全中间件提前拒绝时仍能定位接口。
func TestRejectedBeforeRouterStillLogsPattern(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	router := NewEngine()
	router.Use(func(c *gin.Context) { result.Fail(c.Writer, common.CodeOriginInvalid); c.Abort() })
	router.GET("/items/:id", func(*gin.Context) { t.Fatal("rejected request reached handler") })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/items/secret-id?token=secret-query", nil))
	if strings.Contains(output.String(), "secret-") || !strings.Contains(output.String(), `"route":"/items/:id"`) {
		t.Fatal("early rejection lost safe route correlation")
	}
}
