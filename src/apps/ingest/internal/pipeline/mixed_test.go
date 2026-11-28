package pipeline_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

const mixedTestRule = `{
		"contract_version":"v1","parse":{"mode":"cef"},
		"branches":[
			{"id":"traffic","name":"Traffic","class_uid":4001,"match":[{"field":"cat","equals":"traffic:forward"}],"field_map":{"src":"src_endpoint.ip","dst":"dst_endpoint.ip"}},
			{"id":"auth","name":"Authentication","class_uid":3002,"match":[{"field":"cat","in":["event:system"]}],"field_map":{"duser":"user.name","dst":"dst_endpoint.ip"},"constants":{"activity_id":1,"status_id":2}}
		]
	}`

func TestMixedSourceArchivesEachAcceptedRecord(t *testing.T) {
	rule, err := normalizer.CompileDefinition([]byte(mixedTestRule))
	if err != nil {
		t.Fatal(err)
	}
	line := pipeline.New(pipeline.Options{}, &nullPublisher{})
	source := input.Entry{ID: "source", Protocol: input.ProtocolHTTP, Mapping: rule}
	for _, tc := range []struct {
		name, fields, status string
		class                int
	}{
		{"traffic", "cat=traffic:forward src=192.0.2.1 dst=192.0.2.2", "ok", 4001},
		{"login failure", "cat=event:system duser=operator dst=192.0.2.2", "ok", 3002},
		{"bad source address", "cat=traffic:forward src=bad dst=192.0.2.2", "partial", 4001},
		{"missing identity", "cat=event:system dst=192.0.2.2", "partial", 0},
		{"unclassified", "cat=event:unknown password=private-device-marker", "error", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte("CEF:0|Fortinet|FortiGate|1|7|Log|4|" + tc.fields)
			received := time.Unix(1700000000, 0)
			event := line.Parse(source, raw, received, "")
			preview, _, err := pipeline.PreviewSource(source, string(raw), received)
			if err != nil || preview.Outcome != event.Outcome || preview.BranchID != event.BranchID {
				t.Fatalf("preview differs from published outcome: %v", err)
			}
			if bytes.Contains(event.Body, []byte("private-device-marker")) {
				t.Fatal("unclassified event exposed a source secret")
			}
			var body struct {
				Class       int                 `json:"class_uid"`
				Destination struct{ IP string } `json:"dst_endpoint"`
				Unmapped    map[string]any
				Metadata    struct{ SPX map[string]any }
			}
			if err := json.Unmarshal(event.Body, &body); err != nil {
				t.Fatal(err)
			}
			if body.Class != tc.class || body.Metadata.SPX["parse.outcome"] != tc.status {
				t.Fatalf("unexpected event: %s", event.Body)
			}
			if tc.name == "bad source address" && body.Destination.IP != "192.0.2.2" ||
				tc.name == "missing identity" && body.Unmapped["dst_endpoint"] == nil {
				t.Fatalf("event discarded a validated destination: %s", event.Body)
			}
		})
	}
}

func BenchmarkMixedCEFEvent(b *testing.B) {
	mixed, err := normalizer.CompileDefinition([]byte(mixedTestRule))
	if err != nil {
		b.Fatal(err)
	}
	logs := [][]byte{
		[]byte("CEF:0|Fortinet|FortiGate|7|7|Traffic|4|cat=traffic:forward src=192.0.2.1 dst=192.0.2.2"),
		[]byte("CEF:0|Fortinet|FortiGate|7|32002|Login|6|cat=event:system duser=operator dst=192.0.2.2"),
		[]byte("CEF:0|Fortinet|FortiGate|7|9|Other|4|cat=event:other"),
	}
	received := time.Unix(1700000000, 0)
	for _, mode := range []struct {
		name string
		rule *normalizer.Compiled
	}{{"mixed_rule", mixed}} {
		b.Run(mode.name, func(b *testing.B) {
			line := pipeline.New(pipeline.Options{}, &nullPublisher{})
			source := input.Entry{ID: "benchmark", Protocol: input.ProtocolHTTP, Mapping: mode.rule}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				line.Parse(source, logs[i%len(logs)], received, "a0b10219-0654-464a-8981-d3a656162b5b")
			}
		})
	}
}
