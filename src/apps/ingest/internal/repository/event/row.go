package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

// Projector 从标准 Event 生成 ClickHouse 类型列，复用 DDL 的列映射。
type Projector struct {
	columns []projectedColumn
	classes map[uint64]map[string]map[uint64]bool
}

// NewProjector 检查投影并预编译已发布契约的字段枚举。
func NewProjector() (*Projector, error) {
	columns, current, err := loadProjection()
	if err != nil {
		return nil, err
	}

	p := &Projector{columns: columns, classes: make(map[uint64]map[string]map[uint64]bool)}
	for _, summary := range current.Classes() {
		class, _ := current.Class(summary.UID)
		enums := make(map[string]map[uint64]bool)
		for _, field := range class.Fields {
			if len(field.Enum) == 0 {
				continue
			}
			codes := make(map[uint64]bool, len(field.Enum))
			for _, item := range field.Enum {
				codes[uint64(item.Value)] = true
			}
			enums[field.Path] = codes
		}
		p.classes[uint64(summary.UID)] = enums
	}
	return p, nil
}

// Project 校验存储字段及事件分类，保留收到的原始标准 JSON。
func (p *Projector) Project(body []byte) (map[string]any, error) {
	var object map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&object); err != nil {
		return nil, fmt.Errorf("event body is not JSON")
	}
	if dec.Decode(new(json.RawMessage)) != io.EOF {
		return nil, fmt.Errorf("event body contains trailing data")
	}
	result := make(map[string]any, len(p.columns))
	for _, col := range p.columns {
		if col.Conversion == "event_json" {
			result[col.Name] = string(body)
			continue
		}
		value, ok := projectValue(object, col)
		if !ok {
			return nil, fmt.Errorf("event field %s has an invalid value", col.Path)
		}
		result[col.Name] = value
	}
	version, _ := result["contract_version"].(string)
	if version != events.CurrentVersion || result["ocsf_version"] != events.CurrentOCSFVersion {
		return nil, fmt.Errorf("event schema version is unsupported")
	}
	class, isClass := result["class_uid"].(uint64)
	category, isCategory := result["category_uid"].(uint64)
	activity, isActivity := result["activity_id"].(uint64)
	severity, isSeverity := result["severity_id"].(uint64)
	enums, ok := p.classes[class]
	if !isClass || !isCategory || !isActivity || !isSeverity || !ok || !enums["category_uid"][category] ||
		!enums["activity_id"][activity] || !enums["severity_id"][severity] || result["type_uid"] != class*100+activity {
		return nil, fmt.Errorf("event classification is invalid")
	}

	metadata, _ := object["metadata"].(map[string]any)
	spx, _ := metadata["spx"].(map[string]any)
	quality, _ := spx["parse.outcome"].(string)
	if (quality != "ok" && quality != "partial" && quality != "error") ||
		(quality == "error" && class != 0) {
		return nil, fmt.Errorf("event parse outcome or class is invalid")
	}
	result["parse_outcome"] = quality
	return result, nil
}

func projectValue(object map[string]any, col projectedColumn) (any, bool) {
	nullable := strings.Contains(col.Type, "Nullable(")
	var value any
	for i, segment := range col.segments {
		var present bool
		value, present = object[segment]
		if !present {
			return nil, nullable
		}
		if i+1 < len(col.segments) {
			object, present = value.(map[string]any)
			if !present {
				return nil, false
			}
		}
	}
	switch col.Conversion {
	case "uuid":
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		id, err := uuid.Parse(text)
		return id.String(), err == nil && id != uuid.Nil
	case "epoch_millis":
		number, ok := value.(json.Number)
		if !ok {
			return nil, false
		}
		millis, err := number.Int64()
		if err != nil {
			return nil, false
		}
		at := time.UnixMilli(millis).UTC()
		if at.Year() < 1900 || at.Year() > 2299 {
			return nil, false
		}
		return at.Format("2006-01-02 15:04:05.000"), true
	case "unsigned":
		number, ok := value.(json.Number)
		if !ok {
			return nil, false
		}
		bits, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(col.Type, "Nullable("), "UInt"), ")"))
		n, err := strconv.ParseUint(string(number), 10, bits)
		return n, err == nil
	case "string", "blank_to_null":
		text, ok := value.(string)
		if col.Conversion == "blank_to_null" && ok && text == "" {
			return nil, true
		}
		return text, ok
	case "ipv4_mapped_ipv6":
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		addr, err := netip.ParseAddr(text)
		if err != nil || addr.Zone() != "" {
			return nil, false
		}
		return netip.AddrFrom16(addr.As16()).String(), true
	}
	return nil, false
}
