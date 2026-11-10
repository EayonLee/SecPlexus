package repository

import (
	"encoding/json"

	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// queries 返回绑定到当前事务/连接的生成查询。
// 迁移期生成代码是 database/sql 风味，其 DBTX 与 GORM 的 ConnPool 方法集一致，
// 因此生成查询与现有 GORM 调用共用同一个事务，不会出现两套事务。
func queries(db *gorm.DB) *sqlcgen.Queries {
	return sqlcgen.New(db.Statement.ConnPool)
}

// textArray 把字符串集合编码为 jsonb 参数，供 SQL 侧的 jsonb_array_elements_text 展开。
// database/sql 风味的数组绑定会引入 lib/pq，与项目的 pgx 驱动重复，故改用 jsonb。
func textArray(values []string) (json.RawMessage, error) {
	if values == nil {
		values = []string{}
	}

	return json.Marshal(values)
}
