package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
)

// 停用接入点后，设备用原 token 推送必须立刻被拒绝。
//
// 注册表是运行期的唯一事实来源：停用只改数据库，如果注册表刷新晚一步
// 或者旧条目仍在内存里，被停用的通道会继续收数据——停用就没意义了。
func TestDisabledInputRejectsDevicePush(t *testing.T) {
	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID:       "dev-1",
		Protocol: input.ProtocolHTTP,
		Token:    input.DigestToken(testToken),
	}})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, &fakePublisher{})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{
		Push: devicePush(registry, line),
	}))
	t.Cleanup(server.Close)

	if status := push(t, server.URL); status != http.StatusAccepted {
		t.Fatalf("停用前状态 = %d，期望 202", status)
	}

	registry.Replace(nil)

	if status := push(t, server.URL); status != http.StatusUnauthorized {
		t.Fatalf("停用后状态 = %d，期望 401", status)
	}
}

func push(t *testing.T, baseURL string) int {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/ingest/events/dev-1",
		strings.NewReader(`{"src":"10.0.0.1"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}

	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = response.Body.Close() }()

	return response.StatusCode
}
