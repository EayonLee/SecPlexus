# 数据访问规范

当前 PostgreSQL 保存模块业务数据，Ingest 将事件发布到 Kafka。Ingest 的独立 Event 存储进程消费 Kafka 并写入 ClickHouse。告警状态的持久化由未来告警模块决定。[Ingest 字段与存储设计](../../src/apps/ingest/README.md#event-字段与存储设计)

## 存储职责

| 存储 | 存什么 | 访问方式 | 迁移 |
|---|---|---|---|
| PostgreSQL | 控制面（用户、角色、会话、审计、接入点和解析规则） | `sqlc`：SQL 写在模块的 `queries/*.sql`，生成代码入库，编译期校验列与类型 | `goose`，ledger 按 `<module>.goose_db_version` |
| ClickHouse（暂缓） | 事件流（OCSF）、检索与聚合，只追加 | `github.com/ClickHouse/clickhouse-go/v2`：写用 `PrepareBatch` 分批，读用 `Select`/`ScanStruct`，显式 SQL，SQL 由模块维护 | `goose`（上游已支持 `DialectClickHouse`） |

`sqlc` 不支持 ClickHouse（`engine: clickhouse` 会直接报 `unknown engine`），因此 CK 查询不使用 sqlc。Event 的写入列清单和预期列结构由模块的字段投影派生；
`gorm.io/driver/clickhouse` 仍停留在 v0.7.0（2025-05），不作为选型依据。

## PostgreSQL 规则

1. SQL 只写在模块的 `queries/*.sql`，具名：`-- name: <Verb><Noun> :one|:many|:exec`；Go 里不拼 SQL 片段（含 `LIMIT/OFFSET`、`WHERE` 组合）。
2. 动态过滤只能用静态 SQL + 可空参数（`sqlc.narg` 形式），禁止按条件拼接子句。
3. `repository` 只返回类型化结构；**HTTP 字段名只允许出现在 `internal/model`**，禁止在 SQL 里用 `jsonb_build_object` 生成响应体。
4. 分页二选一并写注释：有界管理列表用 offset + 窗口计数；增长型数据（审计、告警）用 keyset 游标。
5. 关联聚合优先「一次批量查询 + Go 分组」，禁止 N+1，也禁止在 SQL 里产出 API JSON。
6. 生成代码入库且必须与 `.sql` 同步，CI 用 `sqlc diff` 拦截漂移。
7. `service` 只依赖 `*repository.Store` 与 `repository.DB`，不接触驱动类型（pgx/GORM 都不出现在 service 签名里）；换库时替换 `Store` 的实现。
8. 表结构变更走前进迁移，每表每业务列必须有 `COMMENT ON`；封闭取值集用 `CHECK` + `internal/enum` 常量。

## PostgreSQL 已落地的实现约定

1. **生成的查询与调用方共用同一连接或事务**：`repository.DB` 是生成的 `DBTX` 接口，
   `*sql.DB`（根池）与 `*sql.Tx`（事务）都满足它；`Store.Transaction` 把事务句柄下传给每个仓库函数。
   集成测试 `TestGeneratedQueriesShareTheCallerTransaction` 用回滚可见性验证两者是同一条连接。
2. **依赖事务范围的锁操作必须显式校验**：`LockUsers`、`LockManagement` 通过 `requireTransaction`
   拒绝自动提交会话——否则锁会立即释放，静默失效比报错危险。
3. **数组参数用 jsonb 展开**：`jsonb_array_elements_text(sqlc.arg('x')::jsonb)`，由 `textArray`/`uuidArray`
   编码。生成代码用 `database/sql` 参数，数组绑定会引入 `lib/pq` 这套第二驱动；
   `jsonb` 展开保持单一驱动，代价是数组语义由 SQL 侧解释。
4. **封闭取值列映射到 `internal/enum`**：`sqlc.yaml` 用 column override 把 `permissions.status`、
   `permissions.risk_level` 生成为 `enum.PermissionStatus`/`enum.RiskLevel`，编译期即受约束，
   包装层不再做字符串转换。新增封闭取值列时照此添加。
5. **生成物禁止手改**：`sqlcgen/` 只由 `make sqlc` 产生，CI 用 `make sqlc-check`（`sqlc diff`）拦截漂移。
6. **查询在真实库上 PREPARE 过**：`make sqlc-vet` 跑 `sqlc/db-prepare`——脚本自建临时库、应用全部迁移、
   把每条生成查询 PREPARE 一次再删库。静态分析看不见的类型与转换问题只有这一步能发现，CI 每次 PR 都跑。

## ClickHouse 规则

1. 查询必须带时间范围与 `LIMIT`，禁止无界扫描；聚合优先用物化视图预聚合，不要每次全量。
2. 写入采用有条数、字节和期限上限的批次。Ingest 使用同步 `JSONEachRow` 批量写入，确认后提交消费位移。重放沿用已发布事件的同一排序键，精确查询覆盖后台合并前的重复记录。[ClickHouse 批量写入](https://clickhouse.com/docs/optimize/bulk-inserts)
3. 只追加不更新；需要「当前状态」的数据放 PostgreSQL。
4. 建表按保留期和实际查询明确分区、排序键。Ingest 的 [Event 表](../plans/event-contract/storage.md) 按事件发生日分区，以类别、发生时间及事件 ID 排序。变更字段时核对权威目录、物理投影与实际表结构，目标负载下验证扫描量和合并能力。
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
