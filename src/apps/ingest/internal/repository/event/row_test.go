package event_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
)

const sampleEvent = `{
  "time":1764172800123,"class_uid":4001,"category_uid":4,"activity_id":6,"type_uid":400106,"severity_id":4,
  "src_endpoint":{"ip":"192.0.2.1","port":0},"dst_endpoint":{"ip":"2001:db8::2"},
  "metadata":{"uid":"f0347ecd-9830-4eb8-8ae5-c304572c5b53","version":"1.10.0",
    "spx":{"contract_version":"v1","parse.outcome":"ok","received_at":1764172800456,"input_id":"ab1d8777-72bd-4262-81c5-b37b7e01439b",
      "vendor":"Example","product":""}}
}`

func TestProjectEventPreservesEvidenceAndTypes(t *testing.T) {
	projection, err := event.NewProjector()
	if err != nil {
		t.Fatal(err)
	}
	row, err := projection.Project([]byte(sampleEvent))
	if err != nil {
		t.Fatal(err)
	}
	for column, want := range map[string]any{
		"event_time":  "2025-11-26 16:00:00.123",
		"received_at": "2025-11-26 16:00:00.456",
		"src_ip":      "::ffff:192.0.2.1", "dst_ip": "2001:db8::2",
		"src_port": uint64(0), "dst_port": nil, "product": nil,
		"event_json": sampleEvent,
	} {
		if got := row[column]; got != want {
			t.Errorf("列 %s = %#v，期望 %#v", column, got, want)
		}
	}
}

func TestProjectEventRejectsInvalidEvidence(t *testing.T) {
	projection, err := event.NewProjector()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ match, replace string }{
		{`"contract_version":"v1"`, `"contract_version":"1.0.0"`},
		{`"severity_id":4`, `"severity_id":42`},
		{`"type_uid":400106`, `"type_uid":400100`},
		{`"port":0`, `"port":65536`},
		{`"ip":"192.0.2.1"`, `"ip":null`},
	} {
		body := bytes.Replace([]byte(sampleEvent), []byte(test.match), []byte(test.replace), 1)
		if !json.Valid(body) {
			t.Fatal("测试数据应为有效 JSON")
		}
		if _, err := projection.Project(body); err == nil {
			t.Errorf("应拒绝非法字段 %s", test.replace)
		}
	}
}
