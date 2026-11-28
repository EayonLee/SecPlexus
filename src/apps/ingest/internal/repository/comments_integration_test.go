//go:build integration

package repository_test

import (
	"context"
	"testing"
)

// 回归：改名后表与业务列注释必须还在。
//
// 注释是数据库规范的一部分，容易被"重命名表"这类改动悄悄丢掉：
// COMMENT ON 不会随 RENAME 自动跟随，改完就没提示信息了。
func TestIngestTablesAndColumnsKeepComments(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	// 表注释。
	for _, table := range []string{"inputs", "parsing_rules"} {
		var comment *string
		if err := pool.QueryRow(ctx, `
			SELECT obj_description(('ingest.' || $1)::regclass, 'pg_class')`, table).Scan(&comment); err != nil {
			t.Fatalf("%s 表注释查询失败: %v", table, err)
		}

		if comment == nil || *comment == "" {
			t.Fatalf("ingest.%s 缺少表注释", table)
		}
	}

	// 业务列注释：每个暴露给运维的列都必须说明含义，否则改配置只能猜。
	required := map[string][]string{
		"inputs":        {"name", "protocol", "parsing_rule_id", "configuration", "enabled"},
		"parsing_rules": {"name", "definition", "version"},
	}

	for table, columns := range required {
		for _, column := range columns {
			var comment *string
			if err := pool.QueryRow(ctx, `
				SELECT col_description(('ingest.' || $1)::regclass, ordinal_position)
				  FROM information_schema.columns
				 WHERE table_schema = 'ingest' AND table_name = $1 AND column_name = $2`, table, column).Scan(&comment); err != nil {
				t.Fatalf("%s.%s 列注释查询失败: %v", table, column, err)
			}

			if comment == nil || *comment == "" {
				t.Fatalf("ingest.%s.%s 缺少列注释", table, column)
			}
		}
	}
}
