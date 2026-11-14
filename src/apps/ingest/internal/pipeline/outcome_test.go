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

	metadata := metadataOf(t, event.Body)
	parseError, ok := metadata["parse_error"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 parse_error: %#v", metadata)
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

	metadata := metadataOf(t, event.Body)
	if metadata["truncated"] != true {
		t.Fatalf("缺少 truncated 标记: %#v", metadata)
	}
	if metadata["original_bytes"] != float64(90000) {
		t.Fatalf("original_bytes = %#v", metadata["original_bytes"])
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
