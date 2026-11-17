package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
)

// 回归：指标端点必须暴露流水线计数器，且无需会话（内网抓取）。
func TestMetricsExposesPipelineCounters(t *testing.T) {
	server := httptest.NewServer(router.Handler(&handler.Server{
		Stats: func() pipeline.Stats {
			return pipeline.Stats{
				Accepted: 10, Parsed: 8, ParseFailed: 2, Published: 8, QueueDepth: 3,
				ByInput: map[string]pipeline.ProtocolStats{
					"input-a": {Accepted: 2, Parsed: 2},
				},
				ByProtocol: map[string]pipeline.ProtocolStats{
					"http":   {Accepted: 6, Parsed: 5},
					"syslog": {Accepted: 4, Parsed: 3},
				},
			}
		},
	}, router.Deps{}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("状态 = %d，期望 200", response.StatusCode)
	}

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)

	for _, expected := range []string{
		"spx_ingest_accepted_total 10",
		"spx_ingest_parse_failed_total 2",
		"spx_ingest_published_total 8",
		"spx_ingest_queue_depth 3",
		"# TYPE spx_ingest_accepted_total counter",
		`spx_ingest_protocol_accepted_total{protocol="http"} 6`,
		`spx_ingest_protocol_parsed_total{protocol="syslog"} 3`,
		`spx_ingest_input_accepted_total{input_id="input-a"} 2`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("指标缺少 %q\n实际:\n%s", expected, body)
		}
	}
}

// 没有注入 Stats 时不暴露端点，避免输出空指标误导抓取。
func TestMetricsAbsentWithoutStats(t *testing.T) {
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("状态 = %d，期望 404", response.StatusCode)
	}
}
