package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// DB 是仓库可用的数据库连接：根连接池或单一事务。
// 两者都实现生成的 DBTX 接口，因此同一份查询代码既能自动提交也能加入调用方事务。
type DB interface {
	sqlcgen.DBTX
}

// queries 返回绑定到当前连接或事务的生成查询。
func queries(db DB) *sqlcgen.Queries { return sqlcgen.New(db) }

// requireTransaction 拒绝在没有事务的连接上执行依赖事务范围的操作。
// 行锁与 advisory 事务锁只在事务结束前有效：若在自动提交连接上执行，
// 锁会立刻释放，调用方以为拿到了互斥，实际没有。
func requireTransaction(db DB, operation string) error {
	if _, ok := db.(*sql.Tx); !ok {
		return fmt.Errorf("%s must run inside a transaction", operation)
	}

	return nil
}

// textArray 把字符串集合编码为 jsonb 参数，供 SQL 侧的 jsonb_array_elements_text 展开。
// 生成的查询使用 database/sql 参数，数组绑定会引入第二套驱动，故改用 jsonb。
func textArray(values []string) (json.RawMessage, error) {
	if values == nil {
		values = []string{}
	}

	return json.Marshal(values)
}

// uuidArray 把标识集合编码为 jsonb 参数，供 SQL 侧的 jsonb_array_elements_text 展开。
func uuidArray(values []uuid.UUID) (json.RawMessage, error) {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, value.String())
	}

	return json.Marshal(items)
}
