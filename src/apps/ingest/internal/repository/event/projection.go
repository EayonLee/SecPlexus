// Package event 检查标准事件契约与 ClickHouse 物理投影，并生成评审用 DDL。
package event

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/migrations/clickhouse"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
)

//go:embed projection.json
var projectionJSON []byte

type column struct {
	Name       string
	Path       string
	Type       string
	Conversion string
	Comment    string
}

// EventTableMigration 返回用于部署和评审的首版事件表定义。
func EventTableMigration() (string, error) {
	columns, err := migrationV1Columns()
	if err != nil {
		return "", err
	}
	ddl := make([]string, 0, len(columns))
	for _, col := range columns {
		ddl = append(ddl, fmt.Sprintf("    %s %s COMMENT '%s'", col.Name, col.Type, quote.Replace(col.Comment)))
	}

	migration, err := template.New("00001_events.sql.tmpl").Parse(clickhouse.EventTableTemplate)
	if err != nil {
		return "", err
	}
	var sql bytes.Buffer
	if err = migration.Execute(&sql, strings.Join(ddl, ",\n")); err != nil {
		return "", err
	}
	return sql.String(), nil
}

func migrationV1Columns() ([]column, error) {
	var columns []column
	if err := json.Unmarshal([]byte(clickhouse.EventTableColumnsV1), &columns); err != nil {
		return nil, fmt.Errorf("decode Event migration 00001 columns: %w", err)
	}
	return columns, nil
}

type projectedColumn struct {
	column
	segments []string
}

func loadProjection() ([]projectedColumn, *events.Catalog, error) {
	catalog, err := events.Load()
	if err != nil {
		return nil, nil, err
	}
	var projection struct {
		Version string `json:"contract_version"`
		Columns []column
	}
	if err := json.Unmarshal(projectionJSON, &projection); err != nil {
		return nil, nil, err
	}
	if projection.Version != catalog.ContractVersion {
		return nil, nil, fmt.Errorf("event projection contract version %q does not match %q", projection.Version, catalog.ContractVersion)
	}

	columns := make([]projectedColumn, 0, len(projection.Columns))
	names := make(map[string]bool, len(projection.Columns))
	for _, col := range projection.Columns {
		if !identifier.MatchString(col.Name) || names[col.Name] {
			return nil, nil, fmt.Errorf("invalid or repeated event column %q", col.Name)
		}
		names[col.Name] = true
		if col.Comment == "" {
			return nil, nil, fmt.Errorf("event column %s lacks a comment", col.Name)
		}
		columns = append(columns, projectedColumn{column: col, segments: strings.Split(col.Path, ".")})
	}
	for _, required := range []string{"event_id", "event_json", "event_time", "class_uid"} {
		if !names[required] {
			return nil, nil, fmt.Errorf("event projection is missing %s", required)
		}
	}

	for _, summary := range catalog.Classes() {
		class, _ := catalog.Class(summary.UID)
		fields := make(map[string]events.Field, len(class.Fields))
		for _, field := range class.Fields {
			fields[field.Path] = field
		}
		for _, col := range columns {
			if col.Path == "" {
				continue
			}
			field, defined := fields[col.Path]
			if !defined && strings.Contains(col.Type, "Nullable(") {
				continue
			}
			if !defined || field.Array || !col.accepts(field) {
				return nil, nil, fmt.Errorf("event class %d disagrees with %s projection", class.UID, col.Path)
			}
		}
		if outcome := fields["metadata.spx.parse.outcome"]; outcome.JSONType != "string" || outcome.Array {
			return nil, nil, fmt.Errorf("event class %d lacks the parse outcome field", class.UID)
		}
	}

	return columns, catalog, nil
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var quote = strings.NewReplacer("\\", "\\\\", "'", "\\'")

func (col column) accepts(field events.Field) bool {
	switch col.Conversion {
	case "uuid":
		return col.Type == "UUID" && field.JSONType == "string"
	case "epoch_millis":
		return col.Type == "DateTime64(3, 'UTC')" && field.LogicalType == "timestamp_t" && field.Unit == "ms"
	case "unsigned":
		return unsignedColumn.MatchString(col.Type) && field.JSONType == "integer"
	case "string":
		return col.Type == "LowCardinality(String)" && field.JSONType == "string"
	case "blank_to_null":
		return col.Type == "LowCardinality(Nullable(String))" && field.JSONType == "string"
	case "ipv4_mapped_ipv6":
		return col.Type == "Nullable(IPv6)" && field.LogicalType == "ip_t"
	}
	return false
}

var unsignedColumn = regexp.MustCompile(`^(UInt(8|16|32|64)|Nullable\(UInt16\))$`)
