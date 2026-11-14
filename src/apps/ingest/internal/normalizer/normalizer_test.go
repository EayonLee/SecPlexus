package normalizer_test

import (
	"errors"
	"reflect"
	"testing"

	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// receivedAt 是所有 Apply 用例的固定接收时刻，让时间断言可确定。
var receivedAt = time.Unix(1762986541, 0).UTC()

// cefRule 是供应商规则的典型形态：CEF 解析 + 字段映射 + 级别值表 + 常量 + 样本。
func cefRule() normalizer.Rule {
	return normalizer.Rule{
		ID:      "fortinet-fortigate",
		Name:    "FortiGate 攻击日志",
		Vendor:  "Fortinet",
		Product: "FortiGate",
		Parse:   parser.Spec{Mode: parser.ModeCEF},
		FieldMap: map[string]string{
			"src":          "src_endpoint.ip",
			"dst":          "dst_endpoint.ip",
			"spt":          "src_endpoint.port",
			"act":          "activity_name",
			"DeviceVendor": "device.vendor_name",
			"Name":         "finding_info.title",
		},
		Severity: normalizer.SeveritySpec{
			Field: "Severity",
			Values: map[string]int{
				"0":  normalizer.SeverityInformational,
				"5":  normalizer.SeverityMedium,
				"10": normalizer.SeverityHigh,
			},
		},
		Constants: map[string]any{"class_uid": 2004, "kind": "detection"},
		Samples: []string{
			"CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|src=1.2.3.4 dst=5.6.7.8 spt=443 act=deny",
			"CEF:0|Fortinet|FortiGate|7.4|101|Malware|10|src=9.9.9.9 dst=8.8.8.8 act=block",
		},
	}
}

func TestApplyProjectsFieldsIntoNestedOCSF(t *testing.T) {
	engine, err := normalizer.Compile(cefRule())
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte("CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|src=1.2.3.4 act=deny"), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"class_uid":     2004,
		"kind":          "detection",
		"severity_id":   normalizer.SeverityMedium,
		"activity_name": "deny",
		"finding_info":  map[string]any{"title": "Web Attack"},
		"device":        map[string]any{"vendor_name": "Fortinet"},
		"src_endpoint":  map[string]any{"ip": "1.2.3.4"},
		"time":          int64(1762986541000),
	}
	if !reflect.DeepEqual(event.Fields, want) {
		t.Fatalf("fields = %#v, want %#v", event.Fields, want)
	}
}

// 未映射字段要收集起来，规则作者靠它发现漏映射。
func TestApplyCollectsUnmappedFields(t *testing.T) {
	engine, err := normalizer.Compile(cefRule())
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte("CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|src=1.2.3.4 custom=abc"), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Unmapped["custom"] != "abc" {
		t.Fatalf("unmapped = %#v", event.Unmapped)
	}
	if _, ok := event.Unmapped["src"]; ok {
		t.Fatalf("mapped field leaked into unmapped: %#v", event.Unmapped)
	}
}

// 级别值表命中时按映射写 severity_id，大小写与空白不影响匹配。
func TestSeverityNormalizesCaseAndWhitespace(t *testing.T) {
	rule := cefRule()
	rule.Severity.Values = map[string]int{"high": normalizer.SeverityHigh}
	rule.Samples = nil

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"high", " HIGH ", "High"} {
		event, err := engine.Apply([]byte("CEF:0|F|P|1|1|n|"+value+"|src=1.1.1.1"), receivedAt)
		if err != nil {
			t.Fatal(err)
		}

		if event.Fields["severity_id"] != normalizer.SeverityHigh {
			t.Fatalf("severity for %q = %#v", value, event.Fields["severity_id"])
		}
	}
}

// 值表里没有的级别：记 warning 并落到 Unknown，事件不丢。
func TestUnmappedSeverityWarnsAndFallsBack(t *testing.T) {
	rule := cefRule()
	rule.Samples = nil

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte("CEF:0|F|P|1|1|n|99|src=1.1.1.1"), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["severity_id"] != normalizer.SeverityUnknown {
		t.Fatalf("severity = %#v", event.Fields["severity_id"])
	}
	if len(event.Warnings) == 0 {
		t.Fatal("expected a warning for the unmapped severity")
	}
}

// 时间字段按声明格式解析成 epoch 毫秒。
func TestTimeStampsEpochMillis(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "json-time",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"ts": "metadata.event_ts"},
		Time:      normalizer.TimeSpec{Field: "ts", Formats: []string{"unix"}},
		Constants: map[string]any{"class_uid": 2004},
		Samples:   []string{`{"ts":1762986541}`},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"ts":1762986541}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["time"] != int64(1762986541000) {
		t.Fatalf("time = %#v", event.Fields["time"])
	}
}

// 时间解析失败记 warning 并回退到接收时间：事件必须带时间，
// 否则保留期与窗口聚合对它失效。
func TestUnparsableTimeWarnsAndFallsBackToReceivedAt(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "json-time",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"ts": "metadata.event_ts"},
		Time:      normalizer.TimeSpec{Field: "ts", Formats: []string{"2006-01-02"}},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"ts":"not-a-date"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["time"] != int64(1762986541000) {
		t.Fatalf("fallback time = %#v, want the received time", event.Fields["time"])
	}
	if len(event.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

// 常量最后写入，不能被设备字段覆盖。
func TestConstantsOverrideMappedFields(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "const-override",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"kind": "kind"},
		Constants: map[string]any{"class_uid": 2004, "kind": "detection"},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"kind":"log"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["kind"] != "detection" {
		t.Fatalf("constant did not win: %#v", event.Fields["kind"])
	}
}

// 样本校验：任何一条样本失败，规则都不允许加载。
func TestCompileRejectsRuleWhoseSampleFails(t *testing.T) {
	rule := cefRule()
	rule.Samples = []string{"this line is not CEF at all"}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrSampleFailed) {
		t.Fatalf("bad sample accepted: %v", err)
	}
}

func TestCompileRejectsInvalidRules(t *testing.T) {
	tests := map[string]normalizer.Rule{
		"missing id": {Parse: parser.Spec{Mode: parser.ModeJSON}},
		"unknown mode": {
			ID:    "x",
			Parse: parser.Spec{Mode: "protobuf"},
		},
		"severity out of range": {
			ID:       "x",
			Parse:    parser.Spec{Mode: parser.ModeJSON},
			Severity: normalizer.SeveritySpec{Field: "level", Values: map[string]int{"a": 9}},
		},
		"severity field without table": {
			ID:       "x",
			Parse:    parser.Spec{Mode: parser.ModeJSON},
			Severity: normalizer.SeveritySpec{Field: "level"},
		},
		"time field without formats": {
			ID:    "x",
			Parse: parser.Spec{Mode: parser.ModeJSON},
			Time:  normalizer.TimeSpec{Field: "ts"},
		},
		"empty field map target": {
			ID:       "x",
			Parse:    parser.Spec{Mode: parser.ModeJSON},
			FieldMap: map[string]string{"a": " "},
		},
	}

	for name, rule := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizer.Compile(rule); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
}

// 解析失败必须冒泡为错误，交给调用方记 parse_status=error。
func TestApplyReportsParseFailure(t *testing.T) {
	engine, err := normalizer.Compile(cefRule())
	if err != nil {
		t.Fatal(err)
	}

	if _, err = engine.Apply([]byte("not a cef line"), receivedAt); !errors.Is(err, parser.ErrMalformed) {
		t.Fatalf("parse failure not reported: %v", err)
	}
}

// event_id 按（接入点，接收序号，内容）确定：同输入同输出，
// 接入点与序号隔离命名空间。内容相同但序号不同是两条真实事件。
func TestEventIDIsDeterministicPerConnector(t *testing.T) {
	message := []byte("CEF:0|F|P|1|1|n|3|src=1.1.1.1")

	first := normalizer.EventID("connector-a", 0, message)
	if again := normalizer.EventID("connector-a", 0, message); again != first {
		t.Fatalf("same input produced different ids: %s vs %s", first, again)
	}

	if other := normalizer.EventID("connector-b", 0, message); other == first {
		t.Fatal("event ids must differ across connectors")
	}

	if changed := normalizer.EventID("connector-a", 0, []byte("CEF:0|F|P|1|2|n|3|src=1.1.1.1")); changed == first {
		t.Fatal("event ids must differ across content")
	}

	if later := normalizer.EventID("connector-a", 1, message); later == first {
		t.Fatal("repeated identical events must get distinct ids")
	}
}

// 回归：unmapped 里的凭证字段必须替换为 [REDACTED]，
// 否则设备日志里的口令与 API Key 会进入事件存储与查询界面。
func TestUnmappedRedactsSensitiveFields(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "redact",
		Parse:     parser.Spec{Mode: parser.ModeKV},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`src=1.2.3.4 password=SecPlexus#2026 api_key=sk-live-abcdef user_password=hunter2 note=hello`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Unmapped["password"] != "[REDACTED]" || event.Unmapped["api_key"] != "[REDACTED]" {
		t.Fatalf("credentials leaked: %#v", event.Unmapped)
	}
	if event.Unmapped["user_password"] != "[REDACTED]" {
		t.Fatalf("fragment match failed: %#v", event.Unmapped)
	}
	if event.Unmapped["note"] != "hello" {
		t.Fatalf("non-sensitive field was altered: %#v", event.Unmapped)
	}
}

// 回归：每条事件必须带 severity_id 与 time（OCSF 最小字段）。
func TestEventsAlwaysCarrySeverityAndTime(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "minimal",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"a": "activity_name"},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"a":"login"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["severity_id"] != normalizer.SeverityUnknown {
		t.Fatalf("severity_id = %#v", event.Fields["severity_id"])
	}
	if event.Fields["time"] != int64(1762986541000) {
		t.Fatalf("time = %#v", event.Fields["time"])
	}
}

// 回归：规则必须声明 class_uid——下游按它路由，缺失即伪 OCSF。
func TestCompileRejectsRuleWithoutClassUID(t *testing.T) {
	rule := normalizer.Rule{
		ID:       "no-class",
		Parse:    parser.Spec{Mode: parser.ModeJSON},
		FieldMap: map[string]string{"a": "activity_name"},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("rule without class_uid accepted: %v", err)
	}
}

// 回归：常量目标与映射目标互为前缀会让树形写入覆盖标量，加载即拒绝。
func TestCompileRejectsConstantPrefixConflict(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "const-prefix",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"a": "x"},
		Constants: map[string]any{"class_uid": 2004, "x.y": "deep"},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("constant prefix conflict accepted: %v", err)
	}
}

// 回归：设备时区用于解析无时区的时间串——北京 10:00 不能被当成 UTC 10:00。
func TestTimeHonorsDeviceTimezone(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "tz",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"ts": "raw_ts"},
		Time:      normalizer.TimeSpec{Field: "ts", Formats: []string{"2006-01-02 15:04:05"}, Location: "Asia/Shanghai"},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"ts":"2026-11-14 10:00:00"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	// 北京 2026-11-14 10:00:00 = UTC 02:00 = 1794621600000。
	if event.Fields["time"] != int64(1794621600000) {
		t.Fatalf("time with timezone = %#v, want 1794621600000", event.Fields["time"])
	}
}

// 时区名非法时加载即拒绝。
func TestCompileRejectsInvalidTimezone(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "bad-tz",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		Time:      normalizer.TimeSpec{Field: "ts", Formats: []string{"2006"}, Location: "Mars/Olympus"},
		Constants: map[string]any{"class_uid": 2004},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("invalid timezone accepted: %v", err)
	}
}

// 回归（二轮审查）：field_map 直接映射 severity_id/time 会被引擎恒写静默覆盖，
// 必须在加载期拒绝并指路，不留"看起来能配、运行必丢值"的写法。
func TestCompileRejectsFieldMapWritingEngineOwnedKeys(t *testing.T) {
	for _, target := range []string{"severity_id", "time"} {
		t.Run(target, func(t *testing.T) {
			rule := normalizer.Rule{
				ID:        "engine-owned",
				Parse:     parser.Spec{Mode: parser.ModeJSON},
				FieldMap:  map[string]string{"src": target},
				Constants: map[string]any{"class_uid": 2004},
			}

			if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
				t.Fatalf("field_map to %q accepted: %v", target, err)
			}
		})
	}
}

// 常量是"最后盖章"机制：severity_id 常量覆盖引擎默认——固定级别源的唯一写法，必须保留。
func TestConstantSeverityOverridesEngineDefault(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "const-severity",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		FieldMap:  map[string]string{"src": "src_endpoint.ip"},
		Constants: map[string]any{"class_uid": 2004, "severity_id": normalizer.SeverityHigh},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"src":"1.1.1.1"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["severity_id"] != normalizer.SeverityHigh {
		t.Fatalf("severity_id = %#v, want the constant", event.Fields["severity_id"])
	}
}

// 回归（二轮审查）：值表键与事件值同样归一化——表键 "HIGH" 要能匹配事件值 "high"。
func TestSeverityTableKeysAreCaseInsensitive(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "table-case",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		Severity:  normalizer.SeveritySpec{Field: "level", Values: map[string]int{"HIGH": normalizer.SeverityHigh}},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		t.Fatal(err)
	}

	event, err := engine.Apply([]byte(`{"level":"high"}`), receivedAt)
	if err != nil {
		t.Fatal(err)
	}

	if event.Fields["severity_id"] != normalizer.SeverityHigh {
		t.Fatalf("severity_id = %#v, want High for the uppercase table key", event.Fields["severity_id"])
	}
}

// 回归（二轮审查）：归一化后冲突的值表键必须拒绝——否则 map 迭代顺序决定谁赢。
func TestCompileRejectsCollidingSeverityKeys(t *testing.T) {
	rule := normalizer.Rule{
		ID:    "collide",
		Parse: parser.Spec{Mode: parser.ModeJSON},
		Severity: normalizer.SeveritySpec{Field: "level", Values: map[string]int{
			"HIGH": normalizer.SeverityHigh,
			"high": normalizer.SeverityLow,
		}},
		Constants: map[string]any{"class_uid": 2004},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("colliding keys accepted: %v", err)
	}
}

// 回归（二轮审查）：class_uid 显式为 nil 是伪 OCSF，必须拒绝。
func TestCompileRejectsNilClassUID(t *testing.T) {
	rule := normalizer.Rule{
		ID:        "nil-class",
		Parse:     parser.Spec{Mode: parser.ModeJSON},
		Constants: map[string]any{"class_uid": nil},
	}

	if _, err := normalizer.Compile(rule); !errors.Is(err, normalizer.ErrInvalidRule) {
		t.Fatalf("nil class_uid accepted: %v", err)
	}
}
