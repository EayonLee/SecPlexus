# 数据访问规范

两个存储各司其职，互不复制：**事件只在 ClickHouse，告警只在 PostgreSQL**。任何模块新增数据访问前先读本文。

## 存储职责

| 存储 | 存什么 | 访问方式 | 迁移 |
|---|---|---|---|
| PostgreSQL | 控制面（用户、角色、会话、审计）与**告警状态**（可更新、进审计） | `sqlc`：SQL 写在模块的 `queries/*.sql`，生成代码入库，编译期校验列与类型 | `goose`，ledger 按 `<module>.goose_db_version` |
| ClickHouse（暂缓） | 事件流（OCSF）、检索与聚合，只追加 | `github.com/ClickHouse/clickhouse-go/v2`：写用 `PrepareBatch` 分批，读用 `Select`/`ScanStruct`，显式 SQL，**无 codegen** | `goose`（上游已支持 `DialectClickHouse`） |

`sqlc` 不支持 ClickHouse（`engine: clickhouse` 会直接报 `unknown engine`），因此 CK 侧不引入生成器；
`gorm.io/driver/clickhouse` 仍停留在 v0.7.0（2025-05），不作为选型依据。

## PostgreSQL 规则

1. SQL 只写在模块的 `queries/*.sql`，具名：`-- name: <Verb><Noun> :one|:many|:exec`；Go 里不拼 SQL 片段（含 `LIMIT/OFFSET`、`WHERE` 组合）。
2. 动态过滤只能用静态 SQL + 可空参数（`sqlc.narg` 形式），禁止按条件拼接子句。
3. `repository` 只返回类型化结构；**HTTP 字段名只允许出现在 `internal/model`**，禁止在 SQL 里用 `jsonb_build_object` 生成响应体。
4. 分页二选一并写注释：有界管理列表用 offset + 窗口计数；增长型数据（审计、告警）用 keyset 游标。
5. 关联聚合优先「一次批量查询 + Go 分组」，禁止 N+1，也禁止在 SQL 里产出 API JSON。
6. 生成代码入库且必须与 `.sql` 同步，CI 用 `sqlc diff` 拦截漂移。
7. `service` 只依赖 `Store` 接口，不依赖具体驱动；换库时新增实现、不改调用点。
8. 表结构变更走前进迁移，每表每业务列必须有 `COMMENT ON`；封闭取值集用 `CHECK` + `internal/enum` 常量。

## ClickHouse 规则（暂缓实施，先登记）

1. 查询必须带时间范围与 `LIMIT`，禁止无界扫描；聚合优先用物化视图预聚合，不要每次全量。
2. 写入必须批量（`PrepareBatch`，按千行级分批）+ 幂等去重键；失败可安全重试，不允许逐行插入。
3. 只追加不更新；需要「当前状态」的数据放 PostgreSQL。
4. 建表必须显式 `PARTITION BY`（按月）与 `ORDER BY`（时间 + 高频过滤列），否则检索会退化为全表扫。
5. 与 PostgreSQL 不 join：跨存储在 service 层组合（PG 取时间窗与实体 → CK 查事件）。
6. 事件表不返回 API JSON；响应结构仍在 `internal/model` 定义。

## 换库预案（PostgreSQL 之外）

换库的真实成本是**方言能力**，不是驱动。当前已使用的 PostgreSQL 特化项（实测计数）：

| 特性 | 次数 | 换到 MySQL 族时的处置 |
|---|---|---|
| `timestamptz` / `interval` | 26 | 改 `datetime` 并重写区间运算 |
| `::type` 强转 | 20 | 改 `CAST(... AS ...)` |
| `jsonb` | 7 | 改 `JSON`（本次改造后只剩 `audit_events.metadata` 这类领域列） |
| `ON CONFLICT` | 6 | 改 `ON DUPLICATE KEY UPDATE` |
| `FOR UPDATE` 行锁 | 4 | 语法相同，复核隔离级别语义 |
| `text[]` + `ANY($1)` | 4 | 改 `IN (?)` 展开 |
| `ILIKE` / `ESCAPE` | 3 / 3 | 改 `lower(c) LIKE lower(?)` |
| 窗口函数 | 2 | MySQL 8 可直接用 |
| `set_config` / `current_setting` | 2 | 无对应，改应用层开关 |
| `pg_advisory_xact_lock` | 3 | 无对应，改应用层锁 |
| 触发器（审计追加写兜底） | 2 | 无对应，移到应用层（应用层本来已写审计） |

结论与顺序：

- PostgreSQL 协议兼容库（openGauss、Kingbase、GaussDB、CockroachDB 等）迁移成本最低：换驱动与 DSN，SQL 基本不动。
- MySQL 协议族需要按上表逐条改 SQL；因为 SQL 集中在 `queries/*.sql`，改动是可枚举的清单，而不是靠日志反推。
- 换库前先做第 7 条（`Store` 接口），否则整改的是业务层调用点而不是一个实现。
