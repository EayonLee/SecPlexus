package pipeline_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

// TestPublishedContractCorpus 将实际管道产物交给独立的上游 OCSF 校验。
func TestPublishedContractCorpus(t *testing.T) {
	cases := map[int]map[string]any{
		0:    nil,
		1001: {"activity_id": 1, "device.type_id": 1, "device.hostname": "host", "actor.user.name": "user", "file.name": "test", "file.type_id": 1},
		1007: {"activity_id": 1, "device.type_id": 1, "device.hostname": "host", "actor.user.name": "user", "process.pid": 42, "process.file.name": "test", "process.file.type_id": 1},
		2002: {"activity_id": 1, "finding_info.uid": "vulnerability-1", "vulnerabilities": []any{map[string]any{"cve": map[string]any{"uid": "CVE-2026-1234"}}}},
		2004: {"activity_id": 1, "finding_info.uid": "detection-1"},
		3001: {"activity_id": 1, "user.name": "user"},
		3002: {"activity_id": 1, "user.name": "user", "dst_endpoint.ip": "192.0.2.1"},
		3007: {"activity_id": 1, "user.name": "user"},
		4001: {"activity_id": 6, "connection_info.direction_id": 0, "src_endpoint.ip": "192.0.2.1"},
		4002: {"activity_id": 3, "http_response.code": 200, "http_request.url.path": "/"},
		4003: {"activity_id": 1, "query.hostname": "example.test", "dst_endpoint.ip": "192.0.2.1"},
		6003: {"activity_id": 1, "api.operation": "List", "actor.user.name": "user", "src_endpoint.ip": "192.0.2.1", "cloud.provider": "Example"},
	}
	catalog, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	line := pipeline.New(pipeline.Options{}, &nullPublisher{})
	observed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	corpus := make([]json.RawMessage, 0, len(cases))
	for _, class := range catalog.Classes() {
		fields, ok := cases[class.UID]
		if !ok {
			t.Fatalf("no corpus fixture for class %d", class.UID)
		}
		source := input.Entry{ID: "a506dd06-f370-4400-8945-39cf658cc9ee", Name: "Fixture", Vendor: "Example", Product: "Fixture", Protocol: input.ProtocolHTTP}
		if class.UID != 0 {
			definition, err := json.Marshal(map[string]any{
				"contract_version": events.CurrentVersion, "parse": map[string]string{"mode": "json"},
				"branches": []map[string]any{{"id": "fixture", "name": "Fixture", "class_uid": class.UID, "constants": fields}},
			})
			if err != nil {
				t.Fatal(err)
			}
			source.Mapping, err = normalizer.CompileDefinition(definition)
			if err != nil {
				t.Fatalf("class %d could not compile: %v", class.UID, err)
			}
		}
		event := line.Parse(source, []byte(`{"fixture":true}`), observed, "d228c5d2-1e97-4848-8840-893564bc6bba")
		var payload struct {
			ClassUID int `json:"class_uid"`
		}
		if err := json.Unmarshal(event.Body, &payload); err != nil || payload.ClassUID != class.UID {
			t.Fatalf("class %d produced class %d, outcome %v issues %v: %v", class.UID, payload.ClassUID, event.Outcome, event.Issues, err)
		}
		corpus = append(corpus, event.Body)
	}
	if file := os.Getenv("SPX_TEST_OCSF_CORPUS"); file != "" {
		body, err := json.Marshal(corpus)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
