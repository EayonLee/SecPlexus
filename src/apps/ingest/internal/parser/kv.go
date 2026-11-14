package parser

import (
	"fmt"
	"strings"
)

// kvParser 解析 key=value 形式的字段表。
type kvParser struct {
	pairs pairOptions
}

func newKV(spec Spec) (Parser, error) {
	options, err := spec.KV.options(false)
	if err != nil {
		return nil, err
	}

	return kvParser{pairs: options}, nil
}

// Parse 把整条消息当作一组键值对。没有解析出任何字段时按格式不符处理，
// 这样规则配错分隔符会在样本校验阶段就暴露，而不是静默产出空事件。
func (p kvParser) Parse(raw []byte) (map[string]any, error) {
	pairs := parsePairs(raw, p.pairs)
	if len(pairs) == 0 {
		return nil, fmt.Errorf("%w: no key-value pairs found", ErrMalformed)
	}

	fields := make(map[string]any, len(pairs))
	for key, value := range pairs {
		fields[key] = value
	}

	return fields, nil
}

// options 校验分隔符并转换成扫描器参数。
// escapes 只对 CEF 开启：通用键值对里的反斜杠是普通字符（Windows 路径很常见），
// 把它当转义会把 C:\\temp 变成制表符。
func (spec KVSpec) options(escapes bool) (pairOptions, error) {
	delimiter := spec.Delimiter
	if delimiter == "" {
		delimiter = "="
	}
	entry := spec.Entry
	if entry == "" {
		entry = " "
	}

	if len(delimiter) != 1 {
		return pairOptions{}, fmt.Errorf("%w: kv delimiter must be a single character", ErrInvalidSpec)
	}
	if len(entry) != 1 {
		return pairOptions{}, fmt.Errorf("%w: kv entry separator must be a single character", ErrInvalidSpec)
	}
	quote := spec.Quote
	if quote == "" {
		// 默认支持双引号：logfmt 是键值对的主流写法，没有引号就无法表达含空格的值
		// （msg="login failed" 会被切成两个条目）。
		quote = "\""
	}
	if len(quote) > 1 {
		return pairOptions{}, fmt.Errorf("%w: kv quote must be at most one character", ErrInvalidSpec)
	}
	if delimiter == entry {
		return pairOptions{}, fmt.Errorf("%w: kv delimiter and entry separator must differ", ErrInvalidSpec)
	}

	options := pairOptions{
		keyDelimiter: delimiter[0],
		entrySep:     entry[0],
		quote:        quote[0],
		escapes:      escapes,
	}

	return options, nil
}

// pairOptions 是键值扫描器的运行期参数。
type pairOptions struct {
	keyDelimiter byte // 键与值之间的分隔符。
	entrySep     byte // 条目之间的分隔符。
	quote        byte // 引号字符；0 表示不支持引号包围。
	escapes      bool // 是否解析反斜杠转义（CEF 需要，通用键值对不需要）。
	keyAhead     bool // 条目分隔符后必须紧跟 key= 才切分；CEF 的扩展值含空格时需要。
}

// parsePairs 把一组键值对切成字段表。
//
// 用状态机而不是 strings.Split：值里可能出现引号包围的条目分隔符，
// 因此不能先按分隔符切再找等号。键取第一个未被转义的等号之前的部分，
// 值可以包含任意等号（base64、命令行参数都很常见）。
func parsePairs(raw []byte, options pairOptions) map[string]string {
	fields, closed := scanPairs(raw, options)
	if closed {
		return fields
	}

	// 引号没闭合说明这条消息的引号是不配对的（设备拼错、日志被截断、或有人故意投毒）。
	// 与其把后面所有条目都吞进一个值，不如退一步：关掉引号处理重扫一次，
	// 让每个 key= 都还能被读出来。代价只在畸形消息上付一次。
	options.quote = 0
	fields, _ = scanPairs(raw, options)

	return fields
}

// scanPairs 执行一次扫描，并报告引号是否配对。
func scanPairs(raw []byte, options pairOptions) (map[string]string, bool) {
	fields := make(map[string]string, 8)
	pair := make([]byte, 0, 64)
	inQuote := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		switch {
		case escaped:
			pair = append(pair, unescapeByte(ch))
			escaped = false
		case options.escapes && ch == '\\':
			escaped = true
		case options.quote != 0 && ch == options.quote:
			inQuote = !inQuote
		case !inQuote && ch == options.entrySep && (!options.keyAhead || keyFollows(raw[i+1:], options.keyDelimiter)):
			appendPair(fields, pair, options.keyDelimiter)
			pair = pair[:0]
		default:
			pair = append(pair, ch)
		}
	}

	// 转义符出现在末尾时按字面处理，不丢字符。
	if escaped {
		pair = append(pair, '\\')
	}
	appendPair(fields, pair, options.keyDelimiter)

	return fields, !inQuote
}

// keyFollows 判断分隔符之后是否紧跟一个 key= 起始。
// 用来在 CEF 里区分"值中的空格"与"下一个键的边界"：
// msg=login failed user=bob 中只有第二个空格才是边界。
func keyFollows(rest []byte, keyDelimiter byte) bool {
	index := 0
	for index < len(rest) && isKeyByte(rest[index]) {
		index++
	}

	return index > 0 && index < len(rest) && rest[index] == keyDelimiter
}

// isKeyByte 判断字节是否可以用作键的一部分。
func isKeyByte(ch byte) bool {
	switch {
	case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
		return true
	case ch == '_' || ch == '.' || ch == '-':
		return true
	default:
		return false
	}
}

// appendPair 在第一个键分隔符处切开一个条目；没有分隔符或键为空时丢弃。
func appendPair(fields map[string]string, pair []byte, keyDelimiter byte) {
	key, value, found := strings.Cut(string(pair), string(keyDelimiter))
	if !found {
		return
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return
	}

	fields[key] = strings.TrimSpace(value)
}

// unescapeByte 还原 CEF 约定的转义序列，其余字符原样保留。
func unescapeByte(ch byte) byte {
	switch ch {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	default:
		return ch
	}
}
