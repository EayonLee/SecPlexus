package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository/sqlcgen"
)

// queries 返回绑定到当前会话的生成查询。
// 生成代码是 database/sql 风味，其 DBTX 与 GORM 的 ConnPool 方法集一致：
// 事务里拿到的是同一个 *sql.Tx，普通会话拿到的是 *sql.DB，因此两种调用共用同一条连接。
// 前提是 db 来自 gorm.Open 或其会话派生（Statement 非空），仓库内所有调用都满足。
func queries(db *gorm.DB) *sqlcgen.Queries {
	if db == nil || db.Statement == nil || db.Statement.ConnPool == nil {
		panic("repository: gorm session is not bound to a connection")
	}

	return sqlcgen.New(db.Statement.ConnPool)
}

// requireTransaction 拒绝在没有事务的会话上执行依赖事务范围的操作。
// 行锁与 advisory 事务锁只在事务结束前有效：若在自动提交连接上执行，
// 锁会立刻释放，调用方以为拿到了互斥，实际没有。
func requireTransaction(db *gorm.DB, operation string) error {
	if _, ok := db.Statement.ConnPool.(*sql.Tx); !ok {
		return fmt.Errorf("%s must run inside a transaction", operation)
	}

	return nil
}

// textArray 把字符串集合编码为 jsonb 参数，供 SQL 侧的 jsonb_array_elements_text 展开。
// database/sql 风味的数组绑定会引入 lib/pq，与项目的 pgx 驱动重复，故改用 jsonb。
func textArray(values []string) (json.RawMessage, error) {
	if values == nil {
		values = []string{}
	}

	return json.Marshal(values)
}

// uuidArray 把标识集合编码为 jsonb 参数，供 SQL 侧的 jsonb_array_elements_text 展开。
// 与 textArray 同理，避免为数组参数引入第二套驱动。
func uuidArray(values []uuid.UUID) (json.RawMessage, error) {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, value.String())
	}

	return json.Marshal(items)
}
