package router_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
)

// 只有已声明的路由应存在；未知路径必须是 404 而不是被中间件吞成其它状态。
func TestUnknownRouteIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/nope", nil)
	router.Handler(&handler.Server{}, router.Deps{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// 回归：就绪检查在依赖探活失败时必须返回 503，
// 恒真就绪会把流量导给依赖全挂的实例。
// testToken 是本文件推送用例共用的设备令牌。
const testToken = "ing_router_test_token_0123456789"

// fakePublisher 收集发布出去的事件。
type fakePublisher struct {
	mu     sync.Mutex
	events []pipeline.Event
}

func (f *fakePublisher) Publish(_ context.Context, events []pipeline.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, events...)
	return nil
}

// allowAllLimiter 永远放行，用于验证中间件边界。
type allowAllLimiter struct{}

func (allowAllLimiter) Allow(context.Context, string) error { return nil }

func TestReadyReportsUnavailableWhenCheckFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &handler.Server{
		Ready: func(context.Context) error { return errors.New("kafka unavailable") },
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.Handler(server, router.Deps{}).ServeHTTP(recorder, request)

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
	router.Handler(server, router.Deps{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

// 没有依赖检查的阶段同样就绪（进程已启动、配置已加载）。
func TestReadyWithoutCheckIsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.Handler(&handler.Server{}, router.Deps{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

// 回归（P0）：注册表加载完成（哪怕没有接入点）后就绪必须为真。
// 之前从未调用 Replace，Loaded() 恒为假，进程永远 503。
func TestReadyAfterRegistryLoaded(t *testing.T) {
	registry := input.NewMemory()

	handlerInstance := &handler.Server{Ready: func(context.Context) error {
		if !registry.Loaded() {
			return errors.New("source registry is not loaded")
		}

		return nil
	}}

	before := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	router.Handler(handlerInstance, router.Deps{}).ServeHTTP(before, request)

	if before.Code != http.StatusServiceUnavailable {
		t.Fatalf("加载前状态 = %d，期望 503", before.Code)
	}

	// 空集合也是"已加载"：没有接入点不等于没准备好。
	registry.Replace(nil)

	after := httptest.NewRecorder()
	router.Handler(handlerInstance, router.Deps{}).ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if after.Code != http.StatusOK {
		t.Fatalf("加载后状态 = %d，期望 200", after.Code)
	}
}

// 回归（P0）：设备推送路由曾被管理端中间件压成 32 KiB + 仅 JSON。
// 这条测试刻意走完整的 router.Handler（含中间件），
// 因为之前所有推送用例都直接调通道的 Handle，绕过了中间件，缺陷因此没被发现。
func TestDevicePushBypassesAdminInputBoundary(t *testing.T) {
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID:       "conn-1",
		Protocol: input.ProtocolHTTP,
		Token:    input.DigestToken(testToken),
	}})

	publisher := &fakePublisher{}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 10, FlushInterval: 20 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	server := httptest.NewServer(router.RootHandler(&handler.Server{}, router.Deps{
		Registry: registry,
		Limiter:  allowAllLimiter{},
		Pipeline: line,
		MaxBody:  8 << 20,
	}))
	t.Cleanup(server.Close)

	// 100 KiB 的合法 JSON：超过管理端 32 KiB 默认上限，但仍远低于设备通道的 8 MiB。
	payload := `{"msg":"` + strings.Repeat("a", 100*1024) + `"}`

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/events/conn-1", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("100 KiB 推送状态 = %d，期望 202（被中间件压成了 32 KiB）", response.StatusCode)
	}
}
