package handler_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

func TestMetricsGroupsAllSamplesOfEachCounter(t *testing.T) {
	server := handler.Server{Stats: func() pipeline.Stats {
		return pipeline.Stats{
			ByProtocol: map[string]pipeline.ProtocolStats{"http": {Accepted: 3}, "syslog": {Accepted: 7}},
			ByInput:    map[string]pipeline.InputStats{"first": {Parsed: 2}, "second": {Parsed: 4}},
		}
	}, AuditOutboxHealth: func(context.Context) (int64, int64, error) { return 7, 25, nil }}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/metrics", nil)
	server.Metrics(ctx)
	output := recorder.Body.String()
	for _, group := range []struct {
		name    string
		samples string
	}{
		{"spx_ingest_protocol_accepted_total", "spx_ingest_protocol_accepted_total{protocol=\"http\"} 3\nspx_ingest_protocol_accepted_total{protocol=\"syslog\"} 7\n"},
		{"spx_ingest_input_parsed_total", "spx_ingest_input_parsed_total{input_id=\"first\"} 2\nspx_ingest_input_parsed_total{input_id=\"second\"} 4\n"},
	} {
		if strings.Count(output, "# HELP "+group.name+" ") != 1 ||
			strings.Count(output, "# TYPE "+group.name+" counter\n") != 1 ||
			!strings.Contains(output, "# TYPE "+group.name+" counter\n"+group.samples) {
			t.Errorf("metric %s was not exposed as a single group:\n%s", group.name, output)
		}
	}
	if !strings.Contains(output, "spx_ingest_audit_outbox_oldest_seconds 25\n") {
		t.Fatalf("audit backlog age not exposed:\n%s", output)
	}
}
