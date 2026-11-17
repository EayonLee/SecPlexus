package parser

import (
	"bytes"
	"fmt"
	"regexp"
)

// Multiline 描述如何把连续多行合成一条消息。
//
// 现实里最常见的两类：堆栈续行以空白开头，日志续行有固定前缀。
// 两种都能用"起始行"表达，因此这里只保留起始模式一种语义，
// 避免出现"起始 + 续行"两套模式互相冲突的组合。
type Multiline struct {
	// StartPattern 匹配**新消息的第一行**；不匹配的行并入上一条。
	StartPattern string
	// MaxLines 限制单条消息的最大行数，防止畸形输入把内存吃光。
	MaxLines int
	// MaxBytes 限制单条消息的最大字节数。
	MaxBytes int
}

// 多行合并的默认上限：允许合并，但必须有界。
// 无界合并时，一条畸形输入就能把进程内存吃光。
const (
	DefaultMultilineLines = 500
	DefaultMultilineBytes = 256 << 10
)

// compiledMultiline 是预编译后的合并器：构造期编译正则，运行期不再解析。
type compiledMultiline struct {
	start    *regexp.Regexp
	maxLines int
	maxBytes int
}

// compileMultiline 在构造期校验并预编译。
func compileMultiline(spec Multiline) (*compiledMultiline, error) {
	if spec.StartPattern == "" {
		return nil, fmt.Errorf("%w: multiline start pattern is required", ErrInvalidSpec)
	}

	start, err := regexp.Compile(spec.StartPattern)
	if err != nil {
		return nil, fmt.Errorf("%w: multiline start pattern is invalid: %w", ErrInvalidSpec, err)
	}

	maxLines := spec.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultMultilineLines
	}

	maxBytes := spec.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMultilineBytes
	}

	return &compiledMultiline{start: start, maxLines: maxLines, maxBytes: maxBytes}, nil
}

// merge 把按行切好的消息合并成逻辑消息。
//
// 第一行无论是否匹配起始模式都会开启一条新消息：数据的开头天然是一条消息的起点，
// 强行丢弃会静默丢数据。
func (m *compiledMultiline) merge(lines [][]byte) [][]byte {
	merged := make([][]byte, 0, len(lines))
	var current []byte
	linesInCurrent := 0

	flush := func() {
		if len(current) > 0 {
			merged = append(merged, current)
		}

		current = nil
		linesInCurrent = 0
	}

	for _, line := range lines {
		startsNew := len(current) == 0 || m.start.Match(line)

		if startsNew {
			flush()
			current = append([]byte(nil), line...)
			linesInCurrent = 1

			continue
		}

		// 续行：追加换行与原样行，保持堆栈可读。
		current = append(append(current, '\n'), line...)
		linesInCurrent++

		if linesInCurrent >= m.maxLines || len(current) >= m.maxBytes {
			flush()
		}
	}

	flush()

	return merged
}

// NewMultiline 构造一个多行合并器，返回的函数把整段文本切成逻辑消息。
//
// 返回函数而不是暴露内部类型：调用方只需要"文本进、消息出"，
// 预编译结果不外泄，避免有人绕过构造期校验直接拼装。
func NewMultiline(spec Multiline) (func(raw []byte) ([][]byte, error), error) {
	compiled, err := compileMultiline(spec)
	if err != nil {
		return nil, err
	}

	return func(raw []byte) ([][]byte, error) {
		lines := splitRawLines(raw)
		if len(lines) == 0 {
			return nil, fmt.Errorf("%w: payload contains no messages", ErrMalformed)
		}

		return compiled.merge(lines), nil
	}, nil
}

// splitRawLines 按换行切分并去掉空行；与 Split 的行模式保持同一约定。
func splitRawLines(raw []byte) [][]byte {
	lines := make([][]byte, 0, 8)
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}

		lines = append(lines, trimmed)
	}

	return lines
}
