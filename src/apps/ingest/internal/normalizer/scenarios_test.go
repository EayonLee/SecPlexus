package normalizer_test

import (
	"errors"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// 回归：规则自己用掉的级别与时间字段不该出现在 unmapped 里。
// unmapped 的用途是"规则漏映射了什么"，已消费字段混进去会淹没真正的问题。
func TestUnmappedExcludesConsumedFields(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "consumed",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Severity:  normalizer.SeveritySpec{Field: "level", Values: map[string]int{"high": normalizer.SeverityHigh}},
		Time:      normalizer.TimeSpec{Field: "ts", Formats: []string{"unix"}},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"src":"1.1.1.1","level":"high","ts":1762986541,"extra":"x"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	for _, consumed := range []string{"src", "level", "ts"} {
		if _, ok := event.Unmapped[consumed]; ok {
			t.Fatalf("consumed field %q leaked into unmapped: %#v", consumed, event.Unmapped)
		}
	}
	if event.Unmapped["extra"] != "x" {
		t.Fatalf("truly unmapped field missing: %#v", event.Unmapped)
	}
}

// 回归：空路径段会创建没有名字的中间键，必须在加载时拒绝。
func TestCompileRejectsMalformedPaths(t *testing.T) {
	for _, target := range []string{"x..y", ".x", "x.", "", " x", "x "} {
		t.Run(target, func(t *testing.T) {
			rule := normalizer.Rule{
				ID:       "bad-path",
				Parse:    parser.Spec{Mode: parser.ModeJSON},
				FieldMap: map[string]string{"a": target},
			}

			if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
				t.Fatalf("path %q accepted: %v", target, err)
			}
		})
	}
}

// 回归：两个源字段写同一个目标时，map 遍历顺序会让结果不确定，必须拒绝。
func TestCompileRejectsDuplicateTargets(t *testing.T) {
	rule := normalizer.Rule{
		ID:    "dup-target",
		Parse: parser.Spec{Mode: parser.ModeJSON},
		FieldMap: map[string]string{
			"src":    "src_endpoint.ip",
			"src_ip": "src_endpoint.ip",
		},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("duplicate target accepted: %v", err)
	}
}

// 回归：目标路径互为前缀会让同一棵树被两种类型写入，必须拒绝。
func TestCompileRejectsPrefixConflicts(t *testing.T) {
	rule := normalizer.Rule{
		ID:    "prefix",
		Parse: parser.Spec{Mode: parser.ModeJSON},
		FieldMap: map[string]string{
			"a": "x.y",
			"b": "x.y.z",
		},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("prefix conflict accepted: %v", err)
	}
}

// 常量路径同样要校验，否则错误只在生产事件里出现。
func TestCompileRejectsMalformedConstantPath(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "bad-constant",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		Constants: map[string]any{"class_uid": 2004, "a..b": 1},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("constant path accepted: %v", err)
	}
}

// 时间格式按声明顺序回退：第一个不匹配时用后面的。
func TestTimeFormatsFallBackInOrder(t *testing.T) {
	rule := normalizer.Rule{
		ID:       "time-fallback",
		Parse:    parser.Spec{Mode: parser.ModeJSON},
		FieldMap: map[string]string{"ts": "metadata.event_ts"},
		Time: normalizer.TimeSpec{
			Field:   "ts",
			Formats: []string{"2006-01-02T15:04:05Z07:00", "unix"},
		},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	for _, raw := range []string{`{"ts":"2026-11-14T10:00:00Z"}`, `{"ts":1762986541}`} {
		event, err := engine.Apply([]byte(raw), receivedAt)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := event.Fields["time"]; !ok {
			t.Fatalf("time missing for %s: %#v", raw, event.Fields)
		}
	}
}

// 级别字段缺失时记 warning 并落到 Unknown：OCSF 事件不允许缺级别。
func TestMissingSeverityFieldWarnsAndDefaultsToUnknown(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "missing-severity",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		Severity:  normalizer.SeveritySpec{Field: "level", Values: map[string]int{"high": normalizer.SeverityHigh}},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"msg":"no level here"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["severity_id"] != normalizer.SeverityUnknown {
		t.Fatalf("severity_id = %#v, want Unknown", event.Fields["severity_id"])
	}
	if len(event.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

// 没有未映射字段时不分配 unmapped（每事件省一次分配）。
func TestUnmappedStaysNilWhenEverythingIsMapped(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "all-mapped",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"src":"1.1.1.1"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Unmapped != nil {
		t.Fatalf("unmapped allocated unnecessarily: %#v", event.Unmapped)
	}
}

// 全链路：一次 NDJSON 报文经拆分、逐条解析与映射，产出三条带确定性 ID 的事件。
func TestPipelineSplitParseApply(t *testing.T) {
	rule := normalizer.Rule{
		ID:       "pipeline",
		Parse:    parser.Spec{Mode: parser.ModeJSON},
		FieldMap: map[string]string{"src": "src_endpoint.ip", "level": "severity_raw"},
		Severity: normalizer.SeveritySpec{Field: "level", Values: map[string]int{
			"high": normalizer.SeverityHigh,
			"low":  normalizer.SeverityLow,
		}},
		Constants: map[string]any{"class_uid": 2004, "kind": "detection"},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	payload := "{\"src\":\"10.0.0.1\",\"level\":\"high\"}\n" +
		"{\"src\":\"10.0.0.2\",\"level\":\"low\"}\n" +
		"{\"src\":\"10.0.0.3\",\"level\":\"high\"}\n"

	messages, err := parser.Split(parser.ModeJSON, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("split produced %d messages", len(messages))
	}

	seen := make(map[string]struct{}, 3)
	for index, message := range messages {
		event, err := engine.Apply(message, receivedAt)
		if err != nil {
			t.Fatalf("event %d: %v", index, err)
		}

		if event.Fields["class_uid"] != 2004 || event.Fields["kind"] != "detection" {
			t.Fatalf("event %d constants missing: %#v", index, event.Fields)
		}
		if event.Fields["src_endpoint"] == nil {
			t.Fatalf("event %d src_endpoint missing", index)
		}

		id := normalizer.EventID("connector-1", uint64(index), message)
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("event %d shares an id with an earlier message", index)
		}
		seen[id] = struct{}{}
	}
}

// 同一序号同一内容的重复计算必须得到同一个 ID；
// 序号不同的重复投递（网络重试）是两条独立投递，ID 必须不同——
// 内容相同的真实事件不能被去重吞掉。
func TestEventIDDistinguishesDeliveries(t *testing.T) {
	message := []byte(`{"src":"10.0.0.1"}`)

	first := normalizer.EventID("connector-1", 0, message)
	for round := 0; round < 5; round++ {
		if again := normalizer.EventID("connector-1", 0, message); again != first {
			t.Fatalf("id changed for the same sequence: %s vs %s", first, again)
		}
	}

	if retry := normalizer.EventID("connector-1", 1, message); retry == first {
		t.Fatal("a re-delivery with a new sequence must get a new id")
	}
}

// 引擎必须可并发复用：worker 池会同时调用同一个引擎。
func TestEngineIsSafeForConcurrentUse(t *testing.T) {
	engine, err := normalizer.Compile(cefRule())
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|src=1.2.3.4 act=deny")
	done := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		go func() {
			for round := 0; round < 50; round++ {
				event, err := engine.Apply(message, receivedAt)
				if err != nil {
					done <- err
					return
				}
				if event.Fields["severity_id"] != normalizer.SeverityMedium {
					done <- errors.New("severity stamped inconsistently")
					return
				}
			}
			done <- nil
		}()
	}

	for worker := 0; worker < 8; worker++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
