package normalizer

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

const maxBranches = 20
const maxBranchFields = 128

var branchID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type matchCondition struct {
	Field  string   `json:"field"`
	Equals *string  `json:"equals,omitempty"`
	In     []string `json:"in,omitempty"`
}

type branchSpec struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Class int              `json:"class_uid"`
	Match []matchCondition `json:"match"`
	mappingFields
}

type mappingFields struct {
	FieldMap  map[string]string         `json:"field_map"`
	Constants map[string]any            `json:"constants"`
	Defaults  map[string]any            `json:"defaults,omitempty"`
	ValueMap  map[string]map[string]int `json:"value_map"`
	Time      TimeSpec                  `json:"time"`
	Severity  SeveritySpec              `json:"severity"`
}

type branch struct {
	id         string
	conditions map[string]map[string]bool
	mapping    *Compiled
}

func compileBranches(spec mappingSpec) (*Compiled, error) {
	if spec.ContractVersion != events.CurrentVersion || len(spec.Branches) > maxBranches {
		return nil, fmt.Errorf("%w: rule requires the current event contract and at most %d branches", ErrInvalidRule, maxBranches)
	}
	root, err := CompileExtraction(spec.Extraction)
	if err != nil {
		return nil, err
	}
	catalog, err := events.Load()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for _, summary := range catalog.Classes() {
		class, _ := catalog.Class(summary.UID)
		for _, field := range class.Fields {
			if field.Owner == "mapping" {
				known[field.Path] = true
			}
		}
	}
	sharedTargets := make(map[string]bool, len(spec.Shared.FieldMap))
	for source, target := range spec.Shared.FieldMap {
		if isSensitiveKey(source) || !known[target] {
			return nil, fmt.Errorf("%w: invalid shared mapping for %q", ErrInvalidRule, target)
		}
		if sharedTargets[target] {
			return nil, fmt.Errorf("%w: multiple shared mappings for %q", ErrInvalidRule, target)
		}
		sharedTargets[target] = true
	}
	for target := range spec.Shared.Constants {
		if !known[target] {
			return nil, fmt.Errorf("%w: invalid shared constant for %q", ErrInvalidRule, target)
		}
	}
	for target := range spec.Shared.ValueMap {
		if !sharedTargets[target] {
			return nil, fmt.Errorf("%w: missing shared mapping for %q", ErrInvalidRule, target)
		}
	}
	for target := range spec.Shared.Defaults {
		if !sharedTargets[target] {
			return nil, fmt.Errorf("%w: missing shared mapping for %q", ErrInvalidRule, target)
		}
	}
	if len(spec.Shared.FieldMap)+len(spec.Shared.Constants) > maxBranchFields ||
		isSensitiveKey(spec.Shared.Time.Field) || isSensitiveKey(spec.Shared.Severity.Field) {
		return nil, fmt.Errorf("%w: invalid shared fields", ErrInvalidRule)
	}
	ids := make(map[string]bool, len(spec.Branches))
	for _, item := range spec.Branches {
		if !branchID.MatchString(item.ID) || ids[item.ID] || strings.TrimSpace(item.Name) == "" || len(item.Name) > 160 || item.Class <= 0 || len(item.Match) > 12 || len(item.FieldMap) > maxBranchFields || len(item.Constants) > maxBranchFields {
			return nil, fmt.Errorf("%w: invalid branch identity, class or condition count", ErrInvalidRule)
		}
		ids[item.ID] = true
		checks := make(map[string]map[string]bool, len(item.Match))
		for _, match := range item.Match {
			if match.Field == "" || len(match.Field) > 128 || checks[match.Field] != nil ||
				(match.Equals == nil) == (len(match.In) == 0) || len(match.In) > 32 {
				return nil, fmt.Errorf("%w: invalid match in branch %s", ErrInvalidRule, item.ID)
			}
			values := match.In
			if match.Equals != nil {
				values = []string{*match.Equals}
			}
			set := make(map[string]bool, len(values))
			for _, value := range values {
				if len(value) > 256 || set[value] {
					return nil, fmt.Errorf("%w: invalid match value in branch %s", ErrInvalidRule, item.ID)
				}
				set[value] = true
			}
			checks[match.Field] = set
		}
		for _, previous := range root.branches {
			if covers(previous.conditions, checks) {
				return nil, fmt.Errorf("%w: branch %s is shadowed by %s", ErrInvalidRule, item.ID, previous.id)
			}
		}
		if coveredByEarlierBranches(root.branches, checks) {
			return nil, fmt.Errorf("%w: branch %s is shadowed by earlier branches", ErrInvalidRule, item.ID)
		}
		if isSensitiveKey(item.Time.Field) || isSensitiveKey(item.Severity.Field) {
			return nil, fmt.Errorf("%w: sensitive conversion field in branch %s", ErrInvalidRule, item.ID)
		}
		for source := range item.FieldMap {
			if isSensitiveKey(source) {
				return nil, fmt.Errorf("%w: sensitive mapping source in branch %s", ErrInvalidRule, item.ID)
			}
		}
		mapping, err := compileBranch(spec, item, catalog)
		if err != nil {
			return nil, err
		}
		root.branches = append(root.branches, branch{item.ID, checks, mapping})
	}
	base := branchSpec{ID: "base", Name: "Base Event"}
	if len(root.branches) > 0 {
		first := root.branches[0].mapping
		if !slices.ContainsFunc(root.branches[1:], func(branch branch) bool {
			other := branch.mapping.mapping.Time
			return first.mapping.Time.Field != other.Field || first.mapping.Time.Location != other.Location || !slices.Equal(first.mapping.Time.Formats, other.Formats)
		}) {
			base.Time = first.mapping.Time
		}
		if !slices.ContainsFunc(root.branches[1:], func(branch branch) bool {
			return first.mapping.Severity.Field != branch.mapping.mapping.Severity.Field ||
				!reflect.DeepEqual(first.mapping.Severity.Default, branch.mapping.mapping.Severity.Default) ||
				!maps.Equal(first.severityValues, branch.mapping.severityValues)
		}) {
			base.Severity = first.mapping.Severity
		}
	}
	root.fallback, err = compileBranch(spec, base, catalog)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func compileBranch(spec mappingSpec, item branchSpec, catalog *events.Catalog) (*Compiled, error) {
	class, exists := catalog.Class(item.Class)
	if !exists {
		return nil, fmt.Errorf("%w: unknown event class %d", ErrInvalidRule, item.Class)
	}
	fields := make(map[string]string, len(item.FieldMap)+len(spec.Shared.FieldMap))
	constants := make(map[string]any, len(item.Constants)+len(spec.Shared.Constants)+1)
	values := make(map[string]map[string]int, len(item.ValueMap)+len(spec.Shared.ValueMap))
	defaults := make(map[string]any, len(item.Defaults)+len(spec.Shared.Defaults))
	ignored := make(map[string]string)
	for source, target := range spec.Shared.FieldMap {
		if slices.ContainsFunc(class.Fields, func(field events.Field) bool { return field.Path == target && field.Owner == "mapping" }) {
			fields[source] = target
			if table := spec.Shared.ValueMap[target]; table != nil {
				values[target] = table
			}
			if value, ok := spec.Shared.Defaults[target]; ok {
				defaults[target] = value
			}
		} else {
			ignored[source] = target
		}
	}
	for target, value := range spec.Shared.Constants {
		if slices.ContainsFunc(class.Fields, func(field events.Field) bool { return field.Path == target && field.Owner == "mapping" }) {
			constants[target] = value
		}
	}
	for source, target := range item.FieldMap {
		if shared, present := fields[source]; present && shared != target {
			return nil, fmt.Errorf("%w: shared and local mappings reuse %q", ErrInvalidRule, source)
		}
		fields[source] = target
	}
	for target, value := range item.Constants {
		if _, exists := constants[target]; exists || target == "class_uid" {
			return nil, fmt.Errorf("%w: duplicate or platform-owned constant %q", ErrInvalidRule, target)
		}
		constants[target] = value
	}
	for _, target := range fields {
		if _, exists := constants[target]; exists {
			return nil, fmt.Errorf("%w: multiple values for %q", ErrInvalidRule, target)
		}
	}
	maps.Copy(values, item.ValueMap)
	localTargets := make(map[string]bool, len(item.FieldMap))
	for _, target := range item.FieldMap {
		localTargets[target] = true
	}
	for target, value := range item.Defaults {
		if _, ok := defaults[target]; ok || !localTargets[target] {
			return nil, fmt.Errorf("%w: invalid local default for %q", ErrInvalidRule, target)
		}
		defaults[target] = value
	}
	constants["class_uid"] = item.Class
	timeSpec, severity := item.Time, item.Severity
	if timeSpec.Field == "" {
		timeSpec = spec.Shared.Time
	}
	if severity.Field == "" && severity.Default == nil {
		severity = spec.Shared.Severity
	}
	mapping, err := Compile(Mapping{
		ID: item.ID, Name: item.Name, Parse: spec.Parse, ContractVersion: spec.ContractVersion,
		FieldMap: fields, Constants: constants, Time: timeSpec, Severity: severity,
	})
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", item.ID, err)
	}
	mapping.ignored = ignored
	for target, value := range defaults {
		index := slices.IndexFunc(mapping.projections, func(plan projection) bool { return plan.field.Path == target })
		if index < 0 {
			return nil, fmt.Errorf("%w: missing mapping for default %q", ErrInvalidRule, target)
		}
		plan := &mapping.projections[index]
		field := *plan.field
		converted, valid := convertField(field, value)
		if valid && field.JSONType == "integer" {
			if field.Array {
				valid = !slices.ContainsFunc(value.([]any), func(item any) bool {
					_, isNumber := item.(json.Number)
					return !isNumber
				})
			} else {
				_, valid = value.(json.Number)
			}
		}
		if !valid {
			return nil, fmt.Errorf("%w: default violates %s", ErrInvalidRule, target)
		}
		plan.fallback = converted
	}
	for target, table := range values {
		index := slices.IndexFunc(mapping.projections, func(projection projection) bool {
			return projection.field != nil && projection.field.Path == target
		})
		if index < 0 || len(table) == 0 || len(table) > 64 {
			return nil, fmt.Errorf("%w: invalid value map for %s", ErrInvalidRule, target)
		}
		plan := &mapping.projections[index]
		for _, value := range table {
			if _, ok := convertField(*plan.field, value); !ok {
				return nil, fmt.Errorf("%w: value map violates %s", ErrInvalidRule, target)
			}
		}
		plan.values = table
	}
	return mapping, nil
}

// Classifies 指示规则是否配置了按记录判定事件类别。
func (e *Compiled) Classifies() bool {
	return len(e.branches) != 0
}

func covers(earlier, later map[string]map[string]bool) bool {
	for field, allowed := range earlier {
		values, exists := later[field]
		if !exists {
			return false
		}
		for value := range values {
			if !allowed[value] {
				return false
			}
		}
	}
	return true
}

func coveredByEarlierBranches(earlier []branch, conditions map[string]map[string]bool) bool {
	const maxProfiles = 4096
	if len(earlier) < 2 || len(conditions) == 0 {
		return false
	}
	var candidates uint32
	for i, item := range earlier {
		if !slices.ContainsFunc(slices.Collect(maps.Keys(item.conditions)), func(field string) bool {
			return conditions[field] == nil
		}) {
			candidates |= 1 << i
		}
	}
	// 缺少额外字段的记录足以验证覆盖。每个掩码表示给定取值还能命中的分支。
	profiles := map[uint32]struct{}{candidates: {}}
	for _, field := range slices.Sorted(maps.Keys(conditions)) {
		matches := make(map[uint32]struct{})
		for value := range conditions[field] {
			mask := candidates
			for i, item := range earlier {
				if allowed := item.conditions[field]; allowed != nil && !allowed[value] {
					mask &^= 1 << i
				}
			}
			matches[mask] = struct{}{}
		}
		next := make(map[uint32]struct{})
		for profile := range profiles {
			for mask := range matches {
				intersection := profile & mask
				if intersection == 0 {
					return false
				}
				next[intersection] = struct{}{}
				// 复杂规则仍可按顺序运行，限制发布时的组合检查成本。
				if len(next) > maxProfiles {
					return false
				}
			}
		}
		profiles = next
	}
	return true
}

// Issue 标识受影响字段及原因，避免把设备原值写入诊断。
type Issue struct {
	Path string `json:"path,omitempty"`
	Code string `json:"code"`
}

func (e *Compiled) applyRule(original, fields map[string]any, received time.Time) (Result, error) {
	for _, branch := range e.branches {
		matched := true
		for field, allowed := range branch.conditions {
			value, exists := original[field]
			if !exists || !matchValue(allowed, value) {
				matched = false
				break
			}
		}
		if matched {
			event, err := branch.mapping.applyFields(fields, received)
			event.BranchID = branch.id
			return event, err
		}
	}
	event, err := e.fallback.applyFields(fields, received)
	if e.Classifies() {
		event.Issues = append(event.Issues, Issue{Code: "no_matching_branch"})
	}
	return event, err
}

func matchValue(allowed map[string]bool, value any) bool {
	switch value := value.(type) {
	case string:
		return allowed[value]
	case json.Number, float64, int, int64, bool:
		return allowed[textValue(value)]
	default:
		return false
	}
}

// BaseResult 构造具备基础时间、严重度和未分类编码的候选 Event。
func BaseResult(received time.Time) Result {
	return Result{Fields: map[string]any{"class_uid": 0, "activity_id": 0, "time": received.UTC().UnixMilli(), "severity_id": 0}, TimeSource: "received"}
}

func fallbackClass(event *Result, received time.Time) {
	value, _ := integralValue(event.Fields["class_uid"])
	event.CandidateClass = int(value)
	base := BaseResult(received)
	base.Unmapped = event.Unmapped
	for _, key := range []string{"time", "severity_id", "severity", "message", "metadata", "status_id", "status", "status_code", "status_detail"} {
		if value, exists := event.Fields[key]; exists {
			base.Fields[key] = value
		}
	}
	for key, value := range event.Fields {
		if _, present := base.Fields[key]; present || key == "activity_name" {
			continue
		}
		if base.Unmapped == nil {
			base.Unmapped = make(map[string]any)
		}
		base.Unmapped[key] = value
	}
	// 安全发现的状态枚举与基础事件的操作结果含义不同。
	if event.CandidateClass/1000 == 2 {
		for _, key := range []string{"status_id", "status"} {
			delete(base.Fields, key)
		}
	}
	status, _ := integralValue(base.Fields["status_id"])
	if _, present := base.Fields["status"]; !present && status == 99 {
		delete(base.Fields, "status_id")
	}
	event.Fields, event.Unmapped = base.Fields, base.Unmapped
}

// PreviewFields 返回试算可展示的提取字段。
func PreviewFields(fields map[string]any) map[string]any {
	safe := make(map[string]any, min(len(fields), 128))
	budget := 4 * 1024
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if len(safe) >= 128 || budget <= 0 {
			break
		}
		if len(key) > 128 {
			continue
		}
		value := previewValue(key, fields[key])
		keyJSON, _ := json.Marshal(key)
		valueJSON, err := json.Marshal(value)
		size := len(keyJSON) + len(valueJSON) + 2
		if err == nil && size <= budget {
			budget -= size
			safe[key] = value
		}
	}
	return safe
}

// PreviewFieldTypes 返回展示字段的 JSON 类型。敏感字段不提供类型。
func PreviewFieldTypes(fields, preview map[string]any) map[string]string {
	types := make(map[string]string, len(preview))
	for name := range preview {
		if hidePreviewField(name) {
			continue
		}
		switch fields[name].(type) {
		case nil:
			types[name] = "null"
		case string:
			types[name] = "string"
		case json.Number, float64:
			types[name] = "number"
		case bool:
			types[name] = "boolean"
		case []any:
			types[name] = "array"
		case map[string]any:
			types[name] = "object"
		}
	}
	return types
}

func hidePreviewField(key string) bool {
	return isSensitiveKey(key) || key == "message" || key == "msg"
}

func previewValue(key string, value any) any {
	if hidePreviewField(key) {
		return redactedValue
	}
	switch raw := value.(type) {
	case string:
		normalized := strings.TrimSpace(raw)
		if len(raw) > 256 || strings.HasPrefix(normalized, "{") || strings.HasPrefix(normalized, "[") {
			return redactedValue
		}
	case map[string]any, []any:
		return redactedValue
	}
	return value
}

// PreviewEvent 脱敏完整试算的标准事件正文字段。
func PreviewEvent(body []byte) []byte {
	var object map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil {
		return []byte("{}")
	}
	var clean func(map[string]any)
	clean = func(fields map[string]any) {
		for key, value := range fields {
			if child, ok := value.(map[string]any); ok && !isSensitiveKey(key) {
				clean(child)
				continue
			}
			if key != "parse.issues" {
				fields[key] = previewValue(key, value)
			}
		}
	}
	clean(object)
	safe, err := json.Marshal(object)
	if err != nil {
		return []byte("{}")
	}
	if len(safe) > 4*1024 {
		for key := range object {
			switch key {
			case "class_uid", "category_uid", "activity_id", "type_uid", "severity_id", "time":
			default:
				delete(object, key)
			}
		}
		safe, _ = json.Marshal(object)
	}
	return safe
}
