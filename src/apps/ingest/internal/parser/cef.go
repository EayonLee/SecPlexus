package parser

import (
	"bytes"
	"fmt"
	"strings"
)

// CEF 头部字段名按 ArcSight 字典的大驼峰写法。
// 用行业既有名称而不是自造名字，规则作者不必再查一遍映射表。
const (
	cefFieldVersion       = "Version"
	cefFieldDeviceVendor  = "DeviceVendor"
	cefFieldDeviceProduct = "DeviceProduct"
	cefFieldDeviceVersion = "DeviceVersion"
	cefFieldSignatureID   = "SignatureID"
	cefFieldName          = "Name"
	cefFieldSeverity      = "Severity"
)

// cefHeaderFields 是 | 之前的固定段数：版本、厂商、产品、设备版本、签名、名称、级别。
const cefHeaderFields = 7

// cefAnchor 是格式锚点。syslog 转发链路会在前面加 PRI 与时间戳，
// 因此不能假设报文以 CEF 开头。
var cefAnchor = []byte("CEF:")

// cefParser 解析 ArcSight 通用事件格式。
//
// 规范没有公开的完整文档，实现按 ArcSight 字段字典与 AiSOC 的开源解析器对齐：
// 头部 7 段用未转义的 | 切分，扩展区是 key=value 且支持 \= \\ \| \n 转义。
type cefParser struct{ pairs pairOptions }

func newCEF(spec Spec) (Parser, error) {
	// CEF 的分隔符是格式规定的固定字符，不需要 KVSpec 校验。
	// 关掉引号处理是刻意的：CEF 规范不用引号包围值，而一个多余的引号
	// （msg=user said "hi 这种不配对情况）会让扫描器一直处于引号内，
	// 把后面所有字段一起吞掉。含空格的值由 keyAhead 边界判定处理。
	return cefParser{pairs: pairOptions{
		keyDelimiter: '=',
		entrySep:     ' ',
		escapes:      true,
		keyAhead:     true,
	}}, nil
}

// Parse 解析一条 CEF 消息。扩展区的键按原样进字段表，
// 这正是 CEF 的价值：src、dst、spt 等名称已是行业通用。
func (p cefParser) Parse(raw []byte) (map[string]any, error) {
	index := bytes.Index(raw, cefAnchor)
	if index < 0 {
		return nil, fmt.Errorf("%w: missing CEF: anchor", ErrMalformed)
	}

	body := raw[index+len(cefAnchor):]
	// 切 7 次得到 8 段：前 7 段是头部，最后一段是完整扩展区。
	segments := splitEscaped(body, '|', cefHeaderFields)
	if len(segments) <= cefHeaderFields {
		return nil, fmt.Errorf("%w: CEF header needs %d fields", ErrMalformed, cefHeaderFields)
	}

	fields := make(map[string]any, 12)
	fields[cefFieldVersion] = unescapeCEF(segments[0])
	fields[cefFieldDeviceVendor] = unescapeCEF(segments[1])
	fields[cefFieldDeviceProduct] = unescapeCEF(segments[2])
	fields[cefFieldDeviceVersion] = unescapeCEF(segments[3])
	fields[cefFieldSignatureID] = unescapeCEF(segments[4])
	fields[cefFieldName] = unescapeCEF(segments[5])
	fields[cefFieldSeverity] = unescapeCEF(segments[6])

	for key, value := range parsePairs([]byte(segments[cefHeaderFields]), p.pairs) {
		fields[key] = value
	}

	return fields, nil
}

// unescapeCEF 还原头部字段的转义序列（\| \= \\ \n \r \t）。
// splitEscaped 刻意保留反斜杠以避免与扩展区双重反转义；
// 头部字段不经过键值扫描器，由这里完成唯一一次还原。
func unescapeCEF(value string) string {
	if !strings.ContainsRune(value, '\\') {
		return value
	}

	var out strings.Builder
	out.Grow(len(value))
	escaped := false

	for index := 0; index < len(value); index++ {
		ch := value[index]

		switch {
		case escaped:
			out.WriteByte(unescapeByte(ch))
			escaped = false
		case ch == '\\':
			escaped = true
		default:
			out.WriteByte(ch)
		}
	}

	// 末尾孤立的反斜杠按字面保留。
	if escaped {
		out.WriteByte('\\')
	}

	return out.String()
}

// splitEscaped 在未转义的分隔符处切分，最多切 limit 次，
// 因此返回 limit+1 段，最后一段保留剩余全部内容。
//
// 只切头部所需的次数是关键：扩展区的值里合法存在 |（例如命令串），
// 多切一次就会把值截断并丢掉后面所有字段。
//
// 它只判断分隔符是否被转义，**不还原转义**：反斜杠原样留在结果里，
// 由下游的键值扫描器统一还原一次。在这里也还原会导致双重反转义——
// 扩展区的 \\t 会先变成 \t 再变成制表符。
func splitEscaped(raw []byte, separator byte, limit int) []string {
	segments := make([]string, 0, limit+1)
	current := make([]byte, 0, 64)
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		switch {
		case escaped:
			current = append(current, '\\', ch)
			escaped = false
		case ch == '\\':
			escaped = true
		case ch == separator && len(segments) < limit:
			segments = append(segments, string(current))
			current = current[:0]
		default:
			current = append(current, ch)
		}
	}

	if escaped {
		current = append(current, '\\')
	}

	return append(segments, string(current))
}
