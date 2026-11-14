package parser

import (
	"bytes"
	"fmt"
)

// Split 把一次接收到的报文拆成一条条独立消息。
//
// 拆分是线格式的一部分而不是解析的一部分：JSON 报文可能是一个对象、
// 一个数组或 NDJSON，syslog 与 HTTP 纯文本则一行一条。把它们分开，
// Parse 就只需要处理单条消息，两边的复杂度都不会互相渗透。
func Split(mode string, raw []byte) ([][]byte, error) {
	if mode == ModeJSON {
		return splitJSON(raw)
	}

	// 其余模式按行拆分：一行一条是 syslog、CEF 与纯文本推送的通用约定。
	messages := make([][]byte, 0, bytes.Count(raw, []byte{'\n'})+1)
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}

		messages = append(messages, trimmed)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: payload contains no messages", ErrMalformed)
	}

	return messages, nil
}
