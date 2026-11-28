package normalizer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

// mappingSpec 是 mappings.definition 的 JSON 契约。
type mappingSpec struct {
	Extraction
	ContractVersion string          `json:"contract_version"`
	CatalogRevision string          `json:"catalog_revision,omitempty"`
	Description     json.RawMessage `json:"description"`
	Shared          mappingFields   `json:"shared"`
	Branches        []branchSpec    `json:"branches"`
}

// ExecutionDigest 生成执行语义和来源上下文的摘要，规则展示名称和描述不影响摘要。
func ExecutionDigest(raw []byte, vendor, product string) (string, error) {
	spec, err := decodeDefinition(raw)
	if err != nil {
		return "", err
	}
	spec.Description = nil
	for index := range spec.Branches {
		spec.Branches[index].Name = ""
	}
	canonical, err := json.Marshal(struct {
		Definition mappingSpec
		Vendor     string
		Product    string
	}{spec, vendor, product})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// BranchesToVerify 返回发布时需重新试算的分支。来源上下文变更时调用方传入空的旧定义。
func BranchesToVerify(previous, proposed []byte) ([]string, error) {
	current, err := decodeDefinition(proposed)
	if err != nil {
		return nil, err
	}
	var prior mappingSpec
	if len(previous) != 0 {
		prior, err = decodeDefinition(previous)
		if err != nil {
			return nil, err
		}
	}

	currentBranches, priorBranches := current.Branches, prior.Branches
	current.Branches, prior.Branches = nil, nil
	current.Description, prior.Description = nil, nil
	routingChanged := !reflect.DeepEqual(current, prior)
	var required []string
	for index, branch := range currentBranches {
		if index >= len(priorBranches) {
			routingChanged = true
		} else {
			old := priorBranches[index]
			routingChanged = routingChanged || old.ID != branch.ID || old.Class != branch.Class || !reflect.DeepEqual(old.Match, branch.Match)
			if !routingChanged && reflect.DeepEqual(old.mappingFields, branch.mappingFields) {
				continue
			}
		}
		required = append(required, branch.ID)
	}
	if len(required) == 0 && len(currentBranches) != 0 && len(currentBranches) != len(priorBranches) {
		required = append(required, currentBranches[0].ID)
	}
	return required, nil
}

// CompileDefinition 将规则 JSON 编译为不可变的解析计划。
func CompileDefinition(raw []byte) (*Compiled, error) {
	spec, err := decodeDefinition(raw)
	if err != nil {
		return nil, err
	}
	return compileBranches(spec)
}

// PrepareDefinition 校验管理写入，返回归一化定义与预览可复用的解析计划。
func PrepareDefinition(raw json.RawMessage) (json.RawMessage, *Compiled, error) {
	spec, err := decodeDefinition(raw)
	if err != nil {
		return nil, nil, err
	}
	var description string
	if len(spec.Description) != 0 {
		if spec.Description[0] != '"' {
			return nil, nil, fmt.Errorf("%w: %w", ErrInvalidRule, &validation.Issue{Field: validation.Description, Code: "invalid_type"})
		}
		if err := json.Unmarshal(spec.Description, &description); err != nil {
			return nil, nil, err
		}
	}
	clean, err := validation.RuleText(validation.Description, description)
	if err != nil {
		return nil, nil, err
	}
	if len(spec.Branches) == 0 {
		return nil, nil, &validation.Issue{Field: "class_uid", Code: "required"}
	}
	engine, err := compileAuthoring(spec)
	if err != nil {
		return nil, nil, err
	}
	if clean == description {
		return raw, engine, nil
	}
	// 保留调用方的其余定义，只替换发生变化的描述。
	var definition map[string]json.RawMessage
	if err := json.Unmarshal(raw, &definition); err != nil {
		return nil, nil, err
	}
	definition["description"], _ = json.Marshal(clean)
	canonical, err := json.Marshal(definition)
	return canonical, engine, err
}

// CompileDraft 对完整规则预览执行与保存一致的字段和事件类校验。
func CompileDraft(raw []byte) (*Compiled, error) {
	spec, err := decodeDefinition(raw)
	if err != nil {
		return nil, err
	}
	return compileAuthoring(spec)
}

func compileAuthoring(spec mappingSpec) (*Compiled, error) {
	engine, err := compileBranches(spec)
	if err != nil {
		return nil, err
	}
	catalog, err := events.Load()
	if err != nil {
		return nil, err
	}
	if spec.CatalogRevision != "" && catalog.CatalogRevision != spec.CatalogRevision {
		return nil, &validation.Issue{Field: "catalog_revision", Code: "stale_catalog"}
	}
	targets := make(map[string]bool, len(spec.Shared.FieldMap)+len(spec.Shared.Constants))
	for _, target := range spec.Shared.FieldMap {
		targets[target] = true
	}
	for target := range spec.Shared.Constants {
		targets[target] = true
	}
	classes := make(map[int]bool, len(spec.Branches))
	for _, item := range spec.Branches {
		classes[item.Class] = true
	}
	if len(classes) == 0 {
		classes[0] = true
	}
	for _, field := range catalog.Fields() {
		if !targets[field.Path] {
			continue
		}
		var first *events.Field
		for _, usage := range field.Usages {
			if !classes[usage.ClassUID] || usage.Field.Owner != "mapping" {
				continue
			}
			if first == nil {
				first = &usage.Field
				continue
			}
			if first.Unit != usage.Field.Unit || !slices.EqualFunc(first.Enum, usage.Field.Enum, func(a, b events.EnumValue) bool {
				return a.Value == b.Value && a.Name.En == b.Name.En
			}) {
				first = nil
				break
			}
		}
		if first == nil {
			return nil, &validation.Issue{Field: validation.Field("shared." + field.Path), Code: "incompatible_shared_field"}
		}
	}
	for _, branch := range engine.branches {
		mapping := branch.mapping
		shape := make(map[string]any, len(mapping.projections)+len(mapping.constants))
		for _, plan := range mapping.projections {
			writePath(shape, plan.segments, "mapped")
		}
		for _, fixed := range mapping.constants {
			writePath(shape, fixed.segments, fixed.value)
		}
		for _, path := range []string{"src_endpoint", "dst_endpoint"} {
			if !hasIdentity(shape[path], "ip", "mac", "uid", "hostname", "domain") {
				delete(shape, path)
			}
		}
		if path := incompleteClass(shape, mapping.constraints); path != "" {
			return nil, &validation.Issue{
				Field: validation.Field("branches." + branch.id + "." + path), Code: "required_mapping",
			}
		}
	}
	return engine, nil
}

func decodeDefinition(raw []byte) (mappingSpec, error) {
	var spec mappingSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&spec); err != nil {
		return mappingSpec{}, fmt.Errorf("%w: %w", ErrInvalidRule, err)
	}
	if decoder.Decode(new(json.RawMessage)) != io.EOF {
		return mappingSpec{}, fmt.Errorf("%w: trailing data in the rule definition", ErrInvalidRule)
	}
	if len(spec.Description) > 0 && spec.Description[0] != '"' && !bytes.Equal(spec.Description, []byte("null")) {
		return mappingSpec{}, fmt.Errorf("%w: %w", ErrInvalidRule, &validation.Issue{Field: validation.Description, Code: "invalid_type"})
	}
	return spec, nil
}
