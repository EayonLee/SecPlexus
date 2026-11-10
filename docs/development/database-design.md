# 数据库设计规范

本规范适用于 SecPlexus 全部模块的表结构、迁移与查询。它是评审依据，不是建议：
新表或改表在评审时必须逐条对照；没有对应条目的新问题，先补规范再写实现。

规则的依据来自 PostgreSQL 官方文档、Google SQL Style Guide、OWASP 与主流 Go 项目
（见文末[依据](#依据)），不是个人偏好。

## 1. 命名

| 对象 | 规则 | 示例 |
|---|---|---|
| schema | 与模块同名，小写下划线 | `iam`、`ingestor` |
| 表 | 复数、小写下划线、不用模块前缀（schema 已表达归属） | `users`、`user_roles`、`audit_events` |
| 列 | 单数、小写下划线 | `display_name`、`auth_epoch` |
| 主键 | 一律 `id` | `id` |
| 外键列 | `<被引用表单数>_id` | `user_id`、`role_id` |
| 布尔列 | 形容词或过去分词，不用 `is_` 前缀 | `configured`、`builtin`、`delegable` |
| 时间列 | `<动作>_at`，一律 `timestamptz` | `created_at`、`locked_until` |
| 数量列 | 名词 + 单位语义 | `failed_attempts` |
| 索引 | `<表>_<列或用途>` | `user_sessions_user_state` |
| 约束 | 交给 PostgreSQL 默认命名或 `<表>_<含义>` | `users_single_configured` |

禁止缩写猜测：`usr`、`org_id`（无组织模型时）、`dt` 不允许。列名不能与 SQL 保留字冲突，
必要时改语义（`permission_key` 而不是 `key`）。

## 2. 类型选择

- 标识：`uuid`，由服务端生成，不依赖数据库序列。理由：跨服务可预生成、可分片、不泄露规模。
- 时间：`timestamptz`。服务端统一 UTC，写入前调用 `.UTC()`；禁止 `timestamp` 无时区列。
- 金额/精确计数：`numeric`；分数/比率：`double precision`；次数与版本号：`bigint`/`integer` + `CHECK`。
- 文本：`text` + `CHECK(char_length(...))`，不用 `varchar(n)`（PostgreSQL 中二者性能相同，
  长度限制用 CHECK 表达更易演进）。
- 结构化扩展：`jsonb`，必须限制大小（`octet_length(x::text) <= N`）并禁止写入凭据。
- 归一化列：用于唯一性比较的列由 `GENERATED ALWAYS AS (lower(...)) STORED` 生成，
  避免写入方忘记维护、也避免两套真值。

## 3. 封闭取值（枚举）

**规则：任何一列如果取值是封闭集合，必须同时具备三件事。**

1. DDL 用 `CHECK (col IN (...))` 约束取值，并在注释里写清每个值的含义。
2. Go 侧在 `internal/enum` 定义类型化常量（`type UserStatus string`），列结构体字段使用该类型。
3. Go 代码的判断一律比较常量，不写字面量；HTTP 绑定标签的 `oneof` 由测试断言与常量集合一致。

示例：

```go
// internal/enum/enum.go
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)
```

**为什么不用 PostgreSQL 原生 `ENUM` 类型**：原生枚举新增值需要 `ALTER TYPE`，删除或重命名
值几乎无法回退，且与迁移顺序、锁和备份恢复强耦合。`text + CHECK` 可在一条迁移里增删值并
重建约束，网络类型解析也更简单。Crunchy Data 与多家生产团队的经验也是优先 CHECK 约束。

**为什么不用查找表**：取值集合由代码语义决定（例如 `success/denied/failure`），不是运营数据。
引入查找表会多一次连接、多一层缓存一致性，却换不到可配置性。

## 4. 注释

每个表、每个业务列都必须有注释。注释由迁移中的 `COMMENT ON` 维护（`00002_table_comments.sql`），
不要只写在建表语句上——已初始化的环境要靠前向迁移拿到注释。

注释写法：

- 一句话说清**业务含义 + 取值范围/单位 + 关键约束**，不重复字段名。
- 说明"谁能改、什么时候变"，而不是复述类型。
- 反例：`COMMENT ON COLUMN users.status IS '状态'`（没有信息量）。
- 正例：`'账号状态：active 可登录，disabled 停用并立即失效全部会话。'`
- 迁移账本等由第三方组件拥有的表不写注释，避免与组件升级冲突。

检查命令：

```sql
SELECT c.relname, obj_description(c.oid,'pg_class')
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='iam' AND c.relkind='r' ORDER BY 1;
```

## 5. 约束与关联

- 每张表必须有主键；连接表用复合主键，不额外加代理键。
- 外键必须显式声明删除语义，不允许默认 `NO ACTION` 蒙混：
  - `ON DELETE CASCADE`：从属对象，主体消失后无意义（`password_credentials`、`user_sessions`、`role_permissions`）。
  - `ON DELETE SET NULL`：仅用于"操作者/创建者"这类引用（`created_by`），保留主体。
  - 默认 `RESTRICT`：仍需先解除绑定才能删除（`user_roles.role_id`），由服务层给出可操作提示。
- 唯一性用唯一索引表达，并尽量用部分索引缩小范围：
  `CREATE UNIQUE INDEX users_single_configured ON iam.users(configured) WHERE configured;`
- 不变量放进 CHECK：`CHECK(idle_expires_at <= absolute_expires_at)`、`CHECK(auth_epoch > 0)`。
  数据库是最后一道防线，不能只靠应用层自觉。
- 审计类表在数据库层拒绝改写（`BEFORE UPDATE OR DELETE` 触发器），删除只能通过显式事务变量开闸。

## 6. 索引

- 只为真实查询建索引：等值条件在前、范围条件在后；排序方向与查询一致。
- 复合索引的最左前缀必须能被查询命中，否则拆成单列索引。
- 分页、清理、过期扫描必须有索引支撑（`absolute_expires_at`、`(occurred_at,id)`）。
- 键集分页的索引必须与排序键完全一致：`ORDER BY occurred_at DESC,id DESC` 对应
  `(occurred_at,id)` 复合索引；只建单列索引会退化成排序加扫描。
- 索引命名 `<表>_<列>`，用途型索引写用途（`user_sessions_user_state`）。
- 新增索引必须说明它服务哪条查询；没有查询证据的索引视为负债。

## 7. 大数据量、高并发与高可用

设计评审必须回答以下问题，答案写进 Plan：

| 关注点 | 要求 |
|---|---|
| 表增长 | 预计行数、日增、保留期；保留期策略必须可执行（分批清理，不整表 DELETE） |
| 分页 | 行数随时间增长的表用键集分页（`(occurred_at,id)` 游标），不用 OFFSET；有界管理集合可以用 OFFSET，但必须设上限 |
| 热点写入 | 单行热点（会话、计数）不要放在同一行；必要时拆表或用 Redis 承接 |
| 锁 | 低频管理写入统一用 `pg_advisory_xact_lock`，固定锁顺序，避免死锁 |
| 事务 | 事务内只做数据库操作；不在事务里调用外部 IO 或等待网络 |
| 连接 | 每模块独立连接池并设置上限；共享同一 PostgreSQL，不做第二套连接配置 |
| 读多写少 | 先加索引与缓存键前缀，不提前引入物化视图；引入需给出刷新代价 |
| 可用性 | 迁移必须可重复执行（幂等），失败不留半成品；破坏性变更单独计划并有回滚预案 |
| 安全 | 列级最小权限；审计只追加；密码、令牌、密钥永不入库明文 |

## 8. 迁移规范

- 已应用的迁移文件不可修改；任何结构变化都新增编号迁移。
- 基线可以合并（未发版阶段），但必须同步说明"已初始化的环境如何前向修正"。
- 迁移必须幂等：`CREATE ... IF NOT EXISTS`、`ADD COLUMN IF NOT EXISTS`、`DROP ... IF EXISTS`。
- 一个迁移只做一件事；注解写清"为什么"，不写"做了什么"（SQL 已经说明）。
- 模块之间不跨 schema 建外键；跨模块协作走 API（见 `pnpm catalogs:check` 的迁移边界检查）。
- 破坏性变更（删列、改类型、删表）需要：备份方案、停机窗口或在线迁移步骤、验证查询。

### 分页的落地做法（IAM 现状）

| 表 | 方式 | 依据 |
|---|---|---|
| `audit_events` | `(occurred_at,id)` 键集分页，游标不透明，只向前 | 行数随安全事件线性增长，无法预知上限；深 OFFSET 会随历史线性变慢 |
| `users` / `roles` | `limit`+`offset`，偏移上限由入口绑定约束 | 管理集合有界，页面需要随机跳页；限额把最坏查询固定在可控范围 |

键集分页不返回 `total`：计数需要额外全表聚合，而"有多少条历史"对审计使用者没有行动价值，
真正需要的是"继续往更早看"。

## 9. 评审清单

- [ ] 表名、列名符合第 1 节
- [ ] 类型符合第 2 节；时间列是 `timestamptz`
- [ ] 封闭取值有 CHECK + `internal/enum` 常量 + 无字面量比较
- [ ] 表与每个业务列都有 `COMMENT ON`
- [ ] 外键删除语义明确；不变量落到 CHECK
- [ ] 新增索引能说清服务哪条查询
- [ ] 回答了第 7 节的增长、分页、锁、事务、连接问题
- [ ] 迁移幂等、可重复执行、有前向修正路径

## 依据

- [PostgreSQL COMMENT](https://www.postgresql.org/docs/current/sql-comment.html)：`COMMENT ON` 的官方语义与查询方式。
- [Enums vs Check Constraints in Postgres（Crunchy Data）](https://www.crunchydata.com/blog/enums-vs-check-constraints-in-postgres)：CHECK 约束比原生枚举更易演进。
- [PostgreSQL ENUM 与 CHECK 选择（Stack Overflow 讨论）](https://stackoverflow.com/questions/10923213/postgres-enum-data-type-or-check-constraint)
- [Google SQL Style Guide](https://sqlstyle.guide/)：命名、可读性与列顺序约定。
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)：审计事件与敏感字段禁止记录。
- [golang-standards/project-layout](https://github.com/golang-standards/project-layout)：模块内 `internal` 边界与私有代码约定。
