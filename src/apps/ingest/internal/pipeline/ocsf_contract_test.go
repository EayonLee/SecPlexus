package pipeline_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 契约：每个发布出去的事件都必须满足 OCSF base_event 的必填项。
//
// 缺了这些字段，下游无法分类、无法建表、也无法去重；
// 而它们是由管道补齐的，任何一处漏补都只会在消费方暴露。
func TestEveryEventSatisfiesOcsfBaseContract(t *testing.T) {
	line := pipeline.New(pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 10 * time.Millisecond}, &nullPublisher{})
	entry := input.Entry{ID: "conn-1", Protocol: input.ProtocolHTTP, Mapping: jsonRule(t)}

	for _, payload := range []string{
		`{"src":"10.0.0.7"}`,
		`{"src":"10.0.0.8","severity_id":4,"class_uid":2004,"activity_id":1}`,
		`not json at all`,
		`{}`,
	} {
		event := line.Parse(entry, []byte(payload), time.Now())

		var body map[string]any
		if err := json.Unmarshal(event.Body, &body); err != nil {
			t.Fatalf("%s: 事件不是合法 JSON: %v", payload, err)
		}

		assertOcsfBase(t, payload, body, event.EventID)
	}
}

func assertOcsfBase(t *testing.T, payload string, body map[string]any, eventID string) {
	t.Helper()

	classUID, ok := numeric(body["class_uid"])
	if !ok || classUID == 0 {
		t.Fatalf("%s: class_uid 缺失或非法: %#v", payload, body["class_uid"])
	}
	activityID, ok := numeric(body["activity_id"])
	if !ok {
		t.Fatalf("%s: activity_id 缺失", payload)
	}
	categoryUID, ok := numeric(body["category_uid"])
	if !ok || categoryUID != classUID/1000 {
		t.Fatalf("%s: category_uid = %#v, 期望 %d", payload, body["category_uid"], classUID/1000)
	}
	typeUID, ok := numeric(body["type_uid"])
	if !ok || typeUID != classUID*100+activityID {
		t.Fatalf("%s: type_uid = %#v, 期望 %d", payload, body["type_uid"], classUID*100+activityID)
	}

	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("%s: metadata 缺失", payload)
	}
	if metadata["version"] != pipeline.OCSFVersion {
		t.Fatalf("%s: metadata.version = %#v", payload, metadata["version"])
	}
	product, ok := metadata["product"].(map[string]any)
	if !ok || product["name"] == "" || product["vendor_name"] == "" {
		t.Fatalf("%s: metadata.product 不完整: %#v", payload, metadata["product"])
	}
	if metadata["uid"] != eventID {
		t.Fatalf("%s: metadata.uid = %#v, 期望事件标识 %q", payload, metadata["uid"], eventID)
	}

	// 我方簿记固定在 metadata.spx：塞到 metadata 顶层会占用 OCSF 预留语义。
	bookkeeping, ok := metadata["spx"].(map[string]any)
	if !ok {
		t.Fatalf("%s: metadata.spx 缺失: %#v", payload, metadata)
	}
	if bookkeeping["input_id"] != "conn-1" {
		t.Fatalf("%s: metadata.spx.input_id = %#v", payload, bookkeeping["input_id"])
	}
	if outcome, _ := bookkeeping["parse.outcome"].(string); outcome == "" {
		t.Fatalf("%s: metadata.spx 缺少 parse.outcome", payload)
	}
}

func numeric(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	}

	return 0, false
}

type nullPublisher struct{}

func (*nullPublisher) Publish(_ context.Context, events []pipeline.Event) error { return nil }
