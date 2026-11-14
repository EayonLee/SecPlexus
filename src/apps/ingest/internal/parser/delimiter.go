package parser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// delimiterParser 按分隔符切列，列名与列位置一一对应。
type delimiterParser struct {
	separator rune
	columns   []string
}

func newDelimiter(spec Spec) (Parser, error) {
	if len(spec.Delimiter.Columns) == 0 {
		return nil, fmt.Errorf("%w: delimiter mode needs columns", ErrInvalidSpec)
	}

	separator := spec.Delimiter.Separator
	if separator == "" {
		separator = ","
	}

	runes := []rune(separator)
	if len(runes) != 1 {
		return nil, fmt.Errorf("%w: delimiter separator must be a single character", ErrInvalidSpec)
	}

	return delimiterParser{separator: runes[0], columns: spec.Delimiter.Columns}, nil
}

// Parse 用 encoding/csv 切一列行：它已经处理好引号包围与双引号转义，
// 自己按分隔符 split 会在值里出现分隔符时切错。
func (p delimiterParser) Parse(raw []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w: empty message", ErrMalformed)
	}

	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.Comma = p.separator
	// 样本日志很少有严格合法的 CSV 引号，宽松模式避免整条消息因引号不配对被丢弃。
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	record, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("%w: empty record", ErrMalformed)
		}

		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}

	fields := make(map[string]any, len(p.columns))
	for index, column := range p.columns {
		if index >= len(record) {
			break
		}

		fields[column] = record[index]
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: no columns matched the record", ErrMalformed)
	}

	return fields, nil
}
