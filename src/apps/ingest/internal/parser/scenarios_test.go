package parser_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// 回归：CEF 值里出现不配对的引号时，不能吞掉后面所有字段。
// 早年实现开启引号处理后，msg=user said "hi 会让扫描器一直留在引号内，src 直接丢失。
func TestCEFUnbalancedQuoteKeepsFollowingFields(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF},
		`CEF:0|V|P|1|1|n|3|msg=user said "hi src=1.2.3.4 act=deny`)

	if fields["src"] != "1.2.3.4" || fields["act"] != "deny" {
		t.Fatalf("fields after the quote were lost: %#v", fields)
	}
}

// CEF 规范不用引号包围值，引号按字面保留而不是被吃掉。
func TestCEFKeepsQuotesLiteral(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF},
		`CEF:0|V|P|1|1|n|3|msg="quoted value" src=1.2.3.4`)

	if fields["src"] != "1.2.3.4" {
		t.Fatalf("src lost: %#v", fields)
	}
	if fields["msg"] != `"quoted value"` {
		t.Fatalf("quotes were stripped: %#v", fields["msg"])
	}
}

// CEF 头部字段名是行业约定，规则作者直接按这些名字写映射。
func TestCEFExposesIndustryHeaderNames(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF}, "CEF:0|V|P|1|9|n|3|a=1")

	for _, name := range []string{"Version", "DeviceVendor", "DeviceProduct", "DeviceVersion", "SignatureID", "Name", "Severity"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("missing header field %q in %#v", name, fields)
		}
	}
}

// JSON 里的 null、布尔与 unicode 都要原样保留，不能被字符串化。
func TestJSONKeepsValueTypes(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeJSON},
		`{"ok":true,"missing":null,"note":"中文消息","ratio":0.25}`)

	if fields["ok"] != true {
		t.Fatalf("boolean = %#v", fields["ok"])
	}
	if fields["missing"] != nil {
		t.Fatalf("null = %#v", fields["missing"])
	}
	if fields["note"] != "中文消息" {
		t.Fatalf("unicode = %#v", fields["note"])
	}
	if fields["ratio"] != json.Number("0.25") {
		t.Fatalf("float = %#v", fields["ratio"])
	}
}

// 深层嵌套按 dot-path 摊平，便于规则用一条路径定位。
func TestJSONFlattensDeepNesting(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeJSON},
		`{"a":{"b":{"c":{"d":"deep"}}}}`)

	if fields["a.b.c.d"] != "deep" {
		t.Fatalf("deep path missing: %#v", fields)
	}
}

// 键值对的分隔符出现在引号内时不算条目边界。
func TestKVKeepsSeparatorInsideQuotes(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeKV}, `msg="a=b c" level=info`)

	if fields["msg"] != "a=b c" {
		t.Fatalf("msg = %#v", fields["msg"])
	}
	if fields["level"] != "info" {
		t.Fatalf("level = %#v", fields["level"])
	}
}

// 回归：键值对里出现不配对的引号时退化为无引号扫描，不能吞掉后面的条目。
// 设备拼错、日志被截断或有人投毒都可能造成单边引号。
func TestKVUnbalancedQuoteKeepsFollowingPairs(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeKV}, `msg=user said "hi level=warn src=1.2.3.4`)

	if fields["level"] != "warn" || fields["src"] != "1.2.3.4" {
		t.Fatalf("pairs after the quote were lost: %#v", fields)
	}
}

// 制表符分隔的日志在国产设备上很常见。
func TestDelimiterSupportsTab(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode:      parser.ModeDelimiter,
		Delimiter: parser.DelimiterSpec{Separator: "\t", Columns: []string{"time", "level", "msg"}},
	}, "2026-11-14T10:00:00Z\tWARN\tdisk almost full")

	if fields["level"] != "WARN" || fields["msg"] != "disk almost full" {
		t.Fatalf("tab fields = %#v", fields)
	}
}

// 正则可以对同一行里的多个命名组取值，且不要求锚定整行。
func TestRegexReadsFromMiddleOfLine(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode:    parser.ModeRegex,
		Pattern: `user=(?P<user>\w+) action=(?P<action>\w+)`,
	}, "2026-11-14 audit: user=alice action=login result=ok")

	if fields["user"] != "alice" || fields["action"] != "login" {
		t.Fatalf("regex fields = %#v", fields)
	}
}

// 拆分后逐条解析：NDJSON 三条消息各自独立成一条，互不影响。
func TestSplitThenParseEachMessage(t *testing.T) {
	payload := "{\"src\":\"10.0.0.1\"}\n{\"src\":\"10.0.0.2\"}\n{\"src\":\"10.0.0.3\"}\n"

	messages, err := parser.Split(parser.ModeJSON, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("split produced %d messages", len(messages))
	}

	instance, err := parser.New(parser.Spec{Mode: parser.ModeJSON})
	if err != nil {
		t.Fatal(err)
	}

	for index, message := range messages {
		fields, err := instance.Parse(message)
		if err != nil {
			t.Fatalf("message %d: %v", index, err)
		}

		want := "10.0.0." + string(rune('1'+index))
		if fields["src"] != want {
			t.Fatalf("message %d src = %#v, want %s", index, fields["src"], want)
		}
	}
}

// 拆分遇到畸形报文要报错而不是产出半条消息。
func TestSplitRejectsMalformedPayload(t *testing.T) {
	for _, payload := range []string{"", "   \n\t\n", "[{\"a\":1},"} {
		if _, err := parser.Split(parser.ModeJSON, []byte(payload)); !errors.Is(err, parser.ErrMalformed) {
			t.Fatalf("payload %q accepted: %v", payload, err)
		}
	}
}

// 构造好的解析器必须可并发复用：注册表与实现都不持有可变状态。
func TestParserIsSafeForConcurrentUse(t *testing.T) {
	instance, err := parser.New(parser.Spec{Mode: parser.ModeCEF})
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("CEF:0|V|P|1|1|n|3|src=1.2.3.4")
	done := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		go func() {
			for round := 0; round < 100; round++ {
				fields, err := instance.Parse(message)
				if err != nil {
					done <- err
					return
				}
				if fields["src"] != "1.2.3.4" {
					done <- errors.New("unexpected fields: " + strings.Join(nil, ""))
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

// 注册表里的每个模式都必须能用它的规格构造出解析器；
// 未知模式必须被拒绝。Modes 返回副本，调用方改不动注册表。
func TestValidModeMatchesRegistry(t *testing.T) {
	specs := map[string]parser.Spec{
		parser.ModeJSON:      {Mode: parser.ModeJSON},
		parser.ModeCEF:       {Mode: parser.ModeCEF},
		parser.ModeKV:        {Mode: parser.ModeKV},
		parser.ModeRegex:     {Mode: parser.ModeRegex, Pattern: `(?P<a>.+)`},
		parser.ModeDelimiter: {Mode: parser.ModeDelimiter, Delimiter: parser.DelimiterSpec{Separator: ",", Columns: []string{"a"}}},
	}

	for _, mode := range parser.Modes() {
		spec, ok := specs[mode]
		if !ok {
			t.Fatalf("mode %q has no constructor spec in this test", mode)
		}

		if _, err := parser.New(spec); err != nil {
			t.Fatalf("mode %q failed to construct: %v", mode, err)
		}

		if !parser.ValidMode(mode) {
			t.Fatalf("registered mode %q rejected by ValidMode", mode)
		}
	}

	if _, err := parser.New(parser.Spec{Mode: "protobuf"}); !errors.Is(err, parser.ErrUnknownMode) {
		t.Fatalf("unknown mode accepted: %v", err)
	}

	if parser.ValidMode("protobuf") {
		t.Fatal("unknown mode accepted by ValidMode")
	}
}

// 回归：CEF 头部字段的转义必须还原，Vendor\|Inc 不能带着反斜杠进映射。
func TestCEFUnescapesHeaderFields(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF}, `CEF:0|Vendor\|Inc|Product|1|1|n|3|a=1`)

	if fields["DeviceVendor"] != "Vendor|Inc" {
		t.Fatalf("header escape kept: %#v", fields["DeviceVendor"])
	}
}

// 回归：数组闭合后尾随垃圾必须拒绝，不能静默接受半条报文。
func TestTrailingGarbageIsRejected(t *testing.T) {
	instance, err := parser.New(parser.Spec{Mode: parser.ModeJSON})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = instance.Parse([]byte(`{"a":1} junk`)); !errors.Is(err, parser.ErrMalformed) {
		t.Fatalf("object with trailing junk accepted: %v", err)
	}

	if _, err = parser.Split(parser.ModeJSON, []byte(`[{"a":1}] junk`)); !errors.Is(err, parser.ErrMalformed) {
		t.Fatalf("array with trailing junk accepted: %v", err)
	}
}

// 超出 10MiB 上限的 JSON 必须在解码前拒绝，防止单条消息吃掉内存。
func TestJSONRejectsOversizedMessage(t *testing.T) {
	instance, err := parser.New(parser.Spec{Mode: parser.ModeJSON})
	if err != nil {
		t.Fatal(err)
	}

	oversized := `{"a":"` + strings.Repeat("x", parser.MaxJSONSize) + `"}`
	if _, err = instance.Parse([]byte(oversized)); !errors.Is(err, parser.ErrMalformed) {
		t.Fatalf("oversized message accepted: %v", err)
	}
}

// 数字保留原文：超过 2^53 的整数标识不能被 float64 截断。
func TestJSONPreservesBigIntegerPrecision(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeJSON}, `{"device_id":9007199254740993}`)

	if fields["device_id"] != json.Number("9007199254740993") {
		t.Fatalf("precision lost: %#v", fields["device_id"])
	}
}

// 纯空白输入在所有模式下行为一致：格式不符。
func TestWhitespaceOnlyMessagesAreRejectedEverywhere(t *testing.T) {
	specs := map[string]parser.Spec{
		parser.ModeJSON:      {Mode: parser.ModeJSON},
		parser.ModeCEF:       {Mode: parser.ModeCEF},
		parser.ModeKV:        {Mode: parser.ModeKV},
		parser.ModeRegex:     {Mode: parser.ModeRegex, Pattern: `(?P<a>.+)`},
		parser.ModeDelimiter: {Mode: parser.ModeDelimiter, Delimiter: parser.DelimiterSpec{Separator: ",", Columns: []string{"a"}}},
	}

	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			instance, err := parser.New(spec)
			if err != nil {
				t.Fatal(err)
			}

			if _, err = instance.Parse([]byte("   \n\t ")); !errors.Is(err, parser.ErrMalformed) {
				t.Fatalf("whitespace-only message accepted: %v", err)
			}
		})
	}
}
