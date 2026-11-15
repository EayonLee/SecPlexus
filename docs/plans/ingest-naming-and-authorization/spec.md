# Ingest 命名对齐与统一鉴权

状态：待审核，未实施。范围是两件事：把 Ingest 的领域命名对齐行业标准，以及把散落各处的鉴权收敛成"IAM 数据驱动 + 服务薄执行点"。同时记录前几轮审查遗留的问题清单，避免散失。

## 背景

三个已暴露的问题：

1. **命名自造**。当前 `source` / `transport` / `parse rule` 与主流产品用词不一致；同一概念在代码里有四个名字（`parse_rules` 表、`normalizer.Rule`、`repository.ParseRule`、`ruleSpec`）。
2. **鉴权散落**。Console 在每个 handler 里自己调 `authorize`，Ingest 的管理路由只有一个未接线的守卫钩子；两处 router 都没有会话中间件。忘调一次 = 接口裸奔。
3. **特殊逻辑**。"这个接口不需要权限"如果写成服务里的分支，控制面就会随接口数量增长而扩散。

## 一、命名对齐：以 Splunk 为主，其余产品佐证

### 1.1 接入点 → `input`

| 产品 | 该配置实体叫什么 |
|---|---|
| **Splunk** | **input**（`inputs.conf`，每段一个数据输入） |
| Graylog | **input** |
| Fluent Bit | **input** |
| Vector | source |
| QRadar | log source |
| Sentinel | data connector |

**结论**：采用 **`input`**。四家里三家用这个词，且 Splunk 是我们对齐的主目标。当前 `source` 与 OCSF `src_endpoint`、normalizer 内部的"源字段 `source`"撞词，本身就有隐患。

迁移成本：`ingest.sources` 目前为空表，改名不涉及数据迁移。

### 1.2 传输方式 → `protocol`（原 `transport`）

Splunk 把输入机制叫 "input type"（TCP / UDP / HEC）。我们的三种取值 http / syslog / kafka 本质是**协议**，`protocol` 比 `transport` 更准确且不与 "input type（含参数）" 混淆。

### 1.3 解析规则 → `mapping`（原 parse rule / normalizer.Rule）

| 产品 | 该实体叫什么 |
|---|---|
| **Splunk** | **`sourcetype` + `props.conf` / `transforms.conf`**（后者负责字段变换） |
| Elastic | **ingest pipeline** |
| Wazuh | **decoder** |
| Vector | **transform** |

**结论**：采用 **`mapping`**（`ingest.mappings`、`normalizer.Mapping`）。理由：

- 它的职责就是把原始字段**映射**成 OCSF，名字即行为；
- `parser` 这个词已被第 1 层线格式解析占用（`internal/parser`），再用会撞；
- Splunk 的 `transforms.conf` 正是字段变换，语义一致。

编译产物统一叫 **`normalizer.Compiled`**（原 `normalizer.Engine`）："引擎"太泛，编译结果是它的事实。

### 1.4 事件里的私有键改名（消除与 OCSF 的混淆）

| 现键 | 新键 | 理由 |
|---|---|---|
| `metadata.spx.parse_status` | `metadata.spx.parse.outcome` | OCSF 自带 `status_id`，`parse_status` 容易被误当成它；Elastic 用 `event.outcome` 表达同类语义 |
| `metadata.spx.parse_error` | `metadata.spx.parse.error` | 与 `parse.outcome` 归到同一命名空间下 |
| `metadata.spx.source_id` | `metadata.spx.input_id` | 跟随实体改名 |

`metadata.spx` 明确声明为厂商扩展命名空间，OCSF 侧必填项（`category_uid`、`type_uid`、`activity_id`、`metadata.version`、`metadata.product`）不受影响。

### 1.5 改名总表

| 现 | 新 | 位置 |
|---|---|---|
| `internal/source` 包 | `internal/input` | 代码 |
| `source.Entry` | `input.Entry` | 代码 |
| `Entry.Transport` / `source.Transport` | `Entry.Protocol` / `input.Protocol` | 代码 |
| `source.Lookup` | `input.Lookup` | 代码 |
| `ingest.sources` 表 | `ingest.inputs` | 数据库 |
| `sources.transport` 列 | `inputs.protocol` | 数据库 |
| `ingest.parse_rules` 表 | `ingest.mappings` | 数据库 |
| `parse_rules.spec` 列 | `mappings.definition` | 数据库 |
| `normalizer.Rule` | `normalizer.Mapping` | 代码 |
| `normalizer.Engine` | `normalizer.Compiled` | 代码 |
| `repository.ParseRule` / `model.ParseRule` | `repository.Mapping` / `model.Mapping` | 代码 |
| `/api/v1/ingest/sources` | `/api/v1/ingest/inputs` | API |
| `/api/v1/ingest/parse-rules` | `/api/v1/ingest/mappings` | API |
| `connector_id` / `source_id` | `input_id` | 事件、路径、消费组 |

## 二、统一鉴权：IAM 数据驱动，服务只做薄执行

### 2.1 原则

**集中策略 + 薄执行点。** 这是 Kubernetes RBAC（策略在 API Server，组件声明 verbs）、AWS IAM（每个 API 一个 action，策略在数据里）、OPA/Envoy（策略在 OPA，Envoy 一次调用）的共同形态。

推论：

- **所有 API 默认经过 IAM**，只有三类显式白名单（见 2.4）；
- 服务里**没有**"这个接口不需要权限"的分支；需要登录即人人可做的操作，在 **IAM 数据**里表达为基线授予；
- 每个接口在**注册路由时声明所需动作**，业务代码完全不含鉴权逻辑。

### 2.2 权限数据模型（数据库可直接回答"谁能做什么"）

现有 `iam.permissions` / `iam.role_permissions` / `iam.user_roles` 已能回答"谁的权限是什么"。本次补齐三点：

1. **基线角色**：新增内置角色 `authenticated`，每个已登录用户自动拥有。凡"登录即可"的权限授予它，于是"不需要细粒度权限"变成**数据**而不是代码分支。
2. **动作目录即操作清单**：`iam.permissions` 的键是 `module.resource.action`，每条带 `description`、`risk_level`、`requires_recent_auth`、`status`。**"什么操作的权限是什么"因此直接查这张表**。
3. **动作与路由的一致性检查**：服务启动时校验"路由声明的每个动作都在权限目录中"，缺一个就拒绝启动（Prometheus、Envoy 都是启动期校验配置，避免运行期才发现漂移）。

**明确不做**：不建"接口 → 权限"的数据库映射表。API 到动作属于接口契约，随代码发布；放进数据库会让"某个接口到底要什么权限"变得不可静态审查，也不符合 AWS/K8s 的形态。数据库负责**谁拥有什么**，代码负责**接口要求什么**，两者在启动期对齐。

### 2.3 三种校验的层次

| 层次 | 判定者 | 存放位置 |
|---|---|---|
| 登录（会话） | IAM 认证模块（cookie + CSRF + 会话有效期） | IAM 数据 |
| 注册 | `iam.account.create` 动作 | 权限目录 + 角色授予 |
| 操作 | `module.resource.action` | 权限目录 + 角色授予 |
| 页面 | **不单独建模型**，由"页面要调用的接口权限"推导 | 前端读会话返回的权限集合渲染 |

页面不单独授权是刻意的：能打开页面但接口 403 是更差的体验。Sentinel / Splunk 都是按"用户能否执行该操作"来渲染入口。

### 2.4 白名单只有三类（显式登记，可审计）

| 类别 | 示例 | 不加鉴权的理由 |
|---|---|---|
| 健康探针 | `/health/ready` | 探针不带会话 |
| 认证入口 | `/api/v1/iam/login`、`/register` | 此时还没有会话 |
| 设备接入 | `/api/v1/ingest/events/{input_id}` | 设备持接入令牌，与用户身份无关 |

白名单在 `internal/router` 显式列出并附测试，**不允许第四种**。

### 2.5 执行点

```go
// 共享包提供两个中间件，服务不再自己写鉴权
middleware.Session(gateway)                       // 无有效会话 → 401
middleware.Require(gateway, "ingest.input.write")  // 无该动作 → 403
```

- IAM 不可达 → **503**，不降级放行（与 IAM 登录限流同语义）；
- 会话有效但缺权限 → **403**，与"未登录"的 401 区分开。

### 2.6 职责边界：鉴权逻辑全部在 `iamclient`，业务方零鉴权代码

Console 与 Ingest 是**业务方**，不是鉴权实现方。它们不应该：

- 解析会话凭据；
- 判断"这个接口要不要权限"；
- 决定 401 / 403 / 503 的语义；
- 在 handler 里调一次 `authorize`（现在的 Console 就是这么写的，每加一个接口都要记得调）。

这些全部下沉到 **`src/packages/go/iamclient`**，因为它已经拥有 IAM 的协议、超时、错误映射与重试语义。再加一个中间件构造器，业务方接线只剩两行：

```go
engine.Use(client.Session())                       // 无有效会话 → 401
group.POST("/inputs", client.Require("ingest.input.write"), admin.CreateInput) // 缺动作 → 403
```

`iamclient` 新增的公开面：

| 能力 | 作用 |
|---|---|
| `Session()` | 返回 gin 中间件：从请求取会话 → 调 IAM → 失败按 401/503 返回 → 成功把主体写入 context |
| `Require(action)` | 返回 gin 中间件：在会话基础上校验单个动作，缺权限 403 |
| `Actions()` / `ValidateActions(ctx, ...string)` | 启动期校验"路由声明的动作都在权限目录里"，缺项拒绝启动并列出 |
| 已存在的 `Authorize` RPC | 保持不变，中间件内部复用它 |

业务方的义务只有一条：**在注册路由时声明动作**。业务代码里不出现任何 `if` 判断权限。

### 2.7 审计：与鉴权共用一份目录，不另建词表

审计写入走 IAM（方案 B：控制面集中），业务方同样只做接线。设计要点：**不要为审计另立一套操作名**。

#### 一份目录，两种用途

| 事实 | 结论 |
|---|---|
| 权限键已是生成常量：`iam/internal/authorization/permissions.json` → `iamclient/permissions.gen.go` | 审计的 `action` 直接复用**同一个常量**，编译期校验，不写字符串 |
| 审计表 `iam.audit_events.action` 是 `text` | 它的取值 = 权限键，于是"什么操作"只有一个来源 |
| `outcome` 已是数据库闭集 `CHECK IN ('success','denied','failure')` | Go 侧用同名常量镜像，并加**词表对齐测试**（常量集合 == 数据库 CHECK 集合） |
| `target_type` 目前无约束 | 新增闭集：迁移加 `CHECK`，Go 侧镜像常量，同样加对齐测试 |

#### 业务方的接线只剩一个中间件

```go
group.POST("/inputs",
    client.Guard(iamclient.PermissionIngestInputWrite, iamclient.TargetInput),
    admin.CreateInput)
```

`Guard(action, targetType)` 一次完成三件事，业务代码里不出现任何鉴权或审计分支：

1. **会话**：无效 → 401；IAM 不可达 → 503；
2. **授权**：缺动作 → 403（并写 `outcome=denied` 的审计事实）；
3. **审计**：请求结束后按响应状态写事实——2xx → `success`、403 → `denied`、其余 → `failure`。

#### 字段来源固定，不让业务方拼

| 审计列 | 来源 |
|---|---|
| `action` | 生成常量（与权限键同一份） |
| `outcome` | 共享闭集常量，由中间件按响应状态映射 |
| `target_type` | 共享闭集常量，由路由声明 |
| `target_id` | 路由参数（`:id`），无参数时为空 |
| `actor_type` / `actor_id` / `actor_name` | 会话中间件写入 context 的主体，仅 `user` |
| `request_id` | 既有 `middleware.RequestID` |
| `decision_id` | IAM 授权决策返回，贯穿"授权 → 审计"两行记录 |
| `metadata` | 中间件只放技术字段（耗时、方法、路径），**不放请求体**；表上限 8KB |

#### 审计范围（避免写爆表）

| 操作 | 是否审计 | 理由 |
|---|---|---|
| 管理写操作（建/改/删接入点、换令牌、改规则） | **是** | 安全敏感，必须可追 |
| 授权拒绝（403） | **是**，`outcome=denied` | 拒绝本身就是安全信号 |
| 管理读操作（列表/详情） | 否 | 量大且无状态变更；确有需要再开 |
| 设备数据接入（每一条事件） | **否** | 5 万 EPS 下会把审计表写爆；接入点生命周期已覆盖 |

`reason_code` 复用既有的 `result` 码常量（`permission_denied`、`invalid_request` 等），不新增词表。

#### 两条明确约束

- **令牌明文永不入审计，也永不入日志**：字段里只允许 `input_id` 与摘要前缀，加测试断言。
- **审计写失败不静默**：高风险动作（建接入点、换令牌）审计写失败时返回 503，并用 `request_id` 让运维可追；此时业务可能已经生效，属已知限制（跨服务无法用一次事务保证），记录在案而不是假装原子。

### 2.8 落地顺序

1. 共享包新增 `middleware.Session` / `middleware.Require`；
2. IAM 增加基线角色与 ingest 权限目录条目；
3. Console 从"每 handler 自己 authorize"改为路由声明动作；
4. Ingest 管理路由接上守卫，设备路由保持令牌鉴权；
5. 启动期动作一致性检查；
6. 前端按会话返回的权限集合渲染菜单。

## 三、计划自审发现的缺口（实施前必须解决）

### 3.1 阻断级

| # | 缺口 | 影响 | 处理 |
|---|---|---|---|
| 1 | **已应用迁移无法回改** | 测试库与本地已跑过 `00001/00002`；直接改基线 SQL 不会重跑，schema 不会变，代码与库立刻不一致 | 改为**前向迁移** `00003_rename_to_inputs_and_mappings.sql`（`ALTER TABLE ... RENAME`、`RENAME COLUMN` 并保留 `COMMENT ON`），dev 库按此前进；不修改已应用迁移 |
| 2 | **写操作缺 CSRF 防护** | 会话是 cookie 型；管理面 `POST/PATCH/DELETE` 若无 CSRF 校验，可被跨站伪造 | 管理写路由统一挂 IAM 的 CSRF 校验件（复用 IAM 已有 CSRF key 与会话绑定），与 IAM 自身管理接口同语义 |
| 3 | **管理操作无审计** | 建接入点、换令牌属安全敏感操作，AGENTS.md 要求写入 `audit_events`；缺失会让"谁在什么时候换过令牌"不可追 | 每次管理写操作写审计事实（动作、目标 `input_id`/`mapping_id`、操作者、结果），令牌明文绝不入审计 |

### 3.2 重要

| # | 缺口 | 处理 |
|---|---|---|
| 4 | 管理 API 的调用方未定：web 直连 Ingest 还是经 Console | 按 AGENTS.md「Console 不是其他模块的必经网关」，**web 直连 Ingest 管理 API**，同源 + `withCredentials`；CORS 保持默认同源，不开放跨域 |
| 5 | 令牌明文可能进日志 | 加测试断言：创建/换令牌的日志与审计输出中不出现明文；日志字段只记 `input_id` 与摘要前缀 |
| 6 | 权限目录为空的新环境 | 启动期校验：`permissions.json` 同步后目录为空则拒绝启动，避免整站 403 |
| 7 | 改名后表/列注释丢失 | 前向迁移里同步 `COMMENT ON`，并在测试中断言注释存在 |
| 8 | 前端文案未走 i18n | 菜单与操作入口文案进 `next-intl` 目录（`en` / `zh-CN`），不硬编码 |

### 3.3 记录为已知限制（不阻塞）

| # | 事实 | 说明 |
|---|---|---|
| 9 | `protocol` 把"接收"与"消费"混在一个维度 | `http`/`syslog` 是入站接收，`kafka` 是拉取外部总线。当前用 `protocol` 能表达现状；若将来加入拉模式连接器，需要独立的 `mode`（inbound/pull）维度，届时再加字段 |
| 10 | 事件键改名会与历史 Kafka 数据不一致 | 当前无下游消费者，改名零成本；一旦有消费方，事件键变更必须走版本化 |

## 四、前几轮审查的遗留问题（一并登记，避免散失）

### 3.1 能力缺口（按价值排序）

| 缺口 | 现状 | 备注 |
|---|---|---|
| 指标端点 | `Stats()` 已实现但无调用方，无 `/metrics` | spec 第 6 条要求过 |
| 死信 topic + 重放 | 解析失败事件与正常事件同 topic | 无隔离、无重放 |
| schema 漂移哨兵 | 无 | AiSOC 用字段名哈希指纹存库并告警 |
| 拉模式连接器 + 游标 | 只有推送 | 大量设备只能拉不能推 |
| 多副本选主 | 隐含单实例 | AiSOC 用 Postgres 租约表 |
| 多行日志合并 | 无 | Java 堆栈会变成 N 条独立事件 |
| 编码转换 | spec 承诺 GBK→UTF-8，未实现 | 国产设备乱码 |
| 出站 TLS/SASL + 压缩 | 无 | 接不了生产 Kafka 集群 |
| 入站 syslog TLS | 无 | 明文接收 |

### 3.2 设计债

| 债务 | 说明 |
|---|---|
| Kafka 消费绕开批处理 | 两条发布路径，`Stats.Published` 不含 Kafka 来源 |
| `parse_rules` 与 `normalizer.Rule` 双名 | 本次命名统一即解决 |
| `handler.Server` 与 `router.Deps` 两套依赖 | 装配路径应唯一 |
| DB 配置与连接在 iam / ingest 各一份 | 应下沉到共享包 |
| 无集成测试的通道能力 | 只有 httptest，没有 `-tags=integration` 的真实 Kafka 用例 |
| CI 不跑集成测试 | 集成测试只在本地对着测试库跑过 |

### 3.3 明确不做

- 磁盘持久缓冲：参考实现（AiSOC）在 ingest 侧没有，用游标与背压代替；要做得先改 spec 核心取舍。
- 接口→权限的数据库映射表：见 2.2。
- 页面级独立权限模型：见 2.3。

## 验收

1. 代码、数据库、API、事件里不再出现 `source` / `transport` / `parse rule` 三个旧词；
2. 除三类白名单外，所有路由都经过 `middleware.Session`，且有测试证明缺失会话返回 401；
3. "登录即可"的操作在 IAM 数据库中由基线角色授予，服务代码中不存在相关分支；
4. 启动期校验路由声明的动作全部存在于权限目录；
5. 数据库查询可直接回答：某用户拥有哪些权限、某权限被哪些角色持有。

