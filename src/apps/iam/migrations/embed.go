// Package migrations 发布 IAM 的版本化数据库迁移，已执行脚本不应修改。
package migrations

import "embed"

// Files 包含随二进制发布的 SQL 迁移；版本表仍由 Goose 管理。
//
//go:embed *.sql
var Files embed.FS
