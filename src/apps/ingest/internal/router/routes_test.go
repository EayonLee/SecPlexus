package router_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
)

// 只有已声明的路由应存在；未知路径必须是 404 而不是被中间件吞成其它状态。
func TestUnknownRouteIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/nope", nil)
	router.Handler(&handler.Server{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// 回归：就绪检查在依赖探活失败时必须返回 503，
// 恒真就绪会把流量导给依赖全挂的实例。
func TestReadyReportsUnavailableWhenCheckFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &handler.Server{
		Ready: func(context.Context) error { return errors.New("kafka unavailable") },
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.Handler(server).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

// 依赖可用时返回 200。
func TestReadySucceedsWhenCheckPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &handler.Server{
		Ready: func(context.Context) error { return nil },
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.Handler(server).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

// 没有依赖检查的阶段同样就绪（进程已启动、配置已加载）。
func TestReadyWithoutCheckIsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.Handler(&handler.Server{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}
