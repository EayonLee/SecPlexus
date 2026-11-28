package normalizer

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

func (e *Compiled) bindContract() error {
	catalog, err := events.LoadVersion(e.mapping.ContractVersion)
	if err != nil {
		return err
	}
	uid, integral := integralValue(e.mapping.Constants["class_uid"])
	// 事件类型 ID 在不同位宽的平台上使用同一取值范围。
	if !integral || uid < 0 || uid > math.MaxInt32 {
		return fmt.Errorf("%w: unsupported event class", ErrInvalidRule)
	}
	class, supported := catalog.Class(int(uid))
	if !supported {
		return fmt.Errorf("%w: unsupported event class", ErrInvalidRule)
	}
	fields := make(map[string]*events.Field, len(class.Fields))
	for i := range class.Fields {
		field := &class.Fields[i]
		fields[field.Path] = field
	}
	for i := range e.projections {
		plan := &e.projections[i]
		path := strings.Join(plan.segments, ".")
		field := fields[path]
		if field == nil || field.Owner != "mapping" {
			return fmt.Errorf("%w: mapping target %q is unavailable in class %d", ErrInvalidRule, path, uid)
		}
		plan.field = field
	}
	for i := range e.constants {
		plan := &e.constants[i]
		path := strings.Join(plan.segments, ".")
		field := fields[path]
		if field == nil || (field.Owner != "mapping" && path != "class_uid") {
			return fmt.Errorf("%w: constant target %q is unavailable in class %d", ErrInvalidRule, path, uid)
		}
		converted, ok := convertField(*field, plan.value)
		if !ok {
			return fmt.Errorf("%w: constant %q violates the field type or allowed values", ErrInvalidRule, path)
		}
		plan.value = converted
	}
	e.usesCatalog = true
	e.classUID = class.UID
	e.constraints = class.Constraints
	return nil
}

func incompleteClass(fields map[string]any, constraints []events.Constraint) string {
	for _, check := range constraints {
		if check.When != "" && !hasEventValue(eventValue(fields, check.When)) {
			continue
		}
		count := 0
		for _, path := range check.Paths {
			if hasEventValue(eventValue(fields, path)) {
				count++
			}
		}
		if count == 0 || (check.Mode == "just_one" && count != 1) {
			return strings.Join(check.Paths, "|")
		}
	}
	for _, pair := range [][2]string{{"severity_id", "severity"}, {"activity_id", "activity_name"}, {"status_id", "status"}} {
		if code, _ := integralValue(fields[pair[0]]); code == 99 {
			text, _ := fields[pair[1]].(string)
			if strings.TrimSpace(text) == "" {
				return pair[1]
			}
		}
	}
	return ""
}

func eventValue(fields map[string]any, path string) any {
	var value any = fields
	for part := range strings.SplitSeq(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[part]
	}
	return value
}

func hasEventValue(value any) bool {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case []any:
		return len(value) != 0
	case map[string]any:
		return len(value) != 0
	default:
		return value != nil
	}
}

func hasIdentity(object any, keys ...string) bool {
	fields, _ := object.(map[string]any)
	return slices.ContainsFunc(keys, func(key string) bool {
		value, _ := fields[key].(string)
		return strings.TrimSpace(value) != ""
	})
}

func convertField(field events.Field, value any) (any, bool) {
	if field.Array {
		items, ok := value.([]any)
		if !ok || len(items) > 64 {
			return nil, false
		}
		converted := make([]any, len(items))
		field.Array = false
		for i, item := range items {
			entry, ok := convertField(field, item)
			if !ok {
				return nil, false
			}
			converted[i] = entry
		}
		return converted, true
	}
	switch field.JSONType {
	case "string":
		raw, ok := value.(string)
		if !ok {
			return nil, false
		}
		if field.LogicalType == "ip_t" {
			addr, err := netip.ParseAddr(raw)
			if err != nil || addr.Zone() != "" {
				return nil, false
			}
			raw = addr.Unmap().String()
		}
		return raw, true
	case "boolean":
		boolean, ok := value.(bool)
		return boolean, ok
	case "object":
		object, ok := value.(map[string]any)
		if !ok || len(field.ObjectFields) == 0 {
			return nil, false
		}
		checked := make(map[string]any, len(field.ObjectFields))
		for _, path := range field.ObjectFields {
			if item, ok := eventValue(object, path).(string); ok && strings.TrimSpace(item) != "" && len(item) <= 4096 {
				writePath(checked, strings.Split(path, "."), item)
			}
		}
		for _, path := range field.RequiredOutput {
			if !hasEventValue(eventValue(checked, path)) {
				return nil, false
			}
		}
		return checked, true
	case "integer":
		number, ok := integralValue(value)
		if !ok || (field.LogicalType == "port_t" && (number < 0 || number > 65535)) {
			return nil, false
		}
		if bounds := field.IntegerRange; bounds != nil && (number < bounds.Min || number > bounds.Max) {
			return nil, false
		}
		if (field.Path == "connection_info.protocol_num" && (number < 0 || number > 255)) ||
			(strings.HasPrefix(field.Path, "cumulative_traffic.") && number < 0) {
			return nil, false
		}
		if len(field.Enum) != 0 && !slices.ContainsFunc(field.Enum, func(entry events.EnumValue) bool {
			return int64(entry.Value) == number
		}) {
			return nil, false
		}
		return number, true
	default:
		return nil, false
	}
}

func integralValue(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		return int64(value), value >= math.MinInt64 && value < math.MaxInt64 && math.Trunc(value) == value
	case json.Number:
		result, err := value.Int64()
		return result, err == nil
	case string:
		result, err := strconv.ParseInt(value, 10, 64)
		return result, err == nil
	}
	return 0, false
}
