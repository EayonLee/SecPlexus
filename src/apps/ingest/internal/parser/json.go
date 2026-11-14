package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxJSONSize 是单个 JSON 消息的解码上限，与 spec 的 HTTP 通道上限一致。
// 大小限制属于通道职责，这里再设一道防线：流式解码挡不住单个巨型对象。
const MaxJSONSize = 10 << 20 // 10 MiB。

// jsonParser 展开 JSON 对象的字段树，嵌套对象用 dot-path 表示。
type jsonParser struct{}

func newJSON(Spec) (Parser, error) { return jsonParser{}, nil }

// Parse 解析单个 JSON 对象。数组与 NDJSON 由 Split 拆分后逐条进入这里，
// 因此解析器只需处理一个值，复杂度留在拆分这一步。
func (jsonParser) Parse(raw []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: empty JSON message", ErrMalformed)
	}
	if len(trimmed) > MaxJSONSize {
		return nil, fmt.Errorf("%w: JSON message exceeds %d bytes", ErrMalformed, MaxJSONSize)
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("%w: JSON message must be a single object; use Split for arrays or NDJSON", ErrMalformed)
	}

	return decodeObject(trimmed)
}

// decodeObject 解码一个对象并摊平；要求输入在对象结束后没有剩余内容。
// 数字用 json.Number 保留原文：encoding/json 默认的 float64 会把超出
// 2^53 的整数截断，而 19 位的设备与事件标识在实际日志里很常见。
func decodeObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}

	fields := make(map[string]any, len(decoded))
	flatten("", decoded, fields)

	// 零字段与其它模式保持同一契约：空对象不携带任何数据，
	// 让它"成功"会产出只有常量的空事件，污染事件流。
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: JSON object has no fields", ErrMalformed)
	}

	return fields, nil
}

// ensureEOF 确认解码器已读到输入末尾；尾随垃圾按格式不符拒绝，
// 否则 `{"a":1} junk` 这类报文会被静默接受半条。
func ensureEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: unexpected content after the JSON value", ErrMalformed)
	}

	return nil
}

// flatten 把嵌套对象摊平为 dot-path 键。数组保持原值，
// 因为规则无法对未知长度的数组逐个写映射；需要展开时由规则另配。
func flatten(prefix string, source map[string]any, target map[string]any) {
	for key, value := range source {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		if nested, ok := value.(map[string]any); ok {
			flatten(path, nested, target)
			continue
		}

		target[path] = value
	}
}

// jsonArray 是 JSON 数组的首字节；其余形态走流式解码。
const jsonArray = '['

// Split 把 JSON 报文拆成独立消息。三种形态在实际设备上都存在，
// 且无法只按首字节区分：以 { 开头的既可能是单对象，也可能是 NDJSON 的第一行。
// 因此统一用流式解码器逐个取值，直到输入结束——一个对象得一条，NDJSON 得 N 条。
// 用流式解码而不是整体 Unmarshal：大报文不会在内存里翻倍。
func splitJSON(raw []byte) ([][]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: empty JSON payload", ErrMalformed)
	}

	if trimmed[0] == jsonArray {
		return splitJSONArray(trimmed)
	}

	return splitJSONStream(trimmed)
}

// splitJSONStream 从输入里逐个解码顶层值，供单对象与 NDJSON 共用。
func splitJSONStream(raw []byte) ([][]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	messages := make([][]byte, 0, 8)
	for {
		var value json.RawMessage
		err := decoder.Decode(&value)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}

		messages = append(messages, value)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: empty JSON payload", ErrMalformed)
	}

	return messages, nil
}

// splitJSONArray 逐元素解码一个 JSON 数组；闭合括号之后不允许再有内容。
func splitJSONArray(raw []byte) ([][]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return nil, fmt.Errorf("%w: expected a JSON array", ErrMalformed)
	}

	messages := make([][]byte, 0, 8)
	for decoder.More() {
		var element json.RawMessage
		if err = decoder.Decode(&element); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}

		messages = append(messages, element)
	}

	// 消费闭合的 ]，再确认后面没有尾随内容。
	if token, err = decoder.Token(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != ']' {
		return nil, fmt.Errorf("%w: unclosed JSON array", ErrMalformed)
	}
	if err = ensureEOF(decoder); err != nil {
		return nil, err
	}

	return messages, nil
}
