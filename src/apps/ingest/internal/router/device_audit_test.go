package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// 回归：设备数据接入不写审计。
//
// 审计只记管理写操作与授权拒绝。设备推送是高频数据面请求，
// 混进审计会把真正的管理留痕淹没，也会把审计表写成事件表。
func TestDevicePushDoesNotWriteAudit(t *testing.T) {
	var auditCalls atomic.Int64

	iam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "audit") {
			auditCalls.Add(1)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(iam.Close)

	client, err := iamclient.New(iam.URL, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID:       "dev-1",
		Protocol: input.ProtocolHTTP,
		Token:    input.DigestToken(testToken),
	}})

	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 10, FlushInterval: 20 * time.Millisecond}, &fakePublisher{})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	server := httptest.NewServer(router.RootHandler(&handler.Server{}, router.Deps{
		Registry: registry,
		Limiter:  allowAllLimiter{},
		Pipeline: line,
		MaxBody:  8 << 20,
		IAM:      client,
		Admin:    &fakeStore{},
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/events/dev-1",
		strings.NewReader(`{"src":"10.0.0.1"}`))
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
		t.Fatalf("状态 = %d，期望 202", response.StatusCode)
	}
	if auditCalls.Load() != 0 {
		t.Fatalf("设备推送写入了 %d 条审计", auditCalls.Load())
	}
}
