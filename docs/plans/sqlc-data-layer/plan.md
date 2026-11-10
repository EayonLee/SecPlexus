# 数据访问层改造：实施计划

关联 Spec：[spec.md](spec.md)。前置计划：[go-quality-gates](../go-quality-gates/plan.md)（先有 lint 门禁，再做大改）。

## 一、目标结构

```
src/apps/iam/internal/repository/
├── queries/                 # 新增：SQL 唯一事实源（人写）
│   ├── users.sql
│   ├── roles.sql
│   ├── permissions.sql
│   ├── sessions.sql
│   ├── audit.sql
│   └── maintenance.sql
├── sqlcgen/                 # 新增：生成代码，禁止手改
│   └── db.go models.go users.sql.go roles.sql.go ...
├── store.go                 # 事务/连接/启动同步（database/sql 适配 + Store.Transaction）
├── users.go roles.go ...    # 包装层：类型转换、枚举、错误归一、审计写入
└── lists.go                 # 删除（被 queries/*.sql + 包装层替代）
```

`sqlc.yaml`（实测可直接解析现有迁移）：

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: src/apps/iam/migrations        # goose 迁移即 schema 源，已被实测验证（触发器/COMMENT/CHECK）
    queries: src/apps/iam/internal/repository/queries
    gen:
      go:
        package: sqlcgen
        out: src/apps/iam/internal/repository/sqlcgen
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
        overrides:
          - column: "iam.audit_events.metadata"
            go_type: { import: "encoding/json", type: "RawMessage" }
```

## 一·补：适用范围

本计划只覆盖 **PostgreSQL**（IAM 的全部 SQL：应用内 127 处命中，`internal/repository` 包内 88 处）。
Console 与 Web 当前不访问数据库（实测 SQL/迁移/gorm+pgx 引用均为 0）。
ClickHouse（事件）本阶段**不实施**：规则先登记在 `docs/development/data-access.md`，由 correlator 计划启用。

## 二、关键决策与取舍

| # | 决策 | 备选与被否原因 |
|---|---|---|
| D1 | 采用 **sqlc**（SQL-first 代码生成） | **ent**：schema as code 由它生成迁移，与现有 goose 迁移 + 数据库触发器/约束体系冲突；**GORM ORM 模式**：本项目依赖 DB 端语义（`FOR UPDATE`、窗口计数、`ON CONFLICT`、`set_config`），链式最终仍退回 `.Raw()`，隐式更新/预加载难排查；**sqlx**：只解决扫描，不解决 SQL 静态校验；**squirrel/goqu**：把可 review 的 SQL 变成代码，仍无编译期列检查 |
| D2 | schema 源 = 现有 goose 迁移 | 用 `pg_dump` 快照要新增同步纪律；实测 sqlc 已能解析迁移，无需第二份 DDL |
| D3 | 生成物入库 + CI `sqlc diff` 门禁 | 现场生成会让构建依赖 sqlc 二进制；入库 + diff 与现有 `catalogs:check` 纪律一致 |
| D4 | 动态过滤改 **静态 SQL + 可空参数** | 现状审计列表用字符串拼接 6 个可选条件。改为单条静态查询 + `sqlc.narg`；被否：按组合生成多条查询（组合爆炸）、引入 query builder（同 D1 被否理由） |
| D5 | 分页策略保持：管理列表 offset + `count(*) OVER()`；审计列表 keyset | `limit/offset/total` 与 `next_cursor` 是对外契约，不改；窗口计数保持单次往返 |
| D6 | **HTTP JSON 从 SQL 移回 Go**：repository 返回类型化行，handler 映射 `model.API*`。注意：被禁止的是**响应投影**，`audit_events.metadata` 这类 JSONB 业务列仍映射为 `json.RawMessage` | 继续 `jsonb_build_object` 会让 API 契约散落在 SQL；`service` 不能导入 `model`（AGENTS 既有规则），映射只能放 handler，与 `handler/self.go` 的 `APIMe` 写法一致 |
| D7 | 用户列表的角色聚合改 **两次具名查询 + Go 分组** | 现状在 SQL 里 `jsonb_agg`。改为先查当页用户，再 `WHERE user_id = ANY($1)` 查角色；避免 N+1，也避免 JSON in SQL。单页 ≤ 100 用户、角色数十 |
| D8 | 枚举列保持 `text`，包装层用 `internal/enum` 转换 | 让 sqlc 生成自定义类型需 overrides 与 CHECK 同步，收益低；CHECK 仍是权威 |
| D9 | 事务统一走包装层 `WithTx`（pgx `BeginFunc`） | 现状用 GORM `Transaction`；迁移期并存，最后一批统一 |
| D10 | `locks.go`（advisory lock）与 `maintenance.go`（保留期清理 + `set_config`）同样进 `.sql` | 逻辑短但语义特殊，sqlc 只是把它们变成具名 `:one/:exec` |

## 三、迁移批次（每批一个 PR，独立可回滚）

| 批次 | 范围 | 涉及文件 / 语句数 | 关键点 | 必需测试层 |
|---|---|---|---|---|
| B0 | 骨架 | 新增 `sqlc.yaml`、`queries/`、`sqlcgen/`；`sqlc diff` 进 CI | 生成物首次入库；先迁 `locks.go`(1) 演练流程 | 集成：advisory lock 行为不变 |
| B1 | 权限与角色 | `authorization_queries.go`(5)、`roles.go`(8) | 内置角色/权限同步保持幂等；`builtin` 标记 | 集成：角色创建/编辑/删除 + 权限校验 |
| B2 | 用户与凭据 | `administrator.go`(3)、`store.go`(7) 的用户/凭据部分 | 配置托管管理员不变量；`auth_epoch` 递增 | 集成：管理员同步、改密、锁定 |
| B3 | 会话 | `session_queries.go`(9) | 双超时、令牌轮换、`state` 状态机 | 集成：会话创建/校验/撤销 |
| B4 | 列表与审计（含 JSON 出 SQL） | `lists.go`(9)：用户列表、角色列表、审计列表、用户详情、角色详情 | 动态 where→静态 narg；`jsonb_build_object` 4 处清零；`listJSON` 删除 | 集成：分页/筛选/游标 + 一次性契约等价对比 |
| B5 | 维护与审计清理 | `maintenance.go`(10) | 事务内 `set_config('iam.audit_purge','on')` 保持 | 集成：保留期清理 + 追加写触发器仍拒绝 UPDATE |
| B6 | 清理 | 删除 GORM 使用、`listJSON`、旧 SQL 常量 | 依赖注入从 `*gorm.DB` 改为 `repository.DB`；**未**接口化（见第十节） | 全量：`make test` + 集成 + E2E |

集成测试里的 SQL（`migrations_integration_test.go` 10、`maintenance_integration_test.go` 8、
`administrator_integration_test.go` 7、`timezone_integration_test.go` 4）随对应批次改为调用生成查询，
不允许为"少改测试"而保留手写 SQL。

## 四、B4 的详细设计（最复杂的一批）

**用户列表**（现状：一条 SQL 里 `jsonb_build_object` + 窗口计数）

1. `-- name: ListUsersPage :many` → 返回 `id, username, display_name, email, status, configured, auth_epoch, total_count`
   （保留 `count(*) OVER()`），筛选用 `WHERE ($1 = '' OR username ILIKE $1 ESCAPE ...)`。
2. `-- name: ListRolesByUserIDs :many` → `SELECT user_id, r.id, r.name FROM iam.user_roles ... WHERE user_id = ANY($1)`。
3. 包装层组装 `[]UserRow{Roles []RoleRef}`，handler 映射 `model.APIUser`。

**审计列表**（现状：6 个可选条件 + keyset 游标，字符串拼接）

单条静态查询：

```sql
-- name: ListAuditEvents :many
SELECT occurred_at, id, actor_type, actor_id, actor_name, action, target_type, target_id,
       outcome, reason_code, request_id, decision_id, metadata
FROM iam.audit_events
WHERE (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('outcome')::text IS NULL OR outcome = sqlc.narg('outcome'))
  AND (sqlc.narg('from_date')::date IS NULL OR occurred_at >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR occurred_at < (sqlc.narg('to_date')::date + 1))
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL
       OR (occurred_at, id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('page_size')::int;
```

风险与对策：多条件 `IS NULL OR` 可能让规划器退化。实施时对空筛选、单条件、全条件三种情况各跑一次
`EXPLAIN (ANALYZE, BUFFERS)` 与现状拼接 SQL 对比；若退化，退回"两个具名查询 + 包装层按有无筛选选择"
（仍然零拼接）。

**契约等价验证**（临时、不长期保留）：在 B4 分支内对同一数据库状态分别请求旧实现与新实现，排序键归一后
比对响应 JSON 字节一致；确认后删除对比代码，只保留现有集成测试断言。

## 五、任务依赖与进度

```
go-quality-gates（T1–T6）
        │
        ▼
B0 骨架 ─► B1 权限/角色 ─► B2 用户/凭据 ─► B3 会话 ─► B4 列表/审计 ─► B5 维护 ─► B6 清理 GORM
                                                        │
                                                        └─► docs/development/data-access.md + AGENTS 规范引用
```

进度记录在各实施 PR 描述中（本文件只维护任务与验收，不记录逐次执行日志）。

## 六、风险登记

| 风险 | 影响 | 对策 |
|---|---|---|
| planner 对 `narg` 形式退化 | 审计查询变慢 | B4 三个 EXPLAIN 基线；必要时拆两条具名查询 |
| 契约漂移（null/空数组/排序） | 前端渲染异常 | 现有集成测试 + B4 一次性对比；`model` 显式标注 `omitempty` 语义 |
| 生成物与 SQL 不同步 | 行为与代码不一致 | CI `sqlc diff` 门禁 |
| sqlc 版本升级改变生成结果 | 大 diff、review 困难 | 版本固定在 `sqlc.yaml` 注释与 Makefile；升级单独 PR |
| Windows CRLF 造成 diff 假阳性 | 本地噪声 | 以 LF 检出为准；`.gitattributes` 已 `eol=lf` |
| 迁移期事务边界混用（GORM `Transaction` 与 pgx `BeginFunc`） | 部分提交遗漏 | 每批次只允许一种边界；B6 后统一 pgx |
| 迁移期两套实现并存被遗忘 | 死代码 | 每批次同一 PR 内删除被替代的旧函数；B6 全量 grep 校验 |

## 七、完成定义（DoD）

1. Spec「验收标准」7 条全部可验证通过，并记录实测命令与输出。
2. `make lint-go`、`make vuln-go`、`make test`、`make integration`、E2E 全绿。
3. `docs/development/data-access.md` 完成，AGENTS.md 的 Project conventions 增加一行指向它。
4. `docs/plans/README.md` 状态更新为「已实施」，并记录实际批次与偏离。

## 八、工作量估算（供排期，不是承诺）

| 批次 | 乐观 | 说明 |
|---|---|---|
| B0 | 0.5 天 | 骨架 + 流程演练 |
| B1–B3 | 各 0.5 天 | 语句少、契约清晰 |
| B4 | 1.5–2 天 | 动态过滤重写 + JSON 出 SQL + 契约对比 |
| B5 | 0.5 天 | 维护与保留期清理 |
| B6 | 0.5 天 | 移除 GORM + 全量回归 |
| 文档与规范 | 0.5 天 | data-access.md + AGENTS |

合计约 **5–6 个工作日**（含自测与回归），分 6–7 个 PR 交付。

## 九、实施记录（as-built）

第二节至第八节保留计划原文，其中涉及“目标形态”的表述以本章与第十一节的实际结果为准。

| 批次 | 结果 | 与计划的差异 |
|---|---|---|
| B0 | PR #41 已合并 | 采用 `database/sql` 风味以便与 GORM 事务共用连接；`LockUsers` 改为单 id 查询由 Go 排序逐条加锁（数组形式会引入 `lib/pq`） |
| B1 | PR #42 已合并 | 实际拆成 14 条具名查询（原计划记 13 条：新增 `RolePermissionKeys`/`TouchRole`/`DeleteUserRoles`/`InsertUserRoles` 以便数组解耦）；数组参数统一用 jsonb 展开；封闭取值列（`iam.permissions.status`/`risk_level`）用 column override 映射到 `internal/enum` |
| B2 | PR #50 已合并 | 账号与凭据 12 条、配置管理员同步 6 条查询；`iam.users.status` 增加 column override；仓库函数签名不变 |
| B3 | PR #51 已合并 | `Queries` 包装类型删除，读账号/会话改自由函数；出参（`*bool`/`*time.Time`/`*uuid.UUID`）改为返回值；interval 参数改为微秒 bigint；`Session.ReauthenticatedAt` 改 `*time.Time` |
| B4 | PR #52 已合并 | 4 处 `jsonb_build_object` 与 `listJSON` 删除；列表改类型化行 + `model.API*` 投影；页内角色/权限用第二条查询批量读取；审计筛选改静态 SQL（空串即不筛选） |
| B5 | PR #53 已合并 | 保留期清理与恢复前后 8 条查询；advisory 锁统一走 `LockManagement` |
| B6 | PR #54 已合并 | 移除 `gorm.io/gorm`/`gorm.io/driver/postgres`；`repository.DB`（生成的 `DBTX`）统一连接与事务；`Store.Transaction` 取代 `db.Transaction`；日志从 GORM logger 换成 pgx `QueryTracer`；`gorm.ErrRecordNotFound` 全部并入 `sql.ErrNoRows`；未引入 42 方法的 `Store` 接口（见下） |

落地时补充的两条硬约束（已进 `data-access.md`）：

1. 生成查询与调用方的写入**必须共用同一事务**；新增集成测试 `TestGeneratedQueriesShareTheCallerTransaction` 用回滚可见性守住这一点。
2. 依赖事务范围的锁操作（`LockUsers`、`LockManagement`）在自动提交会话上会静默失效，因此加了 `requireTransaction` 守卫与对应测试。

顺带修掉的存量红灯：`migrations_integration_test.go`（按单基线断言）、`server_integration_test.go`（用 `offset` 断言游标分页接口）。

## 十、刻意保留的 gap（不视为遗漏）

- `sqlc vet` 的 `db-prepare`/`query-plan` 规则需要可连接的数据库；CI 目前没有 PostgreSQL 服务，故只在本地可用时手工跑，未进门禁。需要时再加 `services: postgres`。
- `emit_interface`、`:copyfrom`、`sqlc.embed` 暂未使用：当前不需要 mock 查询层、没有大批量插入场景、也没有需要嵌入的关联结构；为避免过度设计而保留。
- **未引入 42 方法的 `Store` 接口**：`service` 依赖 `*repository.Store` 与 `repository.DB`，两者都不含驱动类型；当前只有 PostgreSQL 一个实现，集成测试也直连真实库，接口没有第二个实现或替换需求。真需要换库时再抽接口，代价只是机械替换。
- **保留 `database/sql` 风味而非切 pgx/v5 风味**：切风味后生成的参数类型变成 pgx 专属，把驱动耦合进仓库签名；唯一收益（数组绑定）已被 jsonb 展开覆盖。
- **集成测试仍不在 CI**：需要 `services: postgres/redis` 与 pg_dump；本地用真实 PostgreSQL 16 + Redis 8 跑全套 13 个包。

## 十一、GORM 移除进度（唯一事实源）

**GORM 已移除**（B6，PR #54）：`go.mod` 不再依赖 `gorm.io/gorm` 与 `gorm.io/driver/postgres`，
仓库层只由 sqlc 生成查询、`database/sql` 适配与显式事务组成。下表保留逐文件的迁移轨迹。

| 文件 | 迁移前 | 批次 |
|---|---|---|
| `repository/authorization_queries.go` | `Table/Where` 权限读取 | B1 完成 |
| `repository/roles.go` | 角色 CRUD 与授权集合替换 | B1 完成 |
| `repository/locks.go` | 用户行锁 | B0 完成 |
| `repository/accounts.go` | `Table/Where/Updates` 账号读写 | B2 完成 |
| `repository/administrator.go` | 配置托管管理员同步（事务 + 行锁） | B2 完成 |
| `repository/queries.go` | `Queries` 包装类型：登录、用户/会话锁、续期与撤销 | B3 完成（文件已删除） |
| `repository/session_queries.go` | 登录失败计数、会话查询 | B3 完成 |
| `repository/lists.go` | 用户/角色/审计列表（含 `jsonb_build_object`、`listJSON`） | B4 完成 |
| `repository/maintenance.go` | 保留期清理、`set_config`、操作员恢复 | B5 完成 |
| `repository/store.go` | `gorm.Open`、`Transaction`、启动同步 | B6 完成（`stdlib.OpenDBFromPool` + `Store.Transaction`） |
| `repository/permissions.go` | `Table/Order/Find` 权限目录 | B6 完成（`ListPermissions` 查询） |
| `repository/logging.go` | GORM `logger.Interface` 适配 | B6 完成（pgx `QueryTracer`） |
| `repository/sqlc.go` | `queries(gorm)` 读取 `Statement.ConnPool` | B6 完成（`repository.DB` 接口） |
| `service/{service,roles,accounts,password,timezone}.go` | 事务入口与 `*gorm.DB` 参数 | B6 完成（`*sql.Tx` + `Store.Transaction`） |
| `authentication/{session,credentials}.go`、`handler/server.go`、`authorization/catalog.go` | `*gorm.DB` 参数、GORM 标签与错误哨兵 | B6 完成 |
| `go.mod` | `gorm.io/gorm`、`gorm.io/driver/postgres` | B6 完成（已删除） |

文档与图标已同步到最终状态：架构文档、后端结构规范、数据访问规范、角色管理文档、IAM README、README 双语致谢（删除 GORM 图标与单元格）、`docs/assets/README.md`。
