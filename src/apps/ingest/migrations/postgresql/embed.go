// Package postgresql 嵌入 Ingest 的 PostgreSQL 迁移。
package postgresql

import "embed"

// Files 包含随二进制发布的 SQL 迁移；版本表仍由 Goose 管理。
//
//go:embed *.sql
var Files embed.FS
