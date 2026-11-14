package parser

import (
	"bytes"
	"fmt"
	"regexp"
)

// regexParser 用命名捕获组从非结构化文本里取字段。
type regexParser struct{ expression *regexp.Regexp }

func newRegex(spec Spec) (Parser, error) {
	if spec.Pattern == "" {
		return nil, fmt.Errorf("%w: regex pattern is required", ErrInvalidSpec)
	}

	// 预编译一次，运行期不再编译。Go 的 regexp 是 RE2 实现，
	// 匹配时间与输入长度线性相关，没有灾难性回溯，恶意样本拖不垮 worker。
	expression, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}

	if !hasNamedGroup(expression) {
		return nil, fmt.Errorf("%w: regex needs at least one (?P<name>...) group", ErrInvalidSpec)
	}

	return regexParser{expression: expression}, nil
}

// hasNamedGroup 检查表达式是否至少有一个命名捕获组；
// 没有命名组的规则解析出的字段没有名字，等于没有规则。
func hasNamedGroup(expression *regexp.Regexp) bool {
	for _, name := range expression.SubexpNames() {
		if name != "" {
			return true
		}
	}

	return false
}

// Parse 匹配整条消息并按命名组产出字段；未匹配到的组不进结果，
// 这样可选字段不会以空字符串污染映射。
func (p regexParser) Parse(raw []byte) (map[string]any, error) {
	// 与其它模式保持一致：纯空白的输入按格式不符拒绝，
	// 不因模式恰好能匹配空白而产出一组空白字段。
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w: empty message", ErrMalformed)
	}

	matches := p.expression.FindSubmatch(raw)
	if matches == nil {
		return nil, fmt.Errorf("%w: regex did not match", ErrMalformed)
	}

	names := p.expression.SubexpNames()
	fields := make(map[string]any, len(names))
	for index, name := range names {
		if name == "" || index >= len(matches) || matches[index] == nil {
			continue
		}

		fields[name] = string(matches[index])
	}

	return fields, nil
}
