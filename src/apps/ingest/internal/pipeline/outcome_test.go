package pipeline_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// metadataOf 取出事件体里的 metadata。
func metadataOf(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("事件体不是 JSON: %v", err)
	}

	metadata, ok := envelope["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 metadata: %#v", envelope)
	}

	return metadata
}

// 解析结果必须把"哪一层失败、为什么、具体是什么"分开表达。
// 只给一个字符串时，聚合告警与人工排查只能二选一。
func TestOutcomeSeparatesStageReasonAndDetail(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`not json`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	_ = line.Flush(context.Background())

	event := publisher.events()[0]
	if event.Outcome.Stage != pipeline.StageDecode {
		t.Fatalf("stage = %q, want decode", event.Outcome.Stage)
	}
	if event.Outcome.Reason != pipeline.ReasonFormatMismatch {
		t.Fatalf("reason = %q", event.Outcome.Reason)
	}
	if event.Outcome.Detail == "" {
		t.Fatal("detail 为空：排查时不知道具体错在哪")
	}

	bookkeeping, ok := metadataOf(t, event.Body)["spx"].(map[string]any)
	if !ok {
		t.Fatal("metadata.spx 缺失")
	}

	parseError, ok := bookkeeping["parse_error"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 parse_error: %#v", bookkeeping)
	}
	for _, key := range []string{"stage", "reason", "message"} {
		if _, exists := parseError[key]; !exists {
			t.Fatalf("parse_error 缺字段 %q: %#v", key, parseError)
		}
	}
}

// 未挂规则与格式不符是两件事：前者要补规则，后者要查报文。
func TestNoRuleIsDistinctFromFormatMismatch(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-generic", Channel: connector.ChannelHTTP}

	if err := line.Submit(context.Background(), snapshot, []byte("raw line"), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}

	_ = line.Flush(context.Background())

	event := publisher.events()[0]
	if event.Outcome.Reason != pipeline.ReasonNoRule {
		t.Fatalf("reason = %q, want no_rule", event.Outcome.Reason)
	}
	if event.Outcome.Stage != pipeline.StageMap {
		t.Fatalf("stage = %q, want map", event.Outcome.Stage)
	}
}

// 回归：截断不能覆盖规则产出的 class_uid 与 severity_id。
// 头部完整的 CEF 仍属于 2004，覆盖成通用类会让它落进错误分类。
func TestTruncationKeepsRuleClassUID(t *testing.T) {
	engine, err := normalizer.Compile(normalizer.Rule{
		ID:        "cef-rule",
		Parse:     parser.Spec{Mode: parser.ModeCEF},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Severity:  normalizer.SeveritySpec{Field: "Severity", Values: map[string]int{"5": normalizer.SeverityMedium}},
		Constants: map[string]any{"class_uid": 2004, "kind": "detection"},
	})
	if err != nil {
		t.Fatal(err)
	}

	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-cef", Channel: connector.ChannelSyslog, Engine: engine}

	meta := pipeline.Meta{Truncated: true, OriginalBytes: 90000, DroppedBytes: 20000}
	if err = line.Submit(context.Background(), snapshot,
		[]byte("CEF:0|F|P|1|1|n|5|src=1.2.3.4"), time.Now(), meta); err != nil {
		t.Fatal(err)
	}

	_ = line.Flush(context.Background())

	event := publisher.events()[0]
	if event.Outcome.Reason != pipeline.ReasonTruncated {
		t.Fatalf("reason = %q, want truncated", event.Outcome.Reason)
	}
	if event.Outcome.Stage != pipeline.StageLimit {
		t.Fatalf("stage = %q, want limit", event.Outcome.Stage)
	}

	var envelope map[string]any
	if err = json.Unmarshal(event.Body, &envelope); err != nil {
		t.Fatal(err)
	}

	// 规则给出的类与级别必须保留。
	if envelope["class_uid"] != float64(2004) {
		t.Fatalf("class_uid 被覆盖: %#v", envelope["class_uid"])
	}
	if envelope["severity_id"] != float64(normalizer.SeverityMedium) {
		t.Fatalf("severity_id 被覆盖: %#v", envelope["severity_id"])
	}
	// 已解析出的字段也要保留。
	if envelope["src_endpoint"] == nil {
		t.Fatal("解析出的字段丢失")
	}

	bookkeeping, ok := metadataOf(t, event.Body)["spx"].(map[string]any)
	if !ok {
		t.Fatal("metadata.spx 缺失")
	}
	if bookkeeping["truncated"] != true {
		t.Fatalf("缺少 truncated 标记: %#v", bookkeeping)
	}
	if bookkeeping["original_bytes"] != float64(90000) {
		t.Fatalf("original_bytes = %#v", bookkeeping["original_bytes"])
	}
}

// 截断说明要能说清"保留多少、原始多少"，否则运维无法判断丢了多少。
func TestTruncationDetailReportsBytes(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	meta := pipeline.Meta{Truncated: true, OriginalBytes: 100000, DroppedBytes: 34464}
	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"1.1.1.1"}`), time.Now(), meta); err != nil {
		t.Fatal(err)
	}

	_ = line.Flush(context.Background())

	detail := publisher.events()[0].Outcome.Detail
	if detail == "" {
		t.Fatal("detail 为空")
	}
	t.Logf("截断说明: %s", detail)
}

// 回归（P0）：OCSF base_event 的 required 属性必须齐全。
// 依据 ocsf/ocsf-schema events/base_event.json：category_uid、type_uid、
// activity_id、metadata 均为 required；objects/metadata.json 又要求 version 与 product。
func TestEmittedEventCarriesOcsfRequiredAttributes(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	if err := line.Submit(context.Background(), snapshot, []byte(`{"src":"10.0.0.7"}`), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	var envelope map[string]any
	if err := json.Unmarshal(publisher.events()[0].Body, &envelope); err != nil {
		t.Fatal(err)
	}

	// class_uid 2004 → category_uid = 2004/1000 = 2。
	if envelope["category_uid"] != float64(2) {
		t.Fatalf("category_uid = %#v，期望 2", envelope["category_uid"])
	}
	// type_uid = class_uid*100 + activity_id；未声明活动时为 0。
	if envelope["type_uid"] != float64(200400) {
		t.Fatalf("type_uid = %#v，期望 200400", envelope["type_uid"])
	}
	if envelope["activity_id"] != float64(0) {
		t.Fatalf("activity_id = %#v，期望 0", envelope["activity_id"])
	}

	metadata, ok := envelope["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata 不是对象: %#v", envelope["metadata"])
	}
	if metadata["version"] == nil || metadata["version"] == "" {
		t.Fatal("metadata.version 缺失（OCSF 必填）")
	}
	product, ok := metadata["product"].(map[string]any)
	if !ok || product["name"] == nil || product["vendor_name"] == nil {
		t.Fatalf("metadata.product 缺失或不完整: %#v", metadata["product"])
	}

	// 事件标识映射到 OCSF 的 metadata.uid。
	if metadata["uid"] != publisher.events()[0].EventID {
		t.Fatalf("metadata.uid = %#v，期望 event_id", metadata["uid"])
	}

	// 我方簿记必须落在扩展命名空间，不能占用 OCSF 标准属性。
	bookkeeping, ok := metadata["spx"].(map[string]any)
	if !ok {
		t.Fatalf("私有簿记未放进 metadata.spx: %#v", metadata)
	}
	if bookkeeping["connector_id"] != "conn-1" || bookkeeping["parse_status"] != "ok" {
		t.Fatalf("metadata.spx = %#v", bookkeeping)
	}
}

// 回归（P1）：规则未映射的源字段曾被整体丢弃，
// 导致"未映射字段一条不丢"（spec）不成立、脱敏代码也是死代码。
// 它们现在进 OCSF 的 unmapped 属性，且敏感键已脱敏。
func TestUnmappedFieldsReachTheEvent(t *testing.T) {
	line, publisher := start(t, pipeline.Options{Workers: 1, BatchSize: 1, FlushInterval: 20 * time.Millisecond})
	snapshot := connector.Snapshot{ID: "conn-1", Channel: connector.ChannelHTTP, Engine: jsonRule(t)}

	payload := `{"src":"10.0.0.7","password":"hunter2","vendor_note":"keep me"}`
	if err := line.Submit(context.Background(), snapshot, []byte(payload), time.Now(), pipeline.Meta{}); err != nil {
		t.Fatal(err)
	}
	_ = line.Flush(context.Background())

	var envelope map[string]any
	if err := json.Unmarshal(publisher.events()[0].Body, &envelope); err != nil {
		t.Fatal(err)
	}

	unmapped, ok := envelope["unmapped"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 unmapped: %#v", envelope)
	}
	if unmapped["vendor_note"] != "keep me" {
		t.Fatalf("未映射字段丢失: %#v", unmapped)
	}
	if unmapped["password"] == "hunter2" {
		t.Fatalf("敏感值未脱敏: %#v", unmapped["password"])
	}
}
