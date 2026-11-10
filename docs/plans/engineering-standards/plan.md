# 工程规范固化实施计划

## 方案

1. 新增 [数据库设计规范](../../development/database-design.md) 与
   [后端工程结构规范](../../development/backend-structure.md)，各自附依据链接。
2. `AGENTS.md` 只保留硬性规则与索引：补"决策要有证据"、"考虑大数据量与高并发"、
   "日志用简单英文"、"封闭取值用 enum 常量"、"每表每列必须有注释"。
3. 新增 `src/apps/iam/migrations/00002_table_comments.sql`：一次性补齐 9 张表全部业务列注释。
4. 新增 `src/apps/iam/internal/enum`：`UserStatus`、`PermissionStatus`、`RiskLevel`、
   `SessionState`、`ActorType`、`AuditOutcome`；列结构体字段改为这些类型。
5. 用常量替换 `repository`/`service`/`authentication`/`authorization` 中的取值字面量；
   SQL 片段内的取值保持字面量（由 DDL 的 CHECK 定义）。
6. 新增 `internal/model/vocabulary_test.go`：断言 HTTP `oneof` 标签与 enum 常量一致，
   因为标签是编译期字符串无法引用常量。
7. 全部日志消息改为简单英文；同步 IAM README 的结构说明与规范入口。
8. 删除死代码：`Store.SyncAdministrator`、`builtinRoleID`、`repository.UserStatus`；
   修正两个 `pgtypes.go` 的排版。

## 证据

- Go 目录布局：`go.dev/doc/modules/layout` 与 `golang-standards/project-layout`（`internal` 由编译器强制私有、
  `pkg` 仅放真实可复用代码）。
- 日志风格：Kubernetes `sig-instrumentation/logging.md`（结构化、消息风格、库不自行记录错误）
  与 OWASP Logging Cheat Sheet（必记安全事件、禁止记录敏感值）。
- 封闭取值：PostgreSQL `COMMENT` 文档、Crunchy Data 的 enum 与 CHECK 对比（CHECK 更易演进）。
- 命名与 SQL 风格：Google SQL Style Guide。

## 验证证据

- `make test`：脚本 40 项（39 通过、1 项 POSIX 专属跳过）、Go 全部包通过、Web 18 文件 59 用例通过。
- `make build`：Go 两个服务与 Next 生产构建成功。
- 迁移：`status` 输出 `{"current_version":2,"requires":2,"pending":false}` 形态（含注释迁移）。
- 注释覆盖率查询：`iam` schema 业务表 9/9、业务列 70/70 有注释（迁移账本表除外）。
- `make dev` + Web 代理登录：错误密码 401，正确密码 200；日志为英文短句。

## 未执行与边界

- 未做本轮之外的表结构调整（例如把深 OFFSET 分页改成键集分页）：`audit_events` 当前数据量下
  OFFSET 仍可接受，已在数据库规范中记录为增长后的改造点。
- 集成/E2E 需要隔离 PostgreSQL/Redis 或 Docker；本次以单测、构建与真实登录链路验收。

## 第二轮：分页与分层解耦

### 方案

1. 审计列表改为键集分页：`(occurred_at,id)` 不透明游标，只向前，不返回 `total`；
   仓库层多取一行判断是否还有更早事件，游标值不暴露给客户端解析。
2. 新增 `00003_audit_keyset_pagination.sql`：`(occurred_at,id)` 支撑排序与游标、
   删除被完全覆盖的单列索引，并补 `(outcome,occurred_at,id)` 供"结果 + 时间"筛选。
3. `users` / `roles` 保留 `limit`+`offset`：管理集合有界且页面需要跳页，
   偏移上限继续由入口绑定约束。
4. `service` 与 HTTP DTO 解耦：新增 `service/input.go`（`UserCreate`、`UserUpdate`、
   `UserRoles`、`RoleCreate`、`RoleUpdate`、`RoleGrants`、`AuditFilter`、`Identity`），
   handler 绑定 `model` DTO 后映射；`service` 不再 import `internal/model`。
5. Web 审计页改为游标导航（较新/更早），新增 `auditPage`/`auditNewer`/`auditOlder` 双语文案。
6. 顺带清理：删除未使用的 `resolvedThemeMode` 解构，消除新增的嵌套三元告警。

### 证据

- 键集分页优于深 OFFSET：PostgreSQL 官方文档 ORDER BY/LIMIT 语义与索引扫描代价；
  主流 API（GitHub GraphQL、Stripe）同样以不透明游标提供"向前翻页"，不提供随机跳页。
- `(occurred_at,id)` 复合索引与 `ORDER BY occurred_at DESC,id DESC` 的排序键一致，
  单列索引无法覆盖第二排序键（见 database-design.md 的索引条目）。

### 验证证据

- `make test`：脚本 39 通过/1 跳过、Go 全部包通过、Web 18 文件 59 用例通过。
- `gofmt -l` 对改动文件无输出；`go vet` 与 `go vet -tags integration` 均通过。
- `pnpm --dir src/apps/web lint`、`typecheck`、`test:unit` 通过。
- 迁移：`current_version 3 / pending false`。
- 真实链路：登录后 `GET /api/v1/iam/audit-events?limit=2` 返回 `next_cursor`，
  带游标继续请求只返回更早事件，非法游标返回 400。

### 判断与边界

- **没有做审计写入批量化**：审计必须与业务变更同事务提交，异步批量会引入"业务成功但审计缺失"
  或"审计提交而业务回滚"的窗口，直接破坏不可否认性。吞吐问题改为：审计表只追加、
  索引最小化、清理分批。这一条按"攻击前提"处理，不作为待办。
- 用户/角色列表未改键集：跳页是管理界面的真实需求，改用游标会牺牲可用性；
  有界集合配合偏移上限已足够。
