// Package parser 把一条原始消息的字节解析为扁平字段表。
//
// 这一层只认识线格式（JSON、CEF、键值对、正则、分隔符），不含任何 OCSF 知识；
// 字段投影到 OCSF 由 normalizer 完成。两层分开是刻意的：
// 新增线格式不影响规则映射，新增映射规则不影响解析实现。
package parser

import (
	"errors"
	"fmt"
	"slices"
)

// 解析模式。取值与规则 YAML 的 parser 字段一一对应。
const (
	ModeJSON      = "json"      // 结构化 JSON/NDJSON。
	ModeCEF       = "cef"       // ArcSight 通用事件格式。
	ModeKV        = "kv"        // key=value，分隔符可配。
	ModeRegex     = "regex"     // 命名捕获组。
	ModeDelimiter = "delimiter" // 按分隔符切列。
	// ModeLines 不是解析模式，而是拆分策略：一行一条。
	// 独立常量替代空字符串，避免调用方传 "" 这种看不出意图的魔法值。
	ModeLines = "lines"
)

// modes 按常用程度排列；新增模式在此登记。
var modes = []string{ModeJSON, ModeCEF, ModeKV, ModeRegex, ModeDelimiter}

// Modes 返回已实现解析模式的副本，按常用程度排列。
// 返回副本：调用方修改切片不能影响注册表的一致性。
func Modes() []string { return slices.Clone(modes) }

var (
	// ErrUnknownMode 表示规则声明了未实现的解析模式。
	ErrUnknownMode = errors.New("unknown parse mode")
	// ErrMalformed 表示消息与规则声明的线格式不符。
	ErrMalformed = errors.New("malformed message")
	// ErrInvalidSpec 表示规则参数不完整或不可用；构造期即拒绝，不留到运行期。
	ErrInvalidSpec = errors.New("invalid parse spec")
)

// Parser 解析一条消息。实现必须在构造后并发安全，且不再做一次性准备工作。
type Parser interface {
	Parse(raw []byte) (map[string]any, error)
}

// KVSpec 描述键值对的三个分隔符。
// 只用一个状态机覆盖 logfmt、CEF 扩展区与自定义键值——通用库无法适配任意分隔符组合。
type KVSpec struct {
	Delimiter string // 键与值之间，默认 "="。
	Entry     string // 条目之间，默认 " "。
	Quote     string // 包围值时可含分隔符；空值使用默认双引号。
}

// DelimiterSpec 描述按列切分的参数。
type DelimiterSpec struct {
	Separator string   // 列分隔符，必须是单个字符，默认 ","。
	Columns   []string // 列名，按位置与列一一对应。
}

// Spec 是解析规则中线格式相关的那部分参数。
type Spec struct {
	Mode      string        // 解析模式，取值见 Mode* 常量。
	Pattern   string        // ModeRegex 的表达式，使用 Go 命名捕获组语法 (?P<name>...)。
	KV        KVSpec        // ModeKV 的分隔符配置。
	Delimiter DelimiterSpec // ModeDelimiter 的分隔符与列名。
}

// Factory 按规则参数构造解析器。构造期完成正则预编译等一次性工作，
// 运行期 Parse 不再分配解析器、不再编译表达式。
type Factory func(spec Spec) (Parser, error)

// registry 是模式到构造函数的唯一登记处；新增模式只需在此加一行。
var registry = map[string]Factory{
	ModeJSON:      newJSON,
	ModeCEF:       newCEF,
	ModeKV:        newKV,
	ModeRegex:     newRegex,
	ModeDelimiter: newDelimiter,
}

// New 校验参数并构造解析器。返回的解析器可直接并发复用。
// 未知模式、缺失参数与非法表达式都在这里拒绝，调用方不必重复校验。
func New(spec Spec) (Parser, error) {
	factory, ok := registry[spec.Mode]
	if !ok {
		return nil, fmt.Errorf("%w: %q (supported: %v)", ErrUnknownMode, spec.Mode, modes)
	}

	return factory(spec)
}

// ValidMode 报告模式是否已实现；配置与规则加载用它给出字段级错误。
func ValidMode(mode string) bool {
	return slices.Contains(modes, mode)
}
