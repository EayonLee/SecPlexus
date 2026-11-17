package router_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// discardPublisher 丢弃事件：压测测的是受理链路，不是 Kafka 往返。
type discardPublisher struct{}

func (discardPublisher) Publish(context.Context, []pipeline.Event) error { return nil }

// allowAll 放行全部请求：限流不在吞吐测量范围内。
type allowAll struct{}

func (allowAll) Allow(context.Context, string) error { return nil }

// benchHandler 构造一条完整的受理链路：HTTP -> 鉴权 -> 解析 -> 归一化 -> 入队。
func benchHandler(b *testing.B, body []byte) http.Handler {
	b.Helper()

	engine, err := normalizer.Compile(normalizer.Mapping{
		ID:        "bench",
		Name:      "bench",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip", "msg": "message"},
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		b.Fatal(err)
	}

	const token = "bench-token"

	registry := input.NewMemory()
	registry.Replace([]input.Entry{{
		ID: "bench", Protocol: input.ProtocolHTTP, Mapping: engine, Token: input.DigestToken(token),
	}})

	line := pipeline.New(pipeline.Options{Workers: 4, BatchSize: 256, FlushInterval: 50 * time.Millisecond}, discardPublisher{})

	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)
	b.Cleanup(func() { _ = line.Flush(context.Background()) })
	go func() { _ = line.Start(ctx) }()

	channel := httpchannel.New(registry, allowAll{}, line)

	// 与生产一致：设备接入走 Go 1.22 ServeMux 模式，通道按 PathValue 取 input_id。
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/ingest/events/{input_id}", channel)

	// 预热一次，避免把首条请求的惰性初始化算进测量。
	warm := httptest.NewRecorder()
	mux.ServeHTTP(warm, benchRequest(body, token))
	if warm.Code != http.StatusAccepted {
		b.Fatalf("预热失败: %d %s", warm.Code, warm.Body.String())
	}

	return mux
}

// benchRequest 构造一条带令牌的受理请求。
func benchRequest(body []byte, token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/events/bench", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	return request
}

// BenchmarkIngestHTTP 测量单条事件从进 HTTP 到入队的完整开销。
//
// 流水线是异步的：这里测的是"受理成功"的延迟，
// 发布吞吐由 BenchmarkApply* 与 Kafka 侧指标回答。
func BenchmarkIngestHTTP(b *testing.B) {
	body := []byte(`{"src":"10.0.0.1","msg":"failed login"}`)
	handler := benchHandler(b, body)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, benchRequest(body, "bench-token"))

		if recorder.Code != http.StatusAccepted {
			b.Fatalf("状态 = %d", recorder.Code)
		}
	}
}

// 批量推送：一次请求携带 100 条事件，测每事件的摊薄成本。
func BenchmarkIngestHTTPBatch100(b *testing.B) {
	lines := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf(`{"src":"10.0.0.%d","msg":"failed login"}`, i%250))
	}

	body := []byte(strings.Join(lines, "\n"))
	handler := benchHandler(b, body)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, benchRequest(body, "bench-token"))

		if recorder.Code != http.StatusAccepted {
			b.Fatalf("状态 = %d", recorder.Code)
		}
	}
}
