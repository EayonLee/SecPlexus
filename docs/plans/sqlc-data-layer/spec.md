# 数据访问层改造：SQL 成为可校验的源，HTTP JSON 回到 Go

## 背景

当前 IAM 的数据访问层是「GORM 当执行器 + 手写 SQL 字符串 + 在 SQL 里拼 HTTP JSON」。它工作正常、测试齐全，
但三类问题会随着模块增多而成倍放大，而且**现在改的成本最低**（未发布、只有一个模块在用）。

### 0. 改造的真实范围（先界定清楚）

全仓目前只有 IAM 访问数据库，实测：`src/apps/console` 的 SQL 命中 **0**、迁移目录 **0**、gorm/pgx 引用 **0**；
`src/apps/web` 无数据库访问。所以"全局改造"的实际范围 = **IAM 的全部 SQL**（应用内 127 处 SQL 关键字命中，
其中 `internal/repository` 包内 88 处），后续新模块（ingestor、correlator 等）按本规范新写即可，没有存量包袱。

### 1. 类型脱节：SQL 与 Go 之间没有编译期契约

盘点 `src/apps/iam/internal/repository`（实测）：

| 指标 | 数值 |
|---|---|
| 包内 Go 文件总行数 | 2,388（含集成测试） |
| SQL 关键字命中（`SELECT/INSERT/UPDATE/DELETE FROM`） | 88 处 |
| 手写 `.Scan(&a, &b, ...)` | 41 处 |
| GORM 用法分布 | `Raw` 24、`Exec` 34、`Where` 20、`Take` 4、`Create` 3、`Model` 1、`Find` 1 |
| 表到结构体的映射标签 | 仅 2 个文件（权限目录），其余全部手写扫描 |

后果：改列名、改类型、调换 SELECT 字段顺序，编译器不会报错，只有集成测试或线上才能发现；
SQL 以 Go 字符串常量存在，没有语法高亮、无法用数据库工具解释执行计划、无法静态校验。

### 2. HTTP JSON 被写进 SQL（更严重）

`internal/repository/lists.go` 用 `jsonb_build_object` 直接产出 API 响应体（4 处投影，例如 `userProjection`、
`roleProjection`、审计投影），再由 handler 原样写出：

```go
// handler/management/users.go:49
result.Success(w, json.RawMessage(data))
// handler/management/audit.go:35
result.Success(c.Writer, map[string]any{"items": page.Items, ...})  // Items 是 json.RawMessage
```

问题不是"性能"，而是**契约位置错误**：

- API 字段名（`display_name`、`user_count`、`next_cursor`）散落在 SQL 字符串里，改字段要改 SQL，且没有类型检查；
- `internal/model/http.go` 里的 `APIUser`/`APIRole` 事实上被绕过，DTO 与真实响应可能长期不一致；
- 排序/聚合（`jsonb_agg(... ORDER BY r.name)`）在数据库里完成，Go 侧无法复用、无法单测、无法按权限裁剪字段；
- 序列化行为（null 与空串、空数组、时间格式）由 PostgreSQL 决定，Go 无法统一。

### 3. 动态 SQL 用字符串拼接实现

```go
// lists.go：分页
sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
db.WithContext(ctx).Raw("SELECT count(*) FROM ("+unpagedSQL+") AS matches", unpagedArgs...)
// lists.go：审计列表 6 个可选条件
statement := auditSelectSQL + " WHERE " + strings.Join(where, " AND ") + fmt.Sprintf(" ORDER BY ...")
```

这是阅读与审查成本最高的部分：参数序号与片段顺序必须人工保持一致，一旦漏掉就变成注入或错参。

### 4. 可行性已验证（不是纸上方案）

- sqlc **v1.31.1** 实测可直接把 `src/apps/iam/migrations/*.sql` 当 schema 源解析：`CREATE SCHEMA IF NOT EXISTS iam`、
  审计追加写触发器（函数 + `current_setting`）、`CHECK` 约束、`COMMENT ON` 全部通过；生成的 `models.go` 使用
  `pgtype.*` 类型，并把列注释带成 Go 字段注释。
- `sqlc diff` 命令存在，可用于 CI "生成物与 SQL 不同步即失败"。
- 生态事实：sqlc 是 SQL-first 代码生成的主流工具（约 17K stars，官方支持 `pgx/v5`，见
  https://docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html ）；GitHub 自家的 Go 数据访问代码
  （`github/orchestrator` 的 `go/db/db.go` 与 `golib/sqlutils`）直接用 `database/sql` 手写 SQL，不用 ORM，
  说明"显式 SQL"本身就是大型 Go 服务的主流选择。

## 目标

1. SQL 成为**唯一事实源**并进入编译期：`.sql` 文件 → `sqlc generate` → 类型安全的查询函数与行结构。
2. **HTTP 契约回到 Go**：repository 不再返回 JSON；由 handler 映射 `model.API*`。
3. 消灭运行时 SQL 拼接：动态过滤只能用静态 SQL（`sqlc.narg` 语义），分页/游标逻辑保留但改为具名查询。
4. 行为与外部 HTTP 契约**零变化**（同一批集成测试必须原样通过）。
5. 沉淀规范，使后续模块（ingestor、correlator、console…）默认按同一套方式写。

## 非目标

- 不更换迁移工具（继续 goose）。
- 不引入 ORM（GORM ORM 模式 / ent）；GORM 最终从 IAM 移除。
- 不改 HTTP 路由、权限语义、审计可用性、前端字段。
- 不做读写分离、连接池改造、缓存层。

## 数据访问层契约（分层职责）

| 层 | 允许 | 禁止 |
|---|---|---|
| `internal/repository/queries/*.sql` | 具名查询、`sqlc.narg` 可空过滤、窗口计数、行锁、UPSERT | 运行时拼接的子句；`jsonb_build_object` 形式的 API 投影 |
| `internal/repository/sqlcgen` | **只能由 `sqlc generate` 生成** | 手改（CI `sqlc diff` 会失败） |
| `internal/repository`（包装层） | 参数校验后的类型转换、枚举映射、事务边界、错误归一；**JSONB 业务列**（如 `audit_events.metadata`）可映射为 `json.RawMessage`，它属于领域数据 | 返回 `map[string]any`、或任何**已序列化的响应体**（列表项、详情投影、`items/limit/total` 信封） |
| `internal/service` | 业务规则、权限判定、审计写入、返回**类型化结果** | 导入 `internal/model`（现有规则不变）、返回 JSON |
| `internal/handler` | 绑定 DTO、映射 `model.API*`、`result.Success/Fail` | 手写 SQL、拼接响应字段 |

### 命名与文件规范

- 每个模块一个 schema 目录：`internal/repository/queries/<domain>.sql`（如 `users.sql`、`roles.sql`、`audit.sql`）。
- 查询名 `-- name: <Verb><Noun> :one|:many|:exec`，例：`-- name: ListUsersPage :many`、`-- name: LockUserForUpdate :one`。
- 生成代码放在 `internal/repository/sqlcgen`，包名 `sqlcgen`；包装层是唯一调用方。
- 枚举列在 SQL 里保持 `text`，在包装层用 `internal/enum` 常量转换；`CHECK` 仍是唯一权威。

### 契约不变性（外部行为）

对现有每个端点，改造前后 JSON **逐字段等价**：字段名、嵌套结构、空值形态（`null` vs `""` vs `[]`）、
角色数组排序、分页字段（`items/limit/offset/total`、`items/limit/next_cursor`）都必须一致。
判定方式：现有 `server_integration_test.go` 与 `management/*_test.go` 原样通过 + 迁移批次内的一次性 golden 对比
（对比旧实现的响应字节，确认等价后删除对比代码，不长期保留）。

## 验收标准

1. `internal/repository` 中不再存在手写 `.Scan(`（sqlc 生成的除外）；`grep` 为 0。
2. `internal/repository` 中不再出现 `jsonb_build_object`（4 处清零）；handler 不再对列表/详情使用
   `json.RawMessage` 直出。
3. `grep -E '"(SELECT|FROM|WHERE|ORDER|LIMIT)[^"]*"\s*\+' src/apps/iam/internal/repository` 无运行时拼接结果。
4. `sqlc diff` 在 CI 通过；故意改一个 `.sql` 而不重新生成，CI 必须失败。
5. 所有现有集成测试与 E2E 通过；`make lint-go`、`make vuln-go`、`make test` 全绿。
6. GORM 依赖从 IAM 移除（`go.mod` 不再被 IAM 引用；如 console 仍用则保留其引用并单独说明）。
7. 新模块按本契约实现即可合规：`docs/development/data-access.md` 存在并被 AGENTS 引用。

## 按存储的数据访问契约（定案）

事件只在 ClickHouse、告警只在 PostgreSQL，两者不互相复制；**CK 侧本阶段不实施**，规则先登记在
`docs/development/data-access.md`，等 correlator 计划评审时启用。

| 存储 | 职责 | 访问方式 | 迁移 |
|---|---|---|---|
| PostgreSQL | 控制面与告警状态（可更新、进审计） | **sqlc**：`.sql` 源 + 生成代码 + 编译期校验 | goose（现有） |
| ClickHouse（暂缓） | 事件流、检索与聚合（只追加） | `clickhouse-go/v2`：`PrepareBatch` 批量写、`Select`/`ScanStruct` 读，显式 SQL，无 codegen | goose（`DialectClickHouse` 上游已支持） |

跨存储不 join：告警与事件的关联只能在 service 层组合（PG 取告警时间窗 → CK 查事件）。

## 仓储接口化（可换库的结构前提）

现状是 `service` 直接持有 `s.Store.DB`（18 处）并调用 42 个包级函数，等于把 PostgreSQL 实现焊进业务层。
本计划要求把 `repository` 收敛为 `Store` 接口（service 依赖接口），换库时只新增实现、不改调用点。

## 对后续模块的强制规范（写入 `docs/development/data-access.md`）

1. 表结构由 goose 迁移定义，`COMMENT ON` 必填；sqlc 直接以迁移为 schema 源，不再维护第二份 DDL。
2. 所有 SQL 写在模块自己的 `queries/*.sql`，具名、参数化；禁止在 Go 里拼接 SQL 片段（含 `LIMIT/OFFSET`）。
3. repository 只返回类型化结构；HTTP 字段名只允许出现在 `internal/model`。
4. 列表分页二选一并写进注释：管理型有界列表用 offset + 窗口计数；增长型数据用 keyset 游标。
5. 关联聚合优先"一次批量查询 + Go 侧分组"，禁止在 SQL 里生成 HTTP JSON，也禁止 N+1。
6. 生成代码入库且必须与 `.sql` 同步（CI `sqlc diff` 门禁）。
