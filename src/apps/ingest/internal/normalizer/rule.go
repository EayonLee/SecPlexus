// Package normalizer 把线格式解析出的扁平字段投影成 OCSF 事件。
//
// 它只认识 OCSF：字段映射、severity 值表、时间解析与常量都不是线格式知识。
// 解析职责在 parser，映射职责在这里，两边独立演进。
//
// 一条规则由样本守护：Compile 会用规则自带的多条真实样本跑一遍，
// 任何一条解析或映射失败都拒绝加载——规则质量不靠人眼。
//
// 数组下标（x[0]）暂不支持：没有规则需要逐元素映射，整值写入已足够；
// 真有需要时再按需扩展，避免为想象中的用法付维护成本。
package normalizer

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

const (
	// SeverityUnknown 到 SeverityFatal 是 OCSF 的 severity_id 值域。
	SeverityUnknown       = 0
	SeverityInformational = 1
	SeverityLow           = 2
	SeverityMedium        = 3
	SeverityHigh          = 4
	SeverityCritical      = 5
	SeverityFatal         = 6
)

var (
	// ErrInvalidRule 表示规则定义不完整或超出值域；加载即拒绝。
	ErrInvalidRule = errors.New("invalid parse rule")
	// ErrSampleFailed 表示规则自带的样本没能通过解析或映射。
	ErrSampleFailed = errors.New("rule sample failed")
)

// SeveritySpec 描述如何把厂商的级别字段折算成 OCSF severity_id。
type SeveritySpec struct {
	Field  string         // 级别字段名；空表示规则不声明级别。
	Values map[string]int // 归一化后的原值到 severity_id 的映射。
}

// TimeSpec 描述事件时间字段与可接受的格式。
// 多个格式按顺序尝试，第一个成功的生效。
type TimeSpec struct {
	Field    string   // 时间字段名；空表示事件时间使用接收时间。
	Formats  []string // Go 时间布局，或 unix、unix_ms 两个关键字。
	Location string   // 设备时区的 IANA 名称（如 Asia/Shanghai）；空值按 UTC。
	// 无时区的时间串按设备时区解析。国产设备普遍发本地时间，
	// 按 UTC 解析会让整条时间线偏移数小时，破坏关联分析的时序窗口。
	// 对 unix/unix_ms 无效——数字时间戳与时区无关。
}

// Rule 是一条解析规则的完整定义。
type Rule struct {
	ID          string            // 规则标识，在同一模块内唯一。
	Name        string            // 展示名称。
	Vendor      string            // 厂商。
	Product     string            // 产品或设备型号。
	Description string            // 规则说明。
	Parse       parser.Spec       // 第 1 层：线格式解析参数。
	FieldMap    map[string]string // 第 1 层字段名到 OCSF dot-path 的映射。
	Severity    SeveritySpec      // 级别折算规则。
	Time        TimeSpec          // 事件时间解析规则。
	Constants   map[string]any    // 最后写入的固定值，覆盖映射结果。
	Samples     []string          // 真实样本，编译时逐条校验。
}

// maxSeverityID 是 OCSF severity_id 的上界。
const maxSeverityID = SeverityFatal

// Compile 校验规则并绑定解析器与预编译的投影计划。
// 返回的引擎不可变，可被多个 worker 并发调用。
// 校验分三层：规则结构（含路径合法性）、解析器参数、样本实跑。
func Compile(rule Rule) (*Engine, error) {
	if err := rule.validate(); err != nil {
		return nil, err
	}

	instance, err := parser.New(rule.Parse)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRule, err)
	}

	location := time.UTC
	if rule.Time.Location != "" {
		parsed, err := time.LoadLocation(rule.Time.Location)
		if err != nil {
			return nil, fmt.Errorf("%w: time.location %q: %w", ErrInvalidRule, rule.Time.Location, err)
		}

		location = parsed
	}

	// 值表键归一化一次：查询侧对事件值做小写与去空白，
	// 表键不做同样处理的话，"HIGH" 永远匹配不到 "high"。
	severityValues := make(map[string]int, len(rule.Severity.Values))
	for value, id := range rule.Severity.Values {
		severityValues[normalizeKey(value)] = id
	}

	engine := &Engine{
		rule:           rule,
		parser:         instance,
		projections:    compileFieldMap(rule.FieldMap),
		constants:      compileConstants(rule.Constants),
		consumed:       consumedSources(rule),
		location:       location,
		severityValues: severityValues,
	}

	for index, sample := range rule.Samples {
		// 样本校验用固定接收时间：样本验证的是解析与映射，不是时间回退。
		if _, err = engine.Apply([]byte(sample), time.Unix(0, 0).UTC()); err != nil {
			return nil, fmt.Errorf("%w: sample %d: %w", ErrSampleFailed, index+1, err)
		}
	}

	return engine, nil
}

// compileFieldMap 把映射表编译成一串 (源字段, 路径段) 计划。
// 逐事件再跑 strings.Split 会把规则里已经确定的路径重复解析数百万次。
func compileFieldMap(fieldMap map[string]string) []projection {
	projections := make([]projection, 0, len(fieldMap))
	for source, target := range fieldMap {
		projections = append(projections, projection{source: source, segments: splitPath(target)})
	}

	return projections
}

// compileConstants 把常量表编译成同样的计划，值单独保存。
func compileConstants(constants map[string]any) []constant {
	compiled := make([]constant, 0, len(constants))
	for target, value := range constants {
		compiled = append(compiled, constant{segments: splitPath(target), value: value})
	}

	return compiled
}

// consumedSources 汇总规则自身消费掉的源字段。
// unmapped 的意义是"规则没有用到的字段"，级别与时间字段已经用过，不该再报。
func consumedSources(rule Rule) map[string]struct{} {
	consumed := make(map[string]struct{}, len(rule.FieldMap)+2)
	for source := range rule.FieldMap {
		consumed[source] = struct{}{}
	}
	if rule.Severity.Field != "" {
		consumed[rule.Severity.Field] = struct{}{}
	}
	if rule.Time.Field != "" {
		consumed[rule.Time.Field] = struct{}{}
	}

	return consumed
}

// validate 检查规则结构的必填项与值域，错误定位到字段。
func (r Rule) validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidRule)
	}

	targets := make(map[string]string, len(r.FieldMap))
	fieldTargets := make(map[string]struct{}, len(r.FieldMap))
	for source, target := range r.FieldMap {
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("%w: field_map has an empty source", ErrInvalidRule)
		}

		segments, err := validatePath(target)
		if err != nil {
			return fmt.Errorf("%w: field_map %q: %w", ErrInvalidRule, source, err)
		}

		key := strings.Join(segments, ".")
		if existing, ok := targets[key]; ok {
			return fmt.Errorf("%w: field_map sources %q and %q both write %q", ErrInvalidRule, existing, source, key)
		}

		// 引擎拥有这两个 OCSF 键：级别与时间由 severity/time 规则声明。
		// 直接映射会被引擎的恒写保障静默覆盖——在加载期拒绝并指路，
		// 不留一个"看起来能配、运行必丢值"的写法。
		if key == "severity_id" || key == "time" {
			return fmt.Errorf("%w: field_map %q writes the engine-owned key %q; declare severity or time rules instead", ErrInvalidRule, source, key)
		}

		targets[key] = source
		fieldTargets[key] = struct{}{}
	}

	// 前缀冲突会让同一棵树被两种类型写入：x.y 与 x.y.z 只有一个能存在。
	for key := range targets {
		for other := range targets {
			if key != other && strings.HasPrefix(other, key+".") {
				return fmt.Errorf("%w: field_map target %q is a prefix of %q", ErrInvalidRule, key, other)
			}
		}
	}

	for _, value := range r.Severity.Values {
		if value < SeverityUnknown || value > maxSeverityID {
			return fmt.Errorf("%w: severity maps to %d, outside 0-%d", ErrInvalidRule, value, maxSeverityID)
		}
	}

	// 值表键在匹配前会归一化（小写、去空白）；两个键归一后相同会让映射不确定，
	// 加载期拒绝，而不是让 map 迭代顺序决定谁赢。
	normalized := make(map[string]struct{}, len(r.Severity.Values))
	for value := range r.Severity.Values {
		key := normalizeKey(value)
		if _, exists := normalized[key]; exists {
			return fmt.Errorf("%w: severity values collide on %q after normalization", ErrInvalidRule, key)
		}

		normalized[key] = struct{}{}
	}

	if r.Severity.Field != "" && len(r.Severity.Values) == 0 {
		return fmt.Errorf("%w: severity field needs a value table", ErrInvalidRule)
	}

	if r.Time.Field != "" && len(r.Time.Formats) == 0 {
		return fmt.Errorf("%w: time field needs at least one format", ErrInvalidRule)
	}

	constantTargets := make(map[string]struct{}, len(r.Constants))
	for target := range r.Constants {
		segments, err := validatePath(target)
		if err != nil {
			return fmt.Errorf("%w: constant %q: %w", ErrInvalidRule, target, err)
		}

		constantTargets[strings.Join(segments, ".")] = struct{}{}
	}

	// class_uid 是下游按类路由的唯一依据，缺失或空值的事件是伪 OCSF，直接拒绝。
	// 缺键与显式 nil 都取到零值 nil，一次判断覆盖两种情况。
	if r.Constants["class_uid"] == nil {
		return fmt.Errorf("%w: constants must declare a non-nil class_uid", ErrInvalidRule)
	}

	// 常量目标与映射目标互为前缀时，树形写入会覆盖另一方的标量（或反之），
	// 被覆盖的一方静默丢失。完全相同的目标是刻意设计：常量最后盖章。
	for constantKey := range constantTargets {
		for fieldKey := range fieldTargets {
			if constantKey != fieldKey && (strings.HasPrefix(constantKey, fieldKey+".") || strings.HasPrefix(fieldKey, constantKey+".")) {
				return fmt.Errorf("%w: constant %q and field_map target %q conflict on the same tree", ErrInvalidRule, constantKey, fieldKey)
			}
		}
	}

	return nil
}

// validatePath 校验 dot-path 并返回切分后的路径段。
// 空段（x..y、.x、x.）会创建没有名字的中间键，既无法映射也无法排查，必须拒绝。
func validatePath(path string) ([]string, error) {
	if strings.TrimSpace(path) != path || path == "" {
		return nil, errors.New("path must be a non-empty dot-path without surrounding spaces")
	}

	segments := strings.Split(path, ".")
	for _, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("path %q has an empty segment", path)
		}
	}

	return segments, nil
}

// splitPath 把已验证的路径切成段；只在 Compile 阶段调用。
func splitPath(path string) []string { return strings.Split(path, ".") }
