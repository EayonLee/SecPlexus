package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
)

// 兜底事件也必须满足同一份契约：字段序列化失败时可以丢字段，
// 但不能让下游拿不到接入点与解析结论——排障全部依赖 metadata.spx。
func TestFallbackBodyKeepsBookkeeping(t *testing.T) {
	job := Job{Source: input.Entry{ID: "conn-1"}, Raw: []byte("payload"), Received: time.Now()}
	event, _, err := job.mixedEvent(Event{EventID: "event-1"})
	if err != nil {
		t.Fatal(err)
	}

	var body map[string]any
	if err := json.Unmarshal(event.Body, &body); err != nil {
		t.Fatalf("兜底事件不是合法 JSON: %v", err)
	}

	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v", body["metadata"])
	}

	bookkeeping, ok := metadata["spx"].(map[string]any)
	if !ok {
		t.Fatalf("兜底事件缺少 metadata.spx: %#v", metadata)
	}
	if bookkeeping["input_id"] != "conn-1" {
		t.Fatalf("metadata.spx.input_id = %#v", bookkeeping["input_id"])
	}
	if bookkeeping["parse.outcome"] != string(StatusError) {
		t.Fatalf("metadata.spx.parse.outcome = %#v", bookkeeping["parse.outcome"])
	}
	if _, ok = metadata["uid"]; !ok {
		t.Fatal("兜底事件缺少 metadata.uid")
	}
}
