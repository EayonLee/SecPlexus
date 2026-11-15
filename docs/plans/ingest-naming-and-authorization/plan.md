# 实施与验收

依据 [spec.md](spec.md)。按测试先行推进；命名是纯重构，用"现有行为不变"的测试作为护栏。

## 依赖关系

```
共享中间件（Session/Require） ──┬── Console 改造 ──┐
                               └── Ingest 改造 ──┼── 启动期动作校验 ── 前端菜单
IAM 基线角色 + 权限目录 ────────────────────────────┘
命名统一（输入/映射）── 独立，可先行
```

命名统一不依赖鉴权，先做可以让后面的路由声明用上新名字。

## 第一组：命名统一（纯重构）

- [ ] 失败测试先行：断言代码中不再出现旧标识符——`grep -r "parse_rules\|ParseRule\|source.Entry\|TransportHTTP" src/apps/ingest` 为空；这条测试用脚本实现并在 CI 跑。
- [ ] **前向迁移** `00003`：`ALTER TABLE ingest.sources RENAME TO inputs`、`RENAME COLUMN transport TO protocol`、`parse_rules → mappings`、`spec → definition`、`sources.parse_rule_id → mappings.mapping_id`（外键与索引随之改名），并同步 `COMMENT ON`；**不修改已应用的 00001/00002**。
- [ ] 迁移 `ingest.sources` → `ingest.inputs`、`sources.transport` → `inputs.protocol`、`ingest.parse_rules` → `ingest.mappings`、`parse_rules.spec` → `mappings.definition`。表为空，直接改基线迁移（项目未发版，历史迁移可合并）。
- [ ] 代码改名：`internal/source` → `internal/input`；`Entry`；`Protocol`；`Lookup`；`normalizer.Rule` → `normalizer.Mapping`；`normalizer.Engine` → `normalizer.Compiled`；`repository/model` 的 `ParseRule` → `Mapping`。
- [ ] API 改名：`/api/v1/ingest/inputs`、`/api/v1/ingest/mappings`；路径参数 `{input_id}`；消费组 `spx-ingest-<input_id>`。
- [ ] 事件键改名：`metadata.spx.parse.outcome`、`metadata.spx.parse.error`、`metadata.spx.input_id`；同步更新 spec 与语料断言。
- [ ] 术语表与 spec 里的旧词清理（`docs/plans/ingest-and-normalization/spec.md`）。

## 第二组：iamclient 承载全部鉴权逻辑

- [ ] 失败测试先行：无会话凭据 → 401；会话有效但缺动作 → 403；IAM 不可达 → 503（不放行、不误判为 401）。
- [ ] `iamclient.Session()`：gin 中间件，取会话并调用既有 `Authorize` RPC；成功把主体（用户、会话标识、权限集合）写入 context，供业务方读取展示用信息。
- [ ] `iamclient.Require(action)`：在会话基础上校验单个动作。
- [ ] `iamclient.ValidateActions(ctx, actions...)`：启动期拿权限目录比对，缺动作则拒绝启动并列出缺失键。
- [ ] 复用既有超时、错误映射与日志约定，不新建第二套鉴权栈。

## 第三组：IAM 数据侧

- [ ] 失败测试先行（集成）：基线角色 `authenticated` 存在且不可删除；撤销其某项权限后，原先"登录即可"的操作返回 403。
- [ ] 迁移增加基线角色（标记 `builtin`），每个已登录用户自动获得其权限。
- [ ] 权限键遵循既有 `module.resource.action` 三段格式，并与 flat-authorization 计划已有的键风格一致；新增 `ingest.input.read/write`、`ingest.mapping.read/write`。
- [ ] 目录权威来源仍是代码里的 `permissions.json`，启动时同步进 `iam.permissions`；同步完成后校验目录非空，空目录拒绝启动（否则新环境所有请求都会 403）。
- [ ] 基线角色权限变更走数据库，不在服务代码里写分支；提供"某用户拥有哪些权限"的查询路径（管理面展示用）。
## 第四组：业务方接线（Console 与 Ingest）

- [ ] 失败测试先行：两服务所有受保护路由在无会话时返回 401（表驱动，逐路由覆盖）。
- [ ] **Console 重构**：删除 `handler/server.go` 里每个 handler 的 `authorize` 调用与各 handler 的鉴权参数，改为路由注册时声明动作（`client.Require(...)`）。这是"业务方零鉴权代码"的关键一步。
- [ ] 管理写路由挂 CSRF 校验（复用 IAM 的 CSRF key 与会话绑定），与 IAM 自身管理接口同语义。
- [ ] IAM 暴露审计写入接口 `POST /api/v1/iam/audit-events`（服务凭证鉴权），字段与 `iam.audit_events` 一一对应；不接受业务方提交 `actor`，主体由 IAM 从会话解析。
- [ ] 迁移把 `audit_events.target_type` 收紧为闭集（`CHECK`），Go 侧在 `iamclient` 镜像常量，并加**词表对齐测试**：常量集合等于数据库 CHECK 集合。
- [ ] `iamclient.Guard(action, targetType)`：一个中间件完成会话、授权、审计三件事；`outcome` 按响应状态映射（2xx/403/其余），`decision_id` 从授权决策透传，`request_id` 取既有中间件。
- [ ] 业务方不再拼审计字段：`action` 用生成常量、`target_type` 用闭集常量、`target_id` 取路由参数；中间件不采集请求体。
- [ ] 失败语义：审计写失败不静默——高风险动作返回 503 且记 Error，业务可能已生效，作为已知限制写入 spec。
- [ ] 测试断言：日志与审计中不出现令牌明文；`denied` 与 `failure` 的 `outcome` 取值正确；`target_type` 非法值被拒绝。
- [ ] 断言改名后表与列注释仍存在。
- [ ] **Ingest 重构**：管理路由挂 `client.Session()` + 按路由 `Require`；设备推送路由保持 Bearer 令牌鉴权，不经过 IAM。
- [ ] 白名单在各自 `internal/router` 显式登记（健康探针、IAM 登录注册、设备接入），并有测试证明其余路由都需要会话。
- [ ] 两服务启动期调用 `client.ValidateActions`，动作目录漂移即拒绝启动。
- [ ] 审计范围按 spec 2.7 执行：只审计管理写操作与授权拒绝；读操作与设备数据接入不写审计（附测试证明设备推送不产生审计行）。
- [ ] `reason_code` 使用既有 `result` 码常量，不新增词表。
- [ ] Console 的 SSE 路由（`/api/v1/console/events`）同样挂会话守卫：长连接在握手阶段校验，失败返回 401 而非建立空流。
- [ ] 部署顺序：`00003` 改名迁移先于新版本代码上线（expand 阶段兼容旧名，contract 阶段移除旧名），在 plan 记录两阶段执行顺序。


- [ ] 失败测试先行（集成）：基线角色 `authenticated` 存在，且每个已登录用户自动获得其权限；移除该角色权限后，原先"登录即可"的操作返回 403。
- [ ] 迁移增加基线角色，并在角色表中标记 `builtin`、不可删除。
- [ ] 权限目录新增 Ingest 动作：`ingest.input.read` / `ingest.input.write` / `ingest.mapping.read` / `ingest.mapping.write`，`read` 授予基线角色，`write` 由管理员角色持有。
- [ ] 权限目录同步逻辑沿用现有 `permissions.json` 权威来源，不新增第二份清单。

## 第五组：前端

- [ ] 会话接口返回权限集合；前端据此渲染导航与操作入口，不再按页面写死可见性。
- [ ] 失败测试（web）：无 `ingest.input.write` 时隐藏"新建接入点"，直接访问路由显示无权限而非空白页。

## 验证入口

```sh
go test ./src/...
go test -tags=integration ./src/apps/ingest/... ./src/apps/iam/...   # 需 SPX_TEST_POSTGRES_URL
pnpm catalogs:check
make build
gofmt -l src/
go vet ./src/...
```

## 验收证据

实施后在此记录。未完成前不得标记完成。

## 风险与约束

- 改名会动 HTTP 路径与事件键：当前无外部消费方，是最后的低成本窗口。
- 基线角色让"登录即可"的操作依赖 IAM 数据：若基线角色被误删，所有基础读操作会 403。迁移需保证该角色不可删除，并在启动期校验其存在。
- 会话中间件使每个请求多一次 IAM 调用：需评估对 5 万 EPS 接入路径的影响。**设备推送不经过 IAM**（令牌鉴权），所以热路径不受影响；管理面流量低。
