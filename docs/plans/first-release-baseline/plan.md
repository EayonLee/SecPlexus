# 首次发行基线实施计划

日期：2026-11-28。配置、运行链路及首版表结构已实施。生产部署和持续容量仍需验收。 [交付规格](spec.md)

## 复审结果

P1 涉及启动阻断或数据缺口。P2 是已定位的行为、设计或维护问题。标为「需实测」的项目已有开销或约束证据，是否调整取决于结果。

| 模块 | 等级 | 已确认的现状 | 处理与批次 |
| --- | --- | --- | --- |
| Compose | P1 | 当前清单只编排 Web、IAM、Console、PostgreSQL、Redis 和 IAM 迁移。Web、Console 没有就绪探针。 [开发 Compose](../../../deploy/compose/compose.yaml#L88) | A、E。补齐 Ingest、Kafka、ClickHouse 和一次性初始化，整栈就绪时验证 Web 与关键 API。保留开发时单独启动组件的入口。 |
| Compose | P2 | 已实测直接指定 `-f` 时构建上下文解析为 `deploy/compose`。当前 Dockerfile 路径需要从仓库根目录解析，`make compose-up` 通过 `--project-directory` 指定该目录。 [启动脚本](../../../scripts/compose.mjs#L20) | A、E。明确并持续验证根入口，把直接调用 Compose 所需的路径参数写入部署文档。 |
| Web 构建 | P1 | 隔离 Compose 实跑时，Go 镜像构建完成，Web 在规则测试的 TypeScript 检查失败。Web 的类型检查覆盖测试文件，Docker 构建阶段没有把该测试依赖的跨目录资料带进去。 [Dockerfile](../../../deploy/docker/web.Dockerfile#L8) · [验证测试](../../../src/apps/web/tests/unit/ingest-rule-validation.test.ts#L3) | A。容器应用构建检查应用自身的实际依赖。测试在仓库检查中继续检查类型并运行，字段目录保持一处维护。用原启动入口重新构建端到端验收。 |
| Web、IAM 可信入口 | P1 | Web 已有凭据验证后转发来源的代码。保留的 Compose 没有向 Web 注入 `SPX_WEB_EDGE_TOKEN`，也没有正式入口的配置示例。IAM 登录限流取 `c.ClientIP()`，缺这条链时 Web 同源请求共用代理来源，审计也会记录代理。 [Web 转发](../../../src/apps/web/src/app/api/v1/%5Bmodule%5D/%5B...path%5D/route.ts#L41) · [Compose](../../../deploy/compose/compose.yaml#L125) · [IAM 路由](../../../src/apps/iam/internal/router/routes.go#L49) | A、E。贯通已有信任机制。只测试两种实际用法，环回开发与正式反代入口。两客户端来源独立，未授权的来源声明不参与限流和审计。参考 [GitLab 的可信代理配置](https://docs.gitlab.com/omnibus/settings/nginx/#configure-gitlab-trusted-proxies-and-nginx-real_ip-module)。 |
| Web 请求边界 | P2 | Web 对 IAM、Console 的非读取请求先调用 `arrayBuffer()`，该分支在缓冲前没有应用级请求体限制。IAM 对登录的 2 KiB 限制位于后端，无法限制 Web 先分配的内存。 [Web 转发](../../../src/apps/web/src/app/api/v1/%5Bmodule%5D/%5B...path%5D/route.ts#L57) · [登录限制](../../../src/apps/iam/internal/router/routes.go#L58) | A、E。用读取上限保护 Web 的缓冲路径，保留 IAM 原校验及 Ingest 流式路径。验证缺少 `Content-Length` 的请求也受限制，超限返回 413。参考 [Grafana Loki 的有界读取](https://github.com/grafana/loki/blob/main/pkg/loghttp/push/push.go)和 [NGINX 请求大小限制](https://nginx.org/en/docs/http/ngx_http_core_module.html#client_max_body_size)。 |
| ClickHouse Event 留存 | P1 | 当前按事件发生时间计算 TTL，历史补采的 Event 也可能被自动删除。配置默认值为 `0`，启用现有存储时却要求正数。实库检查还要求 TTL 存在，单删 DDL 会让检查失败。技术架构也写着按天过期。 [表模板](../../../src/apps/ingest/migrations/clickhouse/00001_events.sql.tmpl#L6) · [配置校验](../../../src/apps/ingest/internal/config/config.go#L253) · [实库检查](../../../src/apps/ingest/internal/repository/event/store.go#L209) · [技术架构](../../design/architecture/02-technical-architecture.md#L373) | B、C、E。一起删除配置、执行路径和文档。新表固定 UTC 月分区，检查拒绝破坏永久保留的 TTL 漂移。用真实 ClickHouse 验证历史补采事件仍能回查。 |
| Kafka 消息留存 | P1 | 当前检查只要求 `retention.ms` 至少 24 小时或 `-1`。按字节清理或同键压缩的主题可能被放行。发布器使用分区键，独立事件可共享一个键。 [预检](../../../src/apps/ingest/internal/connection/kafka/preflight.go#L67) · [发布器](../../../src/apps/ingest/internal/publish/kafka.go#L106) | A、C、E。默认 7 天且可配置。对平台自有业务主题校验有效时间、清理策略和容量上限，用真实 broker 验证。现有集群发生提前清理仍需实际位移证据。 |
| 备份恢复 | P2 | 当前 ClickHouse 集成用例已验证恢复表和迁移账本，未验证用与备份对应的消费位移恢复后继续写入。 [ClickHouse 恢复用例](../../../src/apps/ingest/internal/consumer/worker_integration_test.go#L391) | E。让首版恢复验收覆盖受控停止写入确认、备份、明确的位移起点和回查。记录缺口与实际恢复时间。 |
| Ingest | P2 | `output.kafka` 与 `archive` 混用连接、认证、主题和存储策略。发布者与写入器共用 Kafka 安全配置。外部接入认证仍是部署级单套。配置加载还让外部接入协议影响事件存储的离线检查。 [配置](../../../src/apps/ingest/config.yaml) · [存储装配](../../../src/apps/ingest/internal/bootstrap/store_events.go) | A。拆到共享连接、角色认证、业务资源三处，按命令校验。内置替用户派生，外接明确填写。 |
| Ingest | P2 | 规则预览在缺少目录修订时走宽松编译，准备写入和运行分别有历史形状容忍。Web 将 `legacySharedPaths` 加回字段选择。 [规则编译](../../../src/apps/ingest/internal/normalizer/definition.go) · [规则编辑器](../../../src/apps/web/src/spx/pages/mixed-rule-editor.tsx) | B、D。删除试制兼容路径。完整草稿试算、保存和运行复用同一结构与标准。提取预览继续支持编辑中尚未补全映射的状态。 |
| Web、Ingest 日志样本 | P2 | 数据接入的新建和编辑试算按换行切割样本。按当前切分逻辑，用一条合成多行 JSON 核对后得到 4 条无效记录。同一文本放在规则向导的一个样本框内会被作为一条提交。 [接入试算](../../../src/apps/web/src/spx/pages/ingest-input-controls.tsx#L50) · [规则样本](../../../src/apps/web/src/spx/pages/mixed-rule-editor.tsx#L257) | D。沿用已有样本输入组件和样本数组，每个输入框对应一条。用同一条多行 JSON、单行 CEF 和连续多个样本验证总数、错误索引与预览。参考 [OpenSearch 的样本数组试算](https://docs.opensearch.org/latest/ingest-pipelines/simulate-ingest/)。 |
| Web、Ingest 绑定规则 | P2 | 试算用命中分支数判定通过，服务端把带分支 ID 的部分解析计为命中。试算在浏览器按规则 ID 和版本区分，绑定请求只提交 ID 并可用到另一修订。详情加载失败时现有按钮已拦住提交，但界面会显示未绑定。 [试算判定](../../../src/apps/web/src/spx/pages/ingest-input-controls.tsx#L55) · [预览响应](../../../src/apps/ingest/internal/handler/admin.go#L405) · [请求结构](../../../src/apps/ingest/internal/model/input.go#L51) · [绑定界面](../../../src/apps/web/src/spx/pages/ingest-input-wizard.tsx#L359) | B、D。按每条样本的解析结果确认，沿用规则修订号核对绑定时的版本。区分加载、失败和明确不绑定，回归详情失败和规则并发变化。 |
| Ingest | P2 | handler 同时编排验证、修订读取和持久化。`mappings` 表示整条解析规则又容易和字段映射混淆。旧的全量读仓储函数当前只剩测试调用。 [管理处理器](../../../src/apps/ingest/internal/handler/admin.go) · [输入仓储](../../../src/apps/ingest/internal/repository/inputs.go) · [规则仓储](../../../src/apps/ingest/internal/repository/mappings.go) | B。按用例拆分，统一 `parsing_rule`，删仅被测试调用的旧路径。业务测试改走首版列表或详情接口。 |
| Ingest | P2 · 需实测 | 列表已经分页，规则列表仍展开候选行的 JSON 分支并计算全量总数。接入刷新每轮读取接入和规则定义，编译缓存随刷新重建，只能在单轮复用。 [列表 SQL](../../../src/apps/ingest/internal/repository/queries/management_lists.sql) · [加载器](../../../src/apps/ingest/internal/repository/input_loader.go) | B、F。用类型化摘要支持已存在的筛选。配置刷新按确定的修订或摘要复用不可变编译结果，约束缓存量和失效条件，先测重复编译与读放大。 |
| Ingest | P2 · 需实测 | 外部 Kafka 批次按分区使用 PostgreSQL 事务和行锁锁定规则。边界历史随规则切换增长。Syslog 已修复停用连接回收，TCP 接收仍按连接创建读取协程，入口未设空闲超时或应用级连接数上限。 [Kafka 执行边界](../../../src/apps/ingest/internal/repository/kafka_execution.go) · [Syslog 连接](../../../src/apps/ingest/internal/channel/syslogchannel/channel.go) | C、F。保留重读确定性，实测批次锁和追赶成本。定义边界及引用修订的清理窗口，补连接预算与受控拒收。 |
| Ingest | P2 | 接入管理状态混有配置、内存快照和 Redis 实例确认。确认失败会撤下注册表，影响该进程的接入。审计 outbox 的队首若持续被 IAM 拒绝，后续记录无法推进。 [接入快照](../../../src/apps/ingest/internal/bootstrap/sources.go) · [审计投递](../../../src/apps/ingest/internal/repository/admin_store.go) | C。公开状态语义和停止受理条件。验证 Redis 故障后的恢复。将确定拒绝记录为可修复的隔离状态，临时故障继续重试。 |
| IAM | P2 | 正常迁移可同步并接管同名账号。审计入口直接持有数据库句柄，权限目录仍有手写业务 SQL。`maintain` 每次清理有界批次，当前 Compose 未调度。 [管理员同步](../../../src/apps/iam/internal/repository/administrator.go) · [审计入口](../../../src/apps/iam/internal/handler/server.go) · [维护](../../../src/apps/iam/internal/repository/maintenance.go) | B、E。区分初始化、迁移和托管管理员维护，收敛审计用例及 SQL。给现有维护命令安排可核对的调度。 |
| IAM | P2 · 需实测 | 常规鉴权锁用户和会话行，执行续期写入。Console 长连接也触发该路径。 [会话实现](../../../src/apps/iam/internal/authentication/session.go) · [鉴权 SQL](../../../src/apps/iam/internal/repository/queries/sessions.sql) | F。分别测普通鉴权和真实撤权场景的等待、写入与 WAL。优化要保留账号、会话与到期保护。 |
| Console | P2 | `/context` 用于诊断授权，和试制 `/events` 共用 `console.events.read`。仓库中的事件流调用只有测试，Console 就绪处理器始终返回 `ready: true`。 [Console 路由](../../../src/apps/console/internal/router/routes.go#L14) · [就绪处理器](../../../src/apps/console/internal/handler/server.go#L22) · [流的验收脚本](../../../src/apps/web/tests/e2e/run.mjs#L467) | C。入口按真实用途划分权限。核实调用方后清理试制流及专用配置，保留真实授权与撤权测试。区分可存活和可执行业务。 |
| Web | P2 | 审计结果按用户时区显示，筛选却以无时区日期传入 PostgreSQL。规则工作区集中处理表单、状态和试算，已有通用组件可复用。 [审计页面](../../../src/apps/web/src/spx/pages/audit.tsx) · [日期 SQL](../../../src/apps/iam/internal/repository/queries/lists.sql) · [规则页面](../../../src/apps/web/src/spx/pages/mixed-rule-editor.tsx) | D。按有效时区形成 UTC 半开区间。按功能收拢业务代码，用模板与实际使用任务验收。 |
| 共享工程与文档 | P2 | 配置指南仍禁止共享 `SPX_KAFKA_*`，与首版共用依赖决策冲突。ClickHouse 投影的列注释仍用「接入点标识」。目录正文允许按需建迁移，检查单口径不同。旧的 Kafka 3.9.0 测试版本已换为 4.2.2。 [配置指南](../../development/configuration.md#L262) · [投影](../../../src/apps/ingest/internal/repository/event/projection.json#L76) · [后端指南](../../development/backend-structure.md) · [Kafka 测试镜像](../../../tests/integration/kafka-container.mjs#L31) | A、B、F。确认已有改动以真实入口为准。仅收口需要同步维护的定义，以部署、源码、测试和运行说明四处一致为验收。 |

当前代码已具备 Kafka 重读身份和规则修订追溯、服务端管理分页、Syslog 停用连接回收、ClickHouse 集成测试配置。它们作为回归基线。容量是否达到交付要求仍以本计划的实测为准。 [Kafka 重启测试](../../../src/apps/ingest/internal/channel/kafkachannel/delivery_test.go) · [规则列表](../../../src/apps/ingest/internal/repository/queries/management_lists.sql) · [集成任务](../../../.github/workflows/ci.yml)

## 前轮要求与验收入口

专项交互继续由对应规格定义。下表列出本次跨模块整改可能改变的行为，保留已有测试证据，并随改动复验。

| 范围 | 保留并复验的行为 | 现有入口 |
| --- | --- | --- |
| 解析规则基础配置 | 事件类型和解析类型同排，可达步骤整段可点且居中，必填标记一致，空控件仅失焦时保持普通状态。名称、厂商、产品、描述共用 Unicode 长度和字符用例，名称唯一。键值对保留字段分隔符、键值分隔符、引号和提示。描述、正则与日志样本默认同高，分隔符类型只有一个分隔符设置。样本底部追加、计数到 20、三行后滚动，格式错误逐条定位。 | [规则字段规格](../ingest-rule-field-workflow/spec.md#四步操作) · [字段校验规格](../ingest-rule-validation/spec.md) |
| 提取、映射、预览 | 后续步骤右侧样本约三分之一宽，只读展示。重算由解析设置或字段处理变更触发。窄抽屉支持多行添加源字段，裁剪与解码各有独立入口和预览，裁剪含去除首尾空白、前缀和后缀。映射动作集中在映射页。平台字段名称、路径、类型及源字段对齐，末尾新增。必配行常驻，常用行可删，提交时定位冲突或遗漏。默认值独立列、枚举按目标字段显示名称、未设置留空。HTTP 状态码按整数值域及常用值，接收时间用一致控件。结果按样本对照，错误回到对应字段。 | [字段交互与实际验收](../ingest-rule-field-workflow/plan.md#字段操作与预览布局) |
| 规则详情、列表、系统设置 | 创建、编辑、详情共用四步。规则无导出，列表无版本列，已有四条内置试制规则删除的结果由空库启动验证。规则支持搜名称、描述、厂商、产品，并按事件类型筛选。数据接入列联动过滤，接入支持搜名称、厂商、产品，并按解析规则筛选。两页默认 20 条。表头、数据、操作列左对齐。共享搜索下拉与普通筛选同规格，弹层打开后搜索框为空、选项单行省略且支持键盘。系统设置四个子页使用 `/settings/*`，面包屑父项不链接，角色详情含权限，同规格抽屉。 | [列表与详情](../ingest-rule-field-workflow/spec.md#规则详情与管理列表) · [系统设置](../management-pages-consistency/plan.md#系统设置模板对齐) |
| 字段、术语、目录 | OCSF `1.10.0` 固定上游，平台契约 `v1`。复核当前 11 个可选事件类的公共核心行、类必配约束、常用行、枚举和值域，基础事件在一般选择器中隐藏。翻译按职责统一，清理仅标点不同的匹配别名。Ingest README 用短表说明契约源、生成目录、Go 加载、生成命令及保留上游 LICENSE、NOTICE 的原因。命名测试当前已在 `internal/`，以契约检查和本次多层残留检索收尾。 | [已锁定目录](../../../src/apps/ingest/schema/events/event-contract.json#L1) · [目录维护入口](../../../src/apps/ingest/README.md#event-字段与存储设计) |
| 可信操作、升级与恢复 | 可信入口、样本临时性、规则试算与生效、原始样本权限、Kafka 重读、恢复和容量继续验收。部署按源码和 Compose，首版首装从空库开始。保留既有 IAM 授权与撤权、新用户、角色、审计、时区偏好和业务页读写边界。 | [商业交付实测记录](../commercial-delivery/plan.md#实施记录) · [时区规格](../timezone-and-user-preference/spec.md) |

## 实施批次

A 先确定配置和 Compose 入口，再以 B 定下首版数据与接口。C、D 在相同契约上完成后合并验收，E 用全栈 Compose 检查启动和恢复。F 的基线采样在改动热点之前启动，交付验证和规范复核在收尾执行。

### A. 共享依赖、镜像构建与启动预检

1. 冻结根模板和模块 YAML 的新键名。配置结构使用 `postgresql`、`redis`、`clickhouse`、`kafka`，Ingest 的主题、消费组和写入角色落到 `events`。删除 `archive`、`output.kafka`、`SPX_INGEST_ARCHIVE_*`、旧出站 Kafka 连接键和口令变量名转发。同步 Compose、脚本、探针、日志和测试。从实际调用方一起收口，删掉 `archive` 命令别名、`--with-archive` 和旧配置检查命令，事件写入入口沿用 `store-events`。
2. 收口实际重复的配置来源和按角色环境注入。公开域名从有效 Web origin 派生，内置依赖地址、账号与资源身份由部署记录确定。核对 CSRF 密钥生成与 Kafka 协议格式。发布和写入各用权限匹配的 Kafka 身份，外部接入按来源认证连接。保留 PostgreSQL 和 ClickHouse 的独立迁移账号。服务名单按各入口现有职责维护，已证实有单一权威的定义直接复用。
3. 初始化 Kafka 平台业务主题时，从 `SPX_KAFKA_RETENTION_DAYS` 推导毫秒数，显式应用按时间删除的策略。默认 7 天，验证自定义天数、零、负数、非法字符和换算溢出。内置与外接预检都回读 `retention.ms`、`cleanup.policy`、`retention.bytes`。已有主题经明确的运维操作修改，普通启动只检验。用真实主题覆盖继承默认值、覆盖值、同键压缩、按字节清理和配置变更。
4. 在会改变运行状态的步骤前执行预检。分别处理离线配置检查、真实依赖预检、迁移及启动。Redis TLS 与 Kafka 自定义 CA 只将已贯通应用和探针的组合列入支持范围。事件写入启动前完整检查表结构，运行采用轻量探针配合有界的漂移检查。
5. 修复 Web 镜像构建依赖。把应用构建依赖与跨模块测试夹具的检查边界写清，保留独立的仓库级测试和类型检查。把四应用、基础组件与一次性任务集中到同一份 Compose。用 `docker compose config` 核对有效服务、镜像、端口和依赖顺序。同步根配置模板、启动脚本、Web 私有配置和 IAM 允许的真实代理范围。给 Web 的缓冲路径加有界读取，保留现有上游业务校验和流式设备接收。
6. **完成标准** · 四应用镜像构建通过，分别离线校验。只改无关角色的配置不会阻断当前角色。真实组件验证内置与外接、最小权限、错误凭据拒绝及项目支持的 TLS 模式。预检失败时已运行进程继续服务。

### B. 首版数据库、业务命名和用例边界

1. 将 IAM、Ingest 的 PostgreSQL 和 Ingest 的 ClickHouse 分别收拢成建立最终结构的首版基线。删除试制旧库转换、旧字段回填、种子规则再删除及旧 ClickHouse DDL 纳管路径。基线直接包含现有有效列、约束、索引、注释和登记逻辑。运行账号、迁移账号、Go 枚举、SQLC 生成物及初始化测试一起核对。
2. Ingest 从 API 到 Go、表、权限键、UI 请求层统一解析规则和字段映射术语。`parsing_rule_id`、规则修订与 OCSF 目录版本分别具有明确语义。「数据接入」表示接入实体或规则的引用数量，标识符与数量分别命名，列表使用「事件类型」，对外保留 OCSF 的 `class_uid` 等标准字段。仓储返回类型化数据，业务校验与变更进入用例，HTTP 保留 DTO。现有外部日志协议和 OCSF 枚举按锁定标准表达。先检查当前消费者，再一次换掉旧名。用实际表和列检查注释、取值约束和索引。
3. 删掉旧规则读取、旧共享字段和旧版草稿的特判。将权威编译逻辑用于保存、完整试算和运行。不完整提取输入按自己的请求结构校验。规则摘要随本次修订原子更新，目录或约束失效有明确的字段级错误。数据接入绑定复用现有事务和规则修订冲突校验。清理仅测试调用的全量查询，保留业务覆盖。
4. IAM 将首次管理员绑定、迁移权限目录、密码维护和恢复会话撤销分开。现有审计入口改走用例，SQL 进入已有查询目录。按闭合取值、UTF-8 字段长度、命名、UTC、约束和索引规范验收 PostgreSQL。ClickHouse 校验物理列的来源路径、类型和首版永久保留的表结构。按下表清理 Event 过期删除路径。
5. **完成标准** · 在全新隔离库初始化两次，证明第二次幂等。并发编辑有明确冲突，审计和写变更同事务，失效字段得到确定拒绝。旧 YAML 段、路由和规则形状没有运行入口，旧环境变量不影响有效配置。全仓新配置、接口、权限及 SQL 生成检查通过。

| 永久保留清理范围 | 实施与验收 |
| --- | --- |
| 配置和命令 | 从 `.env.example`、模块 YAML、类型、校验、Compose 注入和命令调用链删除 Event 保存天数。`Migrate`、`CheckSchema` 和写入器都不再接收保留天数。空间与积压诊断继续有效。 |
| 建表和漂移检查 | 首版基线使用 UTC 月分区。删除表 TTL、过期操作及其 DDL 模板参数。实库只读核对建表语义，带删除 TTL 的表拒绝通过；启动过程保持无清理和无自动修改表结构。开发库重建按交付边界办理。 |
| 查询与测试 | 检查按年龄隐藏 Event 的过滤、清理作业和指标。删除仅为过期功能存在的分支。把自动删旧样本的用例改为跨月、历史补采和重启后仍可查询的验证。保留 Kafka 位移出界、ClickHouse 结构漂移和回查测试。 |
| 文档 | 同步根 README、Ingest README、`AGENTS.md`、Event 存储规格、部署与技术架构、配置与数据库指南。去掉当前操作中的按天过期、打开 TTL 和旧命令说明。历史验证记录标清适用版本。 |

### C. Event 交付、查询和运行恢复

1. 固定常规交付为 Kafka 先确认、ClickHouse 异步写入。用不同状态区分已受理与已持久化，诊断消息说明源记录是否仍可回查。下游按照各自契约读取安全事件、解析诊断和写入失败。ClickHouse 中的完整 Event 永久保留。事件写入可靠性继续以批次失败、部分确认、进程重启、重平衡和位移超期验收。积压超过实际可用位移时，操作界面和诊断记录必须能识别尚未落库的缺口。
2. 按已有数据模型提供有界的事件回查。Ingest 管 ClickHouse 查询、读权限、时间窗口与分页。Console 只在实际需要聚合时调用接口。首版建表按事件发生时间固定 UTC 月分区，同步表结构校验、测试和文档。补采旧事件落入其发生月分区，验证跨月边界与写入后回查。按长期容量和代表性查询核对索引与排序。
3. 为 Kafka 分区暂停和写入积压补齐操作入口、主题身份和缺口证据。优先以现有命令、指标与恢复手册实现。规范待投递审计的重试与确定拒绝路径，测试移动失败状态与后续投递不会跳过未确认事实。对接入快照和 Redis 确认失败分别验证保留旧快照、停止受理或恢复条件。
4. 收口 Console 的授权上下文、就绪和试制接口。核对仓库及已登记对外调用，只由测试调用的试制 SSE 连同专用权限、配置、转发及说明一起清理。实际用户会话继续按真实入口覆盖撤权，Console 保留自己的服务鉴权边界。就绪及诊断复用现有探测，区别进程响应与 IAM 依赖异常。
5. **完成标准** · HTTP、Syslog、外部 Kafka 各一条代表性事件走完接收、归一化与存储，准确回查事件 ID、原规则修订和时间。用同一来源复验重试、积压及恢复。权限和原始报文访问符合当前定义。

### D. Web 信息架构与核心任务

1. 按数据接入、解析规则、系统设置归拢业务文件；路由只负责入口与参数。规则的创建、编辑、详情复用同一工作区骨架和字段表示。沿用现有模板的工作台标题、列表、抽屉和表单，将页面专用组件放在功能附近。
2. 权限、路由、错误映射、类型和双语词典对齐后端新契约。列内容和操作靠左，搜索与下拉按共享组件规则展示。规则与接入复用三行样本输入和逐条错误，保留长日志换行与样本边界。正确区分规则加载失败、未选择、部分解析与试算通过。用同一事件验证预览、保存和详情内容。
3. 审计按用户有效时区形成 UTC 半开区间传入服务端。测试本地凌晨、跨夏令时边界、撤销过滤、按游标翻页。页面与后端使用同一筛选语义。
4. **完成标准** · 基于真实服务执行规则建改查、接入状态、事件回查和审计日期旅程。按模板检查桌面和窄宽度、明暗主题、键盘、空态、失败与重试。浏览器结果和接口结果可相互对账。

### E. 全栈 Compose、重复执行与同版本恢复

1. 沿用 `make compose-up`，补齐 Ingest、Kafka、ClickHouse 及一次性初始化任务。Compose 等待实际依赖、Web 与核心服务就绪，迁移、主题初始化和管理员初始化均有明确先后关系。保留只启动指定服务的入口。当前示例的公开默认口令与环回端口只用于隔离开发，真实部署的预检须拒绝样例凭据并检查接入监听和访问边界。
2. 在隔离资源完成空库启动、重复启动、停止排空、重启和同版本备份恢复。停止再启动后核对数据和集群身份仍在，初始化步骤可重复。开发旧库由明确有权的操作者重建，在线启动不会重置卷。
3. 同版本备份包含 ClickHouse、PostgreSQL 和 Kafka 主题、消费组、分区的恢复位移。演练在事件写入器排空并停止提交后记录截止点，以确保恢复材料对应一个已确认的连续消费位置。恢复前核对目标身份和 Kafka 可用位移，区分空流与材料缺失。恢复后先确认旧会话已撤销，再校验事件回查、接续写入和重复消息的逻辑计数。恢复材料先用现有组件备份工具及运行手册表达。
4. **完成标准** · 全栈 Compose 启动后能登录、接入日志并从 ClickHouse 回查事件。正式反代路径上用两个来源核对各自的限流、审计、会话和设备推送。默认 7 天和修改后的 Kafka 保留策略在实际 broker 回读通过。记录初始化、可重复执行、恢复耗时和数据缺口。

### F. 容量、规范与维护交接

1. 使用 [Event 资源预算](../event-contract/performance.md) 的计算方法，先跑代表性基线，再修有证据的瓶颈。记录测试硬件、入流分布、报文大小、规则复杂度、查询组合和并发。采集持续吞吐、p95/p99、峰值内存、错误率、PG 锁等待、ClickHouse 扫描/合并开销、Kafka 积压及最老记录年龄。
2. 按下表核对每个随时间增长的表、任务或缓存。内置资源预算以原入口验证，根据最小样本证据决定索引、缓存范围和连接预算。避免加一套运维系统承接几个已有任务。
3. 更新 `AGENTS.md`、[后端结构](../../development/backend-structure.md)、[数据库设计](../../development/database-design.md)、[数据访问](../../development/data-access.md)、[配置](../../development/configuration.md)、[测试](../../development/testing.md) 与模块 README。修正检查单的矛盾，补可执行的依赖方向与模块所有权检查。将真实命令、表结构、配置和诊断入口写入各所有者文档。正式发行后的升级采用已登记的前向迁移，试制路径随首版清除。
4. 复用当前 Go 可达漏洞扫描、锁文件扫描、工作流和凭据检查。用真实路由和服务身份复核权限拒绝、原始报文可见性及凭据注入范围。记录实际扫描对象与结果，静态预检结果与运行验收分别记录。 [安全检查](../../../.github/workflows/ci.yml#L235)
5. **完成标准** · 性能结论有可重复的输入和命令。四应用的路径、依赖和 README 能用于新模块写法参考。旧业务名仅在历史实施记录和必要的清理清单中出现。

| 资源 | 增长、查询和锁 | 失败与重试验收 |
| --- | --- | --- |
| ClickHouse 事件表与平台 Kafka | ClickHouse 按累计事件数、实测压缩字节和副本计算盘量，观测磁盘水位。Kafka 以日写入量和配置的保留窗口计算。主要读路径为时间窗加类型、接入、IP 或 ID。批写不在 PostgreSQL 持锁。 | Kafka 写入器确认后提交。测 ClickHouse 停写、重放计数、过期位移、晚到和追赶，并核对物理行与逻辑事件口径。 |
| 接入、规则、修订和分区执行边界 | 管理表按记录与修订增长，边界约为有效切换次数乘分区数。管理写和分区锁分离，读取同一规则版本。按来源 Kafka 的实际重读边界评估执行位移记录的清理。保留 Event 引用的不可变规则修订。 | 分别测低有效载量、多分区、重平衡与配置变更。规则提交失败后各项状态保持原子，重复消费保持规则和事件身份。旧 Event 在规则更新后仍能回查原修订。 |
| IAM 会话、审计、Ingest outbox | 会话随登录增长，审计随操作增长，outbox 随未确认数增长。审计按索引游标读和批量清理，会话固定账号再会话的锁顺序，外部投递在事务外。 | 撤权与续期竞态、清理积压、暂时失联、重复入账及永久拒绝均有隔离用例。数量和最老等待时长可观察。 |
| 接入快照与统计 | 快照和活动统计的规模由接入数、活动进程和窗口决定。锁只保护进程内共享状态。 | 验证进程退出、Redis 暂不可用、集中重连、恢复与失效。容量实测同时覆盖多个入口和 IAM 请求。 |

## 依据与取舍

| 参考 | 用于本计划的具体决策 |
| --- | --- |
| [Go 官方布局](https://go.dev/doc/modules/layout) · [Loki 装配源码](https://github.com/grafana/loki/blob/main/pkg/loki/modules.go) · [OpenTelemetry Collector 配置](https://opentelemetry.io/docs/collector/configuration/) | 用 `internal` 表达模块私有，一个制品可承担多个运行角色。连接配置与角色实际启用的流水线分别定义。 |
| [Apache Kafka 生产端](https://kafka.apache.org/42/configuration/producer-configs/) · [消费端](https://kafka.apache.org/42/configuration/consumer-configs/) · [OpenSearch Data Prepper Kafka 源](https://docs.opensearch.org/latest/data-prepper/pipelines/configuration/sources/kafka/) · [Snuba 消费实现说明](https://getsentry.github.io/snuba/architecture/consumer.html) | 集群连接和消费组身份分开。明确 ACK 与提交位移边界。使用现有 Kafka 接入与异步 ClickHouse 写入模式，按实际需求验收消费者和恢复。 |
| [ClickHouse 分区键](https://clickhouse.com/docs/concepts/best-practices/partitioning-keys) · [主键选择](https://clickhouse.com/docs/best-practices/choosing-a-primary-key) · [Kafka 主题配置](https://kafka.apache.org/42/configuration/topic-configs/) · [Graylog 索引集源码](https://github.com/Graylog2/graylog2-server/blob/4912933127e13ba7e84d74daef36f5c182af1320/graylog2-server/src/main/java/org/graylog2/indexer/indexset/IndexSetConfig.java) | 采用低基数月分区与独立排序键。Kafka 的时间、容量与压缩共同影响可读范围，应用按主题实际值验证。事件永久保留是产品政策。 |
| [OCSF 固定源码](https://github.com/ocsf/ocsf-schema/tree/2d209039d9c476ab76e83fb58b6f99374d40a994) · [阿里云标准化日志接入规则](https://help.aliyun.com/zh/security-center/user-guide/standardized-log-access-rules) | 锁定 OCSF 1.10.0 的字段与枚举，标准字段目录给 UI、校验和物理投影共用。目录生成记录上游提交和编译器版本。试算样本和落库规则具有独立生命周期。 |
| [OCSF HTTP Response](https://raw.githubusercontent.com/ocsf/ocsf-schema/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/http_response.json) · [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#name-status-codes) · [IANA 状态码](https://www.iana.org/assignments/http-status-codes) | 状态码字段使用 OCSF 路径，协议给出整数范围，IANA 提供常用提示值。活动、严重度等闭合枚举按各字段的 OCSF 定义展示。 |
| [OpenSearch 试算 API](https://docs.opensearch.org/latest/ingest-pipelines/simulate-ingest/) · [Logstash 字符集](https://www.elastic.co/docs/reference/logstash/plugins/plugins-codecs-plain) · [Filebeat 多行边界](https://www.elastic.co/docs/reference/beats/filebeat/multiline-examples) | 样本以条为单位试算，保持报文内部换行。真实接入按报文字节解码，浏览器粘贴按 UTF-8。协议分帧和来源编码留在各自已有边界。 |
| [GitLab 可信代理](https://docs.gitlab.com/omnibus/settings/nginx/#configure-gitlab-trusted-proxies-and-nginx-real_ip-module) · [Grafana Loki 有界请求读取](https://github.com/grafana/loki/blob/main/pkg/loghttp/push/push.go) | 明确可信代理和来源头，通过现有入口配置接通。在首个缓冲请求的进程限制读入字节。 |
| [Keycloak 初始化与恢复](https://www.keycloak.org/server/bootstrap-admin-recovery) · [Wazuh 单机 Compose 源码](https://github.com/wazuh/wazuh-docker/blob/main/single-node/docker-compose.yml) · [AiSOC 及其他对照](../commercial-delivery/references.md#发行安装与恢复) | 明确首次管理员维护入口、组件依赖和持久卷所有权。首版用 Compose 验收，组件版本和配置从拥有者处读取。 |
| [Compose 命令与项目目录](https://docs.docker.com/reference/cli/docker/compose/) · [有效配置](https://docs.docker.com/reference/cli/docker/compose/config/) | 明确构建上下文，验证 Compose 实际解析的服务和启动顺序。 |
| [ClickHouse 备份与恢复](https://clickhouse.com/docs/concepts/features/backup-restore/overview) | 使用原生备份和隔离恢复演练。消费位移、规则修订和会话撤销由 SecPlexus 按本项目的恢复边界核对。 |
| [Next.js 目录组织](https://nextjs.org/docs/app/getting-started/project-structure) · [现有 Web 模板映射](../../../src/apps/web/README.md#selective-template-adoption) | 按任务就近组织业务组件，跨功能共享视觉与交互原语。 |
| [Next.js 本地字体](https://nextjs.org/docs/app/getting-started/fonts#local-fonts) · [Fontsource 可变字体](https://fontsource.org/docs/getting-started/variable) | Compose 构建实测中字体 CDN 请求失败。将现有字体选择纳入锁定的本地包，应用构建由包内文件提供字体。 |

## 实施交接与验收

每批先复现拟改行为，再用 [改动类型与必需测试层](../../development/testing.md#改动类型与必需测试层) 确定用例。用现有 `make test`、`make catalogs`、`make sqlc`、`make lint`、`make typecheck`、`make build` 及隔离集成/E2E 入口。DB 约束、真实凭据、消息确认、恢复不能只靠单测。真实组件运行应记载使用的依赖组合、成功条件和未执行项。

核对每项变更的实际消费方，随同一批实施更新下列受影响位置。命名和删除都经当前入口和实际存储验证。

| 位置 | 实施时核对 |
| --- | --- |
| 配置、启动和部署 | 根 `.env.example`、应用 YAML、Web 私有配置、Compose、镜像、Makefile、脚本和 CI 使用同一现行键名、参数和组件版本。检查缺省、覆盖、缺失和错误值。更新启动帮助、诊断、配置示例和 TLS 用法。 |
| 源码和接口 | 路由、用例、DTO、Go/TS 名称、权限目录、API 调用者、错误码、日志事件与中英文词典共同切换。删除只对应退役行为的命令、路由、权限、脚本和夹具。保留可观测的真实业务回归。 |
| 数据库、目录与注释 | PostgreSQL 迁移、表列和 `COMMENT ON`、约束与索引、SQLC 查询及生成物一起核对。ClickHouse 建表、类型列、注释、排序、分区和结构检查与事件目录一致。更新锁定契约和生成目录的调用者。源码注释解释当前约束。 |
| 文档与运维 | 根中英文 README、各模块 README、架构和开发规范说明当前所有权、部署前置、配置、命令、错误及恢复路径。旧计划留验证记录，现行使用说明的旧名和旧操作随改动清理。用文档命令在干净环境复验。 |

四个模块收尾以可重复的结果为准。

- **Web、IAM** 实际入口登录、角色权限、撤权、审计日期和系统设置回归通过。真实浏览器通过规则建改查、接入试算和列表筛选，覆盖长文本、键盘、窄屏和明暗主题。
- **Ingest、Console** 从空环境启动后，HTTP、Syslog、Kafka 各出一条可回查的事件。权限拒绝、规则修订回查、积压诊断、Console IAM 降级及同版本恢复可核对。新模块可依赖已明确的事件契约与 IAM 权限边界。
- **维护交接** 通过发布前的真实组件测试、依赖与凭据检查和代表负载实测。对外容量以已记录的混合负载和持续时间为准。缓存、索引、批量改动或共享抽取只处理有瓶颈或重复证据的地方。后续检测、告警、研判和独立安装包按实际功能立项。

### 本轮验证结果

| 范围 | 实测结果 |
| --- | --- |
| 配置标准化复验 | 核对根模板 55 项直接配置和 38 项可选示例。私有环境的 58 个现有值全部保留。扩展的 8 项 Compose 检查通过。从空卷运行 `make compose-up` 后，实测自定义 Web 端口及来源、两次失败锁定、Ingest 256 字节请求体上限和突发限额，并回读两个主题的 3 天留存。独立 IAM 使用显式 PostgreSQL 和 Redis URL 通过就绪检查。Go 与 Web 单测、真实依赖集成测试、配置预检、类型检查及 SQL 生成物核对通过。 |
| 配置收口复验 | 核对根模板和私有环境的 55 项公共键，复核模块 YAML 与 Compose 的注入范围。隔离 Compose 从空卷构建并就绪，验证更改 Web 发布端口、IAM/Ingest 各复用一个镜像，以及 Kafka 命名空间与 3 天保留期回读。Go 集成与 22 项 Chromium 旅程通过。 |
| 全栈 Compose | 从隔离空卷启动四应用及事件写入器，初始化 PostgreSQL、Redis、Kafka 和 ClickHouse。再次用同一部署启动并执行登录、解析规则、数据接入、设备推送和事件回查。 |
| 事件与主题 | 分别经 HTTP、Syslog TCP 和外部 Kafka 接入代表性报文，核对 Kafka 受理和 ClickHouse 事件。外部 Kafka 测试使用隔离 broker 内单独的来源主题。通过事件回查接口核对 ID、规则修订和源 IP，匿名被拒绝。验证 UTC 月分区与历史事件回读。默认主题的有效保留为 7 天，单独创建的主题按 3 天通过预检。 |
| 数据库与消息集成 | 通过真实 PostgreSQL、Redis、Kafka 和 ClickHouse 测试。覆盖首次迁移、管理员重复初始化、审计拒绝后投递继续、消息位移出界、历史补采、ClickHouse 重放、备份回读及沿原消费位移继续向恢复库写入。解析规则的修订与样本验证进入 Ingest 用例层，IAM 的服务审计写入进入 IAM 用例层。 |
| 可信入口 | 隔离 Nginx 反代与两个独立容器验证来源限流分别计数、审计保留不同的来源、客户端自报的来源头被覆盖。实际部署的 TLS 和代理网段仍需按环境复验。 |
| Web 与 IAM | 27 个 Web 单测文件共 132 项通过。Chromium 22 项通过，覆盖登录、退出撤权、明暗主题和窄屏、规则建改查、规则修订冲突、逐条试算、共享列表过滤和系统设置。Console 已用真实会话验证授权及退出后拒绝。 |
| 工程校验 | SQLC、权限目录、类型检查与 diff 检查通过。Web lint 退出码为 0，仍有警告。对版本库文件和未忽略新增文件扫描无凭据命中。锁文件扫描无报告问题，Go 可达漏洞扫描为 0。直接对本地目录运行凭据扫描会命中被 Git 忽略的本机证书。 |
| 短时负载 | Apple M1 Pro 的 8 GiB Docker 虚拟机运行单节点 Compose。单 HTTP 接入以每秒 6 个请求发送，每批 20 条小型 JSON，共 3,600 条。确认结束耗时 31.58 秒，P95 1.89 秒、P99 2.00 秒，零拒收，确认后 2.97 秒内在 ClickHouse 对账完成。当前实测不能用作长期容量承诺。 |

### 收尾出口

- 在目标环境核对外接 PostgreSQL、Kafka、ClickHouse 的真实账号权限、TLS、凭据轮换和持续就绪。以部署覆盖文件验收公网隔离。
- 在目标环境把 PostgreSQL、ClickHouse 和 Kafka 位移恢复串起来，记录恢复时间和实际缺口。已验证恢复后的 ClickHouse 能沿原消费位移继续写入。
- 用真实混合负载测持续吞吐、写入积压、PG 锁等待、ClickHouse 扫描与磁盘容量，决定规则列表摘要、编译缓存和索引是否值得调整。继续完成仍跨层处理业务的 Ingest 管理用例及规则工作区目录对齐。
