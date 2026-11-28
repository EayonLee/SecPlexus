// Package validation 定义 Ingest 管理写入的文本约束。
package validation

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Field 是规则请求中可定位的字段路径。
type Field string

const (
	Name        Field = "name"
	Vendor      Field = "vendor"
	Product     Field = "product"
	Description Field = "definition.description"
)

// Issue 标识业务字段校验错误，且不携带用户提交值。
type Issue struct {
	Field Field
	Code  string
	Limit int
}

func (i *Issue) Error() string { return "Invalid rule field: " + string(i.Field) + " (" + i.Code + ")" }

// RuleText 统一规则文本的空白、换行及长度检查。限值与管理表约束对应。
func RuleText(field Field, value string) (string, error) {
	limit := 120
	multiline := field == Description
	switch field {
	case Name:
		limit = 100
	case Description:
		limit = 1000
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	}
	for _, character := range value {
		if multiline && (character == '\n' || character == '\t') {
			continue
		}
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return "", &Issue{Field: field, Code: "invalid_characters"}
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", &Issue{Field: field, Code: "required"}
	}
	if utf8.RuneCountInString(value) > limit {
		return "", &Issue{Field: field, Code: "too_long", Limit: limit}
	}
	return value, nil
}
