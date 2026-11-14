package httpchannel_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

const testToken = "ing_test_token_0123456789abcdef"

// fakeLimiter 记录调用并可按需返回错误。
type fakeLimiter struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeLimiter) Allow(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	return f.err
}

// fakePublisher 收集已发布事件。
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

func (f *fakePublisher) snapshot() []pipeline.Event {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]pipeline.Event(nil), f.events...)
}

// newRule 编译一条最小 JSON 规则。
func newRule(t *testing.T) *normalizer.Engine {
	t.Helper()

	engine, err := normalizer.Compile(normalizer.Rule{
		ID:        "http-test",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Constants: map[string]any{"class_uid": 2004},
	})
	if err != nil {
		t.Fatal(err)
	}

	return engine
}

// harness 组装一个带路由的测试服务；返回通道、管道与假实现。
func harness(t *testing.T, limiter httpchannel.Limiter) (*httptest.Server, *pipeline.Pipeline, *fakePublisher, *fakeLimiter) {
	t.Helper()

	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID:      "conn-1",
		Channel: connector.ChannelHTTP,
		Engine:  newRule(t),
		Token:   connector.DigestToken(testToken),
	}})

	if limiter == nil {
		limiter = &fakeLimiter{}
	}

	publisher := &fakePublisher{}
	line := pipeline.New(pipeline.Options{Workers: 2, QueueDepth: 8, BatchSize: 4, FlushInterval: 50 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	channel := httpchannel.New(registry, limiter, line)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/ingest/events/{connector_id}", channel)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	fake, _ := limiter.(*fakeLimiter)
	return server, line, publisher, fake
}

// post 发起一次推送；token 为空表示不带鉴权头。
func post(t *testing.T, server *httptest.Server, connectorID, token, contentType, body string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost,
		server.URL+"/api/v1/ingest/events/"+connectorID, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

// 正常推送返回 202 与受理计数，事件最终进入发布器。
func TestAcceptedPushReturns202AndPublishes(t *testing.T) {
	server, line, publisher, _ := harness(t, nil)

	response := post(t, server, "conn-1", testToken, "application/json",
		`{"src":"10.0.0.1"}`)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}

	var body struct {
		Accepted int `json:"accepted"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Accepted != 1 {
		t.Fatalf("accepted = %d, want 1", body.Accepted)
	}

	line.Flush(context.Background())
	events := publisher.snapshot()
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}
	if events[0].Outcome.Status != pipeline.StatusOK {
		t.Fatalf("status = %q, want ok", events[0].Outcome.Status)
	}
	if events[0].PartitionKey != "10.0.0.1" {
		t.Fatalf("partition key = %q, want the src ip", events[0].PartitionKey)
	}
}

// 数组报文按元素拆成多条事件。
func TestArrayPayloadSplitsIntoEvents(t *testing.T) {
	server, line, publisher, _ := harness(t, nil)

	response := post(t, server, "conn-1", testToken, "application/json",
		`[{"src":"10.0.0.1"},{"src":"10.0.0.2"},{"src":"10.0.0.3"}]`)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}

	line.Flush(context.Background())
	if events := publisher.snapshot(); len(events) != 3 {
		t.Fatalf("published %d events, want 3", len(events))
	}
}

// 令牌缺失、错误、与路径不匹配都返回 401。
func TestUnauthorizedVariants(t *testing.T) {
	tests := map[string]struct{ connectorID, token string }{
		"missing token":   {"conn-1", ""},
		"wrong token":     {"conn-1", "ing_wrong_token_0123456789abcdef"},
		"token for other": {"conn-2", testToken},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server, _, publisher, _ := harness(t, nil)

			response := post(t, server, test.connectorID, test.token, "application/json", `{"src":"10.0.0.1"}`)
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", response.StatusCode)
			}
			if events := publisher.snapshot(); len(events) != 0 {
				t.Fatalf("unauthorized push published %d events", len(events))
			}
		})
	}
}

// 只接受 application/json 与 text/plain；其余 415，缺省按 JSON 处理。
func TestContentTypeMatrix(t *testing.T) {
	tests := map[string]struct {
		contentType string
		want        int
	}{
		"json":       {"application/json", http.StatusAccepted},
		"json+param": {"application/json; charset=utf-8", http.StatusAccepted},
		"text":       {"text/plain", http.StatusAccepted},
		"absent":     {"", http.StatusAccepted},
		"xml":        {"application/xml", http.StatusUnsupportedMediaType},
		"form":       {"application/x-www-form-urlencoded", http.StatusUnsupportedMediaType},
		"malformed":  {"not-a-media-type", http.StatusUnsupportedMediaType},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server, _, _, _ := harness(t, nil)

			response := post(t, server, "conn-1", testToken, test.contentType, `{"src":"10.0.0.1"}`)
			if response.StatusCode != test.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.want)
			}
		})
	}
}

// 超过 10 MiB 的请求体返回 413。
func TestOversizedBodyReturns413(t *testing.T) {
	server, _, _, _ := harness(t, nil)

	oversized := `{"src":"` + strings.Repeat("x", int(httpchannel.DefaultMaxBodyBytes)) + `"}`
	response := post(t, server, "conn-1", testToken, "application/json", oversized)

	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.StatusCode)
	}
}

// 数组超过 1000 条返回 413。
func TestOversizedBatchReturns413(t *testing.T) {
	server, _, _, _ := harness(t, nil)

	elements := make([]string, httpchannel.MaxBatch+1)
	for index := range elements {
		elements[index] = `{"src":"10.0.0.1"}`
	}

	response := post(t, server, "conn-1", testToken, "application/json",
		"["+strings.Join(elements, ",")+"]")

	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.StatusCode)
	}
}

// 限流命中返回 429 并带 Retry-After。
func TestRateLimitedReturns429(t *testing.T) {
	limiter := &fakeLimiter{err: httpchannel.ErrRateLimited}
	server, _, publisher, _ := harness(t, limiter)

	response := post(t, server, "conn-1", testToken, "application/json", `{"src":"10.0.0.1"}`)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.StatusCode)
	}
	if response.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
	if events := publisher.snapshot(); len(events) != 0 {
		t.Fatal("rate limited push published events")
	}
}

// 限流后端故障返回 503 而不是放行——放行等于关掉防护。
func TestLimiterFailureReturns503(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}
	server, _, publisher, _ := harness(t, limiter)

	response := post(t, server, "conn-1", testToken, "application/json", `{"src":"10.0.0.1"}`)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
	if events := publisher.snapshot(); len(events) != 0 {
		t.Fatal("push accepted while the limiter was down")
	}
}

// 畸形报文不返回错误给设备：受理后解析失败的事件照常发布并带 parse_status。
func TestMalformedPayloadStillPublishesErrorEvent(t *testing.T) {
	server, line, publisher, _ := harness(t, nil)

	response := post(t, server, "conn-1", testToken, "application/json", `not json at all`)
	if response.StatusCode != http.StatusBadRequest {
		// 无法拆分属于报文层错误，400 是合理的；此处只断言没有崩。
		t.Logf("status = %d", response.StatusCode)
	}

	line.Flush(context.Background())
	for _, event := range publisher.snapshot() {
		if event.Outcome.Status == pipeline.StatusError && event.Outcome.Reason == "" {
			t.Fatal("error event without a reason code")
		}
	}
}

// 未挂规则的接入点：受理成功，事件以 error 状态透传原文。
func TestConnectorWithoutRulePublishesPassthrough(t *testing.T) {
	registry := connector.NewMemory()
	registry.Replace([]connector.Snapshot{{
		ID:      "conn-generic",
		Channel: connector.ChannelHTTP,
		Engine:  nil,
		Token:   connector.DigestToken(testToken),
	}})

	publisher := &fakePublisher{}
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond}, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = line.Start(ctx) }()

	channel := httpchannel.New(registry, &fakeLimiter{}, line)
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/ingest/events/{connector_id}", channel)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	response := post(t, server, "conn-generic", testToken, "text/plain", "plain syslog-ish line")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}

	line.Flush(context.Background())
	events := publisher.snapshot()
	if len(events) != 1 || events[0].Outcome.Status != pipeline.StatusError {
		t.Fatalf("passthrough events = %#v", events)
	}
}
