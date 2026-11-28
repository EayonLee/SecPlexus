package parser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// delimiterParser 按分隔符提取字段，字段名与位置一一对应。
type delimiterParser struct {
	separator  rune
	fieldNames []string
}

func newDelimiter(spec Spec) (Parser, error) {
	separator := spec.Delimiter.Separator
	if separator == "" {
		separator = ","
	}

	runes := []rune(separator)
	if len(runes) != 1 {
		return nil, fmt.Errorf("%w: delimiter separator must be a single character", ErrInvalidSpec)
	}

	return delimiterParser{separator: runes[0], fieldNames: spec.Delimiter.FieldNames}, nil
}

// Parse 按分隔符切一条记录。encoding/csv 处理引号包围与双引号转义，
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

		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	if len(p.fieldNames) == 0 {
		fields := make(map[string]any, len(record))
		for index, value := range record {
			fields["field_"+strconv.Itoa(index+1)] = value
		}
		return fields, nil
	}

	count := min(len(record), len(p.fieldNames))
	fields := make(map[string]any, count)
	for index, name := range p.fieldNames[:count] {
		fields[name] = record[index]
	}

	return fields, nil
}
