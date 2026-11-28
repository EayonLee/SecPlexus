// Package events 加载 Ingest 拥有的标准事件字段目录。
package events

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
)

//go:embed catalog.json
var source []byte

// Text 是字段名称、释义及枚举值的双语内容。
type Text struct {
	En   string `json:"en"`
	ZhCN string `json:"zh-CN"`
}

// EnumValue 是枚举编码及其业务含义。
type EnumValue struct {
	Value       int              `json:"value"`
	Name        Text             `json:"name"`
	Description *EnumDescription `json:"description,omitempty"`
}

// EnumDescription 提供有补充信息的语言版本。
type EnumDescription struct {
	En   string `json:"en,omitempty"`
	ZhCN string `json:"zh-CN,omitempty"`
}

// IntegerRange 是协议定义的闭区间。
type IntegerRange struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

// ValueSuggestion 是常用的合法取值及其协议名称。
type ValueSuggestion struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

// Field 描述一个可查询的事件属性。
type Field struct {
	Path                string            `json:"path"`
	Name                Text              `json:"name"`
	Description         Text              `json:"description"`
	LogicalType         string            `json:"logical_type"`
	JSONType            string            `json:"json_type"`
	Array               bool              `json:"array"`
	Requirement         string            `json:"requirement"`
	Condition           *Text             `json:"condition,omitempty"`
	Owner               string            `json:"owner"`
	Unit                string            `json:"unit,omitempty"`
	Enum                []EnumValue       `json:"enum,omitempty"`
	IntegerRange        *IntegerRange     `json:"integer_range,omitempty"`
	ValueSuggestions    []ValueSuggestion `json:"value_suggestions,omitempty"`
	PreferredEnumValues []int             `json:"preferred_enum_values,omitempty"`
	ObjectFields        []string          `json:"object_fields,omitempty"`
	RequiredOutput      []string          `json:"required_output,omitempty"`
}

// Class 描述一个已发布的事件类。
type Class struct {
	UID                     int                 `json:"class_uid"`
	Name                    Text                `json:"name"`
	Description             Text                `json:"description"`
	CategoryUID             int                 `json:"category_uid"`
	Category                Text                `json:"category"`
	Aliases                 map[string][]string `json:"aliases,omitempty"`
	Fields                  []Field             `json:"fields,omitempty"`
	Constraints             []Constraint        `json:"constraints,omitempty"`
	RecommendedMappingPaths []string            `json:"recommended_mapping_paths,omitempty"`
}

// Constraint 描述一个事件类中字段的存在约束。
type Constraint struct {
	Paths        []string `json:"paths"`
	Mode         string   `json:"mode"`
	When         string   `json:"when,omitempty"`
	MappingPaths []string `json:"mapping_paths"`
}

// FieldUsage 描述一个字段在指定事件类中的有效定义。
type FieldUsage struct {
	ClassUID int   `json:"class_uid"`
	Field    Field `json:"field"`
}

// CatalogField 是按完整路径去重的目录项。
type CatalogField struct {
	Field
	Usages []FieldUsage `json:"usages"`
}

// Version 标识目录的语义版本和内容修订。
type Version struct {
	ContractVersion string `json:"contract_version"`
	OCSFVersion     string `json:"ocsf_version"`
	CatalogRevision string `json:"catalog_revision"`
}

// Catalog 保存进程启动时构建的字段目录。加载完成后只读。
type Catalog struct {
	Version
	classes []Class
	fields  []CatalogField
}

// Classes 返回事件类的摘要。
func (c *Catalog) Classes() []Class {
	classes := slices.Clone(c.classes)
	for i := range classes {
		classes[i].Fields = nil
		classes[i].Constraints = nil
		classes[i].RecommendedMappingPaths = nil
	}
	return classes
}

// Class 返回指定类的字段。返回的字段和枚举仅用于读取。
func (c *Catalog) Class(uid int) (Class, bool) {
	index := slices.IndexFunc(c.classes, func(class Class) bool { return class.UID == uid })
	if index < 0 {
		return Class{}, false
	}
	return c.classes[index], true
}

// Fields 返回按路径排序的只读目录。
func (c *Catalog) Fields() []CatalogField { return c.fields }

// CurrentVersion 是平台当前发布的事件契约。
const CurrentVersion = "v1"

// CurrentOCSFVersion 是当前事件契约固定的上游版本。
const CurrentOCSFVersion = "1.10.0"

// LoadVersion 返回当前只读契约。
func LoadVersion(version string) (*Catalog, error) {
	if version != CurrentVersion {
		return nil, fmt.Errorf("unsupported event contract %q", version)
	}
	return Load()
}

// Load 加载生成的字段目录。
var Load = sync.OnceValues(func() (*Catalog, error) {
	var artifact struct {
		Version
		Classes []Class
	}
	if err := json.Unmarshal(source, &artifact); err != nil {
		return nil, fmt.Errorf("decode event catalog: %w", err)
	}
	if artifact.ContractVersion != CurrentVersion || artifact.OCSFVersion != CurrentOCSFVersion || len(artifact.Classes) == 0 {
		return nil, fmt.Errorf("invalid generated event catalog")
	}
	byPath := make(map[string]*CatalogField)
	for _, class := range artifact.Classes {
		for _, field := range class.Fields {
			entry := byPath[field.Path]
			if entry == nil {
				entry = &CatalogField{Field: field}
				byPath[field.Path] = entry
			}
			if field.JSONType != entry.JSONType || field.LogicalType != entry.LogicalType || field.Array != entry.Array || field.Owner != entry.Owner {
				return nil, fmt.Errorf("inconsistent event field %s", field.Path)
			}
			entry.Usages = append(entry.Usages, FieldUsage{ClassUID: class.UID, Field: field})
		}
	}
	fields := make([]CatalogField, 0, len(byPath))
	for _, field := range byPath {
		fields = append(fields, *field)
	}
	slices.SortFunc(fields, func(a, b CatalogField) int { return strings.Compare(a.Path, b.Path) })
	return &Catalog{Version: artifact.Version, classes: artifact.Classes, fields: fields}, nil
})
