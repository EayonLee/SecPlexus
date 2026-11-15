package repository

import (
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// ruleSpec 是 parse_rules.spec 的 JSON 契约。
//
// 它是数据库与内部结构体之间的边界：字段名与 JSON 绑定写在这里，
// 内部结构体就能自由重命名而不破坏已存数据。
type ruleSpec struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Vendor      string                  `json:"vendor"`
	Product     string                  `json:"product"`
	Description string                  `json:"description"`
	Parse       parser.Spec             `json:"parse"`
	FieldMap    map[string]string       `json:"field_map"`
	Severity    normalizer.SeveritySpec `json:"severity"`
	Time        normalizer.TimeSpec     `json:"time"`
	Constants   map[string]any          `json:"constants"`
	Samples     []string                `json:"samples"`
}

// rule 把 JSON 契约转成编译输入。
func (s ruleSpec) rule() normalizer.Rule {
	return normalizer.Rule{
		ID:          s.ID,
		Name:        s.Name,
		Vendor:      s.Vendor,
		Product:     s.Product,
		Description: s.Description,
		Parse:       s.Parse,
		FieldMap:    s.FieldMap,
		Severity:    s.Severity,
		Time:        s.Time,
		Constants:   s.Constants,
		Samples:     s.Samples,
	}
}
