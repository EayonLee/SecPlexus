package parser_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// parse 按调用方的方式构造解析器并解析一条消息。
func parse(t *testing.T, spec parser.Spec, message string) map[string]any {
	t.Helper()

	instance, err := parser.New(spec)
	if err != nil {
		t.Fatalf("construct parser: %v", err)
	}

	fields, err := instance.Parse([]byte(message))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}

	return fields
}

func TestJSONFlattensNestedObjects(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeJSON},
		`{"time":1762986541200,"level":"high","src_endpoint":{"ip":"10.20.3.44","port":443},"tags":["a","b"]}`)

	want := map[string]any{
		"time":              json.Number("1762986541200"),
		"level":             "high",
		"src_endpoint.ip":   "10.20.3.44",
		"src_endpoint.port": json.Number("443"),
		"tags":              []any{"a", "b"},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("flattened fields = %#v, want %#v", fields, want)
	}
}

func TestCEFParsesHeaderAndExtension(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF},
		"CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|src=1.2.3.4 dst=5.6.7.8 spt=443 act=deny")

	want := map[string]any{
		"Version":       "0",
		"DeviceVendor":  "Fortinet",
		"DeviceProduct": "FortiGate",
		"DeviceVersion": "7.4",
		"SignatureID":   "100",
		"Name":          "Web Attack",
		"Severity":      "5",
		"src":           "1.2.3.4",
		"dst":           "5.6.7.8",
		"spt":           "443",
		"act":           "deny",
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("CEF fields = %#v, want %#v", fields, want)
	}
}

// syslog 中继会在 CEF 前加 PRI 与时间戳，解析必须找到锚点而不是断言开头。
func TestCEFFindsAnchorBehindSyslogPrefix(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF},
		"<134>Nov 14 10:00:00 fw CEF:0|V|P|1.0|7|name|3|src=10.0.0.1")

	if fields["DeviceVendor"] != "V" || fields["src"] != "10.0.0.1" {
		t.Fatalf("anchored CEF fields = %#v", fields)
	}
}

// 扩展区的值里合法存在 |，多切一次会截断值并丢掉后续字段。
func TestCEFKeepsPipeInsideExtensionValue(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF},
		`CEF:0|V|P|1.0|7|name|3|cmd=powershell -enc AAA | iex src=10.0.0.1`)

	if fields["src"] != "10.0.0.1" {
		t.Fatalf("trailing field dropped: %#v", fields)
	}
	if fields["cmd"] != "powershell -enc AAA | iex" {
		t.Fatalf("value truncated at pipe: %#v", fields["cmd"])
	}
}

// CEF 的 \= 与 \\ 转义必须还原，否则值与键都会带反斜杠。
func TestCEFUnescapesExtension(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeCEF},
		`CEF:0|V|P|1.0|7|n|3|msg=path\\to\\file equal\=sign`)

	if fields["msg"] != "path\\to\\file equal=sign" {
		t.Fatalf("unescaped value = %q", fields["msg"])
	}
}

func TestKVParsesLogfmt(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeKV}, "level=info msg=\"hello world\" src=1.2.3.4")

	want := map[string]any{"level": "info", "msg": "hello world", "src": "1.2.3.4"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("logfmt fields = %#v, want %#v", fields, want)
	}
}

func TestKVHonorsCustomSeparators(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode: parser.ModeKV,
		KV:   parser.KVSpec{Delimiter: ":", Entry: ","},
	}, "user:alice,action:login")

	if fields["user"] != "alice" || fields["action"] != "login" {
		t.Fatalf("custom separator fields = %#v", fields)
	}
}

// 通用键值对不解析反斜杠：Windows 路径里的 \t 不能被当成制表符。
func TestKVKeepsBackslashLiteral(t *testing.T) {
	fields := parse(t, parser.Spec{Mode: parser.ModeKV}, `path=C:\\temp\\log.txt level=info`)

	if fields["path"] != `C:\\temp\\log.txt` {
		t.Fatalf("path = %q", fields["path"])
	}
}

func TestRegexExtractsNamedGroups(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode:    parser.ModeRegex,
		Pattern: `(?P<time>[A-Z][a-z]{2} +\d+ \d+:\d+:\d+) (?P<host>\S+) (?P<msg>.*)`,
	}, "Nov 14 10:00:01 fw01 user login failed")

	want := map[string]any{"time": "Nov 14 10:00:01", "host": "fw01", "msg": "user login failed"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("regex fields = %#v, want %#v", fields, want)
	}
}

// 未匹配的可选组不进结果，避免空字符串污染后续映射。
func TestRegexSkipsUnmatchedOptionalGroup(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode:    parser.ModeRegex,
		Pattern: `(?P<level>ERROR|WARN)?:? ?(?P<msg>.+)`,
	}, "disk full")

	if _, ok := fields["level"]; ok {
		t.Fatalf("unmatched group leaked: %#v", fields)
	}
	if fields["msg"] != "disk full" {
		t.Fatalf("msg = %#v", fields["msg"])
	}
}

func TestDelimiterMapsColumnsByPosition(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode:      parser.ModeDelimiter,
		Delimiter: parser.DelimiterSpec{Separator: ",", Columns: []string{"time", "level", "src", "msg"}},
	}, `2026-11-14,high,1.2.3.4,"sql injection detected"`)

	want := map[string]any{"time": "2026-11-14", "level": "high", "src": "1.2.3.4", "msg": "sql injection detected"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("delimiter fields = %#v, want %#v", fields, want)
	}
}

// 列名多于实际列数时只填存在的列，不 panic 也不塞空值。
func TestDelimiterToleratesShortRecord(t *testing.T) {
	fields := parse(t, parser.Spec{
		Mode:      parser.ModeDelimiter,
		Delimiter: parser.DelimiterSpec{Separator: ",", Columns: []string{"a", "b", "c"}},
	}, "1,2")

	if len(fields) != 2 || fields["a"] != "1" || fields["b"] != "2" {
		t.Fatalf("short record fields = %#v", fields)
	}
}

func TestNewRejectsInvalidSpec(t *testing.T) {
	tests := map[string]parser.Spec{
		"unknown mode":       {Mode: "protobuf"},
		"empty pattern":      {Mode: parser.ModeRegex},
		"invalid pattern":    {Mode: parser.ModeRegex, Pattern: "(?P<a>"},
		"no named group":     {Mode: parser.ModeRegex, Pattern: "(\\d+)"},
		"multichar kv delim": {Mode: parser.ModeKV, KV: parser.KVSpec{Delimiter: "=="}},
		"kv equal seps":      {Mode: parser.ModeKV, KV: parser.KVSpec{Delimiter: "=", Entry: "="}},
		"delimiter no cols":  {Mode: parser.ModeDelimiter},
		"multichar delimiter": {
			Mode:      parser.ModeDelimiter,
			Delimiter: parser.DelimiterSpec{Separator: "::", Columns: []string{"a"}},
		},
	}

	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parser.New(spec); !errors.Is(err, parser.ErrInvalidSpec) && !errors.Is(err, parser.ErrUnknownMode) {
				t.Fatalf("spec accepted or wrong error: %v", err)
			}
		})
	}
}

func TestParseReportsMalformedMessages(t *testing.T) {
	tests := map[string]struct {
		spec    parser.Spec
		message string
	}{
		"json not object":  {parser.Spec{Mode: parser.ModeJSON}, `"text"`},
		"json empty":       {parser.Spec{Mode: parser.ModeJSON}, "   "},
		"cef no anchor":    {parser.Spec{Mode: parser.ModeCEF}, "not a cef line"},
		"cef short header": {parser.Spec{Mode: parser.ModeCEF}, "CEF:0|V|P|1.0|7|name"},
		"kv no pairs":      {parser.Spec{Mode: parser.ModeKV}, "plain text only"},
		"regex no match":   {parser.Spec{Mode: parser.ModeRegex, Pattern: `(?P<a>\d+)`}, "none"},
		"delimiter empty":  {parser.Spec{Mode: parser.ModeDelimiter, Delimiter: parser.DelimiterSpec{Separator: ",", Columns: []string{"a"}}}, ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			instance, err := parser.New(test.spec)
			if err != nil {
				t.Fatalf("construct parser: %v", err)
			}

			if _, err = instance.Parse([]byte(test.message)); !errors.Is(err, parser.ErrMalformed) {
				t.Fatalf("malformed message accepted: %v", err)
			}
		})
	}
}

// Split 的三种 JSON 形态与行模式都要能拆开；拆分结果直接交给 Parse。
func TestSplitHandlesJSONShapesAndLines(t *testing.T) {
	tests := map[string]struct {
		mode     string
		payload  string
		messages int
	}{
		"single object": {parser.ModeJSON, `{"a":1}`, 1},
		"object array":  {parser.ModeJSON, `[{"a":1},{"a":2},{"a":3}]`, 3},
		"ndjson":        {parser.ModeJSON, "{\"a\":1}\n{\"a\":2}\n", 2},
		"cef lines":     {parser.ModeCEF, "CEF:0|V|P|1|1|n|1|a=1\nCEF:0|V|P|1|2|n|1|a=2", 2},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			messages, err := parser.Split(test.mode, []byte(test.payload))
			if err != nil {
				t.Fatal(err)
			}
			if len(messages) != test.messages {
				t.Fatalf("message count = %d, want %d", len(messages), test.messages)
			}
		})
	}
}
