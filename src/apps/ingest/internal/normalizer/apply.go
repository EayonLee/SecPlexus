package normalizer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// projection 是一条预编译的字段映射：从源字段取到的值写到目标路径。
// 路径段在 Compile 阶段切好，逐事件不再解析点号。
type projection struct {
	source   string
	segments []string
}

// constant 是一条预编译的常量写入。
type constant struct {
	segments []string
	value    any
}

// redactedValue 替换 unmapped 里命中敏感名单的字段值。
const redactedValue = "[REDACTED]"

// sensitiveFragments 是敏感字段名的判定片段。
// 设备日志里带口令与令牌很常见（配置错误的 curl、调试输出），
// unmapped 原样承载它们会把凭证带进事件存储与查询界面。
var sensitiveFragments = []string{
	"password", "passwd", "pwd", "secret", "token", "api_key",
	"apikey", "authorization", "cookie", "credential", "private_key", "session",
}

// keyNormalizer 把字段名里的分隔符统一成下划线，让 password、user-password 与
// Password 命中同一名单。包级构造一次：它在 5w EPS 热路径上按字段反复调用，
// 放在函数体里会变成每字段一次分配。
var keyNormalizer = strings.NewReplacer("-", "_", " ", "_", ".", "_")

// isSensitiveKey 判定源字段名是否命中敏感名单。
func isSensitiveKey(key string) bool {
	normalized := keyNormalizer.Replace(strings.ToLower(key))

	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}

	return false
}

// Event 是一条归一化后的结果。
type Event struct {
	Fields   map[string]any // 展开为嵌套结构的 OCSF 事件。
	Unmapped map[string]any // 规则未使用的源字段；没有时保持 nil，省一次分配。
	Warnings []string       // 非致命问题：级别或时间无法折算等。
}

// Engine 把一条规则与它的解析器、投影计划绑定在一起。
// 构造后不再修改内部状态，可被多个 worker 并发调用。
type Engine struct {
	rule           Rule
	parser         parser.Parser
	projections    []projection
	constants      []constant
	consumed       map[string]struct{}
	location       *time.Location // 设备时区；解析无时区的时间串时使用。
	severityValues map[string]int // 归一化键（小写、去空白）后的级别值表。
}

// Rule 返回绑定到引擎的规则，供指标与诊断使用。
func (e *Engine) Rule() Rule { return e.rule }

// Apply 解析一条消息并投影成 OCSF 事件。
//
// receivedAt 是通道收到这条消息的时刻，事件时间解析失败时作为回退，
// 因此每条事件都有时间——OCSF 事件缺时间会让保留期与窗口聚合全部失效。
//
// 解析失败返回错误，调用方记为 parse_status=error；
// 级别或时间折算不了只记 warning，事件照常产出——
// 不因一个字段不可信就丢掉整条日志。
func (e *Engine) Apply(raw []byte, receivedAt time.Time) (Event, error) {
	fields, err := e.parser.Parse(raw)
	if err != nil {
		return Event{}, err
	}

	return e.ApplyFields(fields, receivedAt), nil
}

// ApplyFields 只做第 2 层：把已解析的扁平字段投影成 OCSF 事件。
// 第 1 层由通道自己做时用它——syslog 通道先用 go-syslog 剥信封，
// 再把字段交给规则映射，两层职责仍然不混。
func (e *Engine) ApplyFields(fields map[string]any, receivedAt time.Time) Event {
	event := Event{Fields: make(map[string]any, len(e.projections)+len(e.constants)+4)}

	for index := range e.projections {
		plan := &e.projections[index]
		value, ok := fields[plan.source]
		if !ok {
			continue
		}

		writePath(event.Fields, plan.segments, value)
	}

	e.collectUnmapped(&event, fields)
	e.stampSeverity(&event, fields)
	e.stampTime(&event, fields, receivedAt)

	// 常量最后写入：规则声明的 class_uid、kind 等不该被设备字段意外覆盖。
	for index := range e.constants {
		plan := &e.constants[index]
		writePath(event.Fields, plan.segments, plan.value)
	}

	return event
}

// collectUnmapped 记录规则没有用到的源字段；一个都没有时不分配 map。
// 命中敏感名单的字段保留键名、替换值：
// 规则作者仍能看到"这里有个字段值得映射"，凭证却不进入事件存储。
func (e *Engine) collectUnmapped(event *Event, fields map[string]any) {
	for source, value := range fields {
		if _, ok := e.consumed[source]; ok {
			continue
		}

		if event.Unmapped == nil {
			event.Unmapped = make(map[string]any, 4)
		}

		if isSensitiveKey(source) {
			event.Unmapped[source] = redactedValue
			continue
		}

		event.Unmapped[source] = value
	}
}

// stampSeverity 把级别折算成 severity_id。
// OCSF 事件必须带级别：规则没声明、字段缺失或值表未命中都落到 Unknown（0）并记 warning，
// 绝不产出缺级别的伪 OCSF 事件。
func (e *Engine) stampSeverity(event *Event, fields map[string]any) {
	if e.rule.Severity.Field == "" {
		event.Fields["severity_id"] = SeverityUnknown
		return
	}

	raw, ok := fields[e.rule.Severity.Field]
	if !ok {
		event.Warnings = append(event.Warnings, "severity field missing: "+e.rule.Severity.Field)
		event.Fields["severity_id"] = SeverityUnknown
		return
	}

	key := normalizeKey(textValue(raw))
	id, ok := e.severityValues[key]
	if !ok {
		event.Warnings = append(event.Warnings, "severity value unmapped: "+key)
		id = SeverityUnknown
	}

	event.Fields["severity_id"] = id
}

// stampTime 写事件时间：规则声明的时间字段优先，解析失败或未声明时回退接收时间。
// 两种来源都写成 OCSF 的 time（epoch 毫秒），事件永远带时间。
func (e *Engine) stampTime(event *Event, fields map[string]any, receivedAt time.Time) {
	if e.rule.Time.Field != "" {
		raw, ok := fields[e.rule.Time.Field]
		if !ok {
			event.Warnings = append(event.Warnings, "time field missing: "+e.rule.Time.Field)
		} else if millis, err := parseTime(textValue(raw), e.rule.Time.Formats, e.location); err == nil {
			event.Fields["time"] = millis

			return
		} else {
			event.Warnings = append(event.Warnings, "time value unparsed: "+textValue(raw))
		}
	}

	event.Fields["time"] = receivedAt.UTC().UnixMilli()
}

// textValue 把字段值转成用于匹配与解析的文本。
//
// JSON 数字在 UseNumber 之下是 json.Number，原样取字面值；
// 兜底的 float64 用十进制最短形式，避免科学计数法毁掉时间戳与数字级别。
func textValue(value any) string {
	switch number := value.(type) {
	case json.Number:
		return string(number)
	case float64:
		return strconv.FormatFloat(number, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

// normalizeKey 归一化用于值表匹配的键：去空白并转小写，
// 让 "High"、" HIGH " 与 "high" 落到同一档。
func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// parseTime 依次尝试声明的格式；unix 与 unix_ms 是数字时间戳的关键字。
// 无时区的时间串按设备时区解析（ParseInLocation）。
func parseTime(value string, formats []string, location *time.Location) (int64, error) {
	trimmed := strings.TrimSpace(value)

	for _, format := range formats {
		switch format {
		case "unix":
			if seconds, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return seconds * 1000, nil
			}
		case "unix_ms":
			if millis, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return millis, nil
			}
		default:
			if parsed, err := time.ParseInLocation(format, trimmed, location); err == nil {
				return parsed.UTC().UnixMilli(), nil
			}
		}
	}

	return 0, fmt.Errorf("no declared format matched %q", trimmed)
}

// writePath 按预编译的路径段写入嵌套 map，沿途按需创建中间层。
func writePath(target map[string]any, segments []string, value any) {
	current := target
	for _, segment := range segments[:len(segments)-1] {
		nested, ok := current[segment].(map[string]any)
		if !ok {
			nested = make(map[string]any, 2)
			current[segment] = nested
		}

		current = nested
	}

	current[segments[len(segments)-1]] = value
}

// EventID 按接入点、接收序号与原始报文算出确定性 UUIDv5。
//
// 指纹必须包含"发生次"信息：暴力破解的第 1 次与第 50 次尝试、
// 同一条 ACL 拒绝的反复触发，字节完全相同但都是真实发生的独立事件——
// 只按内容去重会把真实告警静默吞掉，这是安全产品里不可观测的漏报。
//
// sequence 是接入点的接收序号（通道层维护，同一接入点内单调）。
// 同一接入点的重复投递（网络重试）因此得到不同 ID；
// 重试去重由查询层按 (接入点, 内容, 时间窗) 处理，不在 ID 里解决。
func EventID(connectorID string, sequence uint64, raw []byte) string {
	namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(connectorID))

	payload := make([]byte, 8+len(raw))
	binary.BigEndian.PutUint64(payload, sequence)
	copy(payload[8:], raw)

	return uuid.NewSHA1(namespace, payload).String()
}
