package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	client, err := iamclient.New("http://127.0.0.1:8081", strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(client.Close)
	return router.Handler(&handler.Server{IAM: client})
}

// 只有已声明的路由应存在；未知路径必须是 404 而不是被中间件吞成其它状态。
func TestUnknownRouteIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/nope", nil)
	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// 健康检查必须存在；管理路由在后续批次挂载。
func TestReadyRouteRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	client, err := iamclient.New("http://127.0.0.1:8081", strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(client.Close)
	router.Register(engine, &handler.Server{IAM: client})

	for _, route := range engine.Routes() {
		if route.Path == "/health/ready" {
			return
		}
	}

	t.Fatal("readiness route is not registered")
}
