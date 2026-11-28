package parser

import (
	"bytes"
	"errors"
	"fmt"
)

// ErrBatchLimit 表示输入包含超过预算的逻辑消息。
var ErrBatchLimit = errors.New("batch exceeds limit")

// Split 把一次接收到的报文拆成一条条独立消息。
//
// 拆分是线格式的一部分而不是解析的一部分：JSON 报文可能是一个对象、
// 一个数组或 NDJSON，syslog 与 HTTP 纯文本则一行一条。把它们分开，
// Parse 就只需要处理单条消息，两边的复杂度都不会互相渗透。
func Split(mode string, raw []byte) ([][]byte, error) {
	return SplitLimited(mode, raw, 0)
}

// SplitLimited 在拆分时验证逻辑消息数量。零表示由调用方管理限制。
func SplitLimited(mode string, raw []byte, limit int) ([][]byte, error) {
	if mode == ModeJSON {
		return splitJSON(raw, limit)
	}

	// 其余模式按行拆分：一行一条是 syslog、CEF 与纯文本推送的通用约定。
	messages := make([][]byte, 0, 8)
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}

		if limit > 0 && len(messages) == limit {
			return nil, ErrBatchLimit
		}
		// 排队的单条消息独立持有字节，避免小切片延长整包的生命周期。
		messages = append(messages, bytes.Clone(trimmed))
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: payload contains no messages", ErrMalformed)
	}

	return messages, nil
}
