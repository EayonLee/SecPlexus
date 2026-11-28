// Package clickhouse 嵌入 Ingest 的 ClickHouse 迁移模板。
package clickhouse

import _ "embed"

// EventTableTemplate 是首版 Event 表的建表模板。
//
//go:embed 00001_events.sql.tmpl
var EventTableTemplate string

// EventTableColumnsV1 冻结首版建表所用的列名、类型和注释。
//
//go:embed 00001_columns.json
var EventTableColumnsV1 string

// LedgerSQL 创建 ClickHouse 事件迁移账本。
//
//go:embed ledger.sql
var LedgerSQL string
