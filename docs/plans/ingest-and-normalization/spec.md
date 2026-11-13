# 数据接入与归一化（ingestor）

状态：待审核，未实施。范围是 ingestor 服务的第一版：三种通道接收、解析映射为 OCSF 事件、发布 Kafka。告警提升与合并归 correlator（另一项规则），事件落 ClickHouse 归 correlator/存储方案，均不在本计划。

## 背景与调研依据

ingestor 是一切外部安全数据的唯一入口。前一轮调研（AiSOC、Vector、Fluent Bit、Logstash、Elastic ingest pipelines、Sentinel ASIM、Chronicle、Splunk、Wazuh、Graylog、Cribl、OCSF 官方）收敛出以下事实，本设计据此定型：

- **通道与格式是两个维度**。通道 = syslog / HTTP / Kafka；格式 = JSON / CEF / LEEF / KV / 厂商私有。行业命名一致：Vector `http_server`、Fluent Bit `http`、Logstash `http`；webhook 是用法不是协议，对外文档与配置一律叫 HTTP。
- **映射层没有行业标准 DSL**。`ocsf/examples` 原文 "by no means a 'gold standard'"，按各自 DSL 分目录；`ocsf-java-tools` 的 `@move/@copy` JSON 规则文档仍标 DRAFT。各项目的实际形态是「声明式内核 + 表达式逃生舱」：Elastic processors + Painless、Fluent Bit modify + Lua、Logstash filters + ruby、Cribl Eval + Code。纯声明式（Splunk props/transforms、Wazuh XML）也存在，但 Wazuh 是 GPLv2 不能抄。
- **OCSF 字段命名是标准，提升规则不是**。`severity_id`/`class_uid` 是 OCSF required 字段（枚举 0–6 + 99 Other）；但 OCSF 官方在 1.4 专门加了 `is_alert`（recommended）让**生产者声明**「这条要人看」，并明确反对从 severity 推断（原文反例：恶意软件被拦截，severity 可以很低但仍是可告警信号）。AiSOC 的 `class_uid//1000==2 || severity_id>=4` 是其本地策略，且其 17 个模板中 12 个把 `class_uid` 写死为已废弃的 2001——「类别」实际是模板作者手写的常量，不是从数据推导的。结论：告警性声明放数据源配置，不放在接入层猜。本计划不实现提升，只保证事件携带可供 correlator 使用的字段。
- **接收路径不写库**。ClickHouse 官方文档明确 Kafka 表引擎存在静默丢行窗口（offset 先于 fsync 提交，"rows are lost with no error"），且 `kafka_handle_error_mode=default` 下一条坏消息可阻塞消费者引发 rebalance 风暴。因此 ingestor 只做「收 → 解析 → 发 Kafka」，不碰任何数据库；ClickHouse 由下游消费方写入。
- **syslog 无身份，绑定必须来自服务端**。syslog 没有认证与 header，报文内任何 tenant 字段都是可伪造的（AiSOC `syslog/sink.go` 注释：tenant 只能来自预配置 token）。绑定方式 = 一个监听端口绑一个数据源，与「接入时由用户选择/登记接入点」的交互一致，不需要独立组件，也不依赖 SO_REUSEPORT（跨 OS/内核不可控，已否决）。
- **解析库用现成的，不自研**：`leodido/go-syslog/v4`（MIT，RFC5424/3164 + RFC6587 两种分帧）、`ubyte-source/go-cef`（Apache 2.0，零分配，35+ 厂商样本）、`golang.org/x/text`（已在依赖树，GBK/GB18030/Big5/Shift-JIS）。Kafka 客户端 `segmentio/kafka-go`（AiSOC 同款，纯 Go）。

## 范围

本计划实现：

1. `ingestor` 服务骨架：`src/apps/ingestor`，Go + Gin，模块自有 `config.yaml`、迁移与启动，经 `iamclient` 接入 IAM。
2. 三种通道：HTTP 接收、syslog 监听（UDP+TCP）、Kafka 消费转发，全部产出 OCSF 事件。
3. 解析层：格式探测（JSON/CEF/LEEF/KV/纯文本）+ 厂商 YAML 模板映射（dot-path、severity 值表、常量、时间/消息字段、`kind` 声明），未映射字段进 `unmapped`。
4. 发布层：批量写 Kafka topic `spx.ingest.event`，分区键为实体标识，背压阻塞不丢数据。
5. 数据源注册（connectors）最小闭环：HTTP token、syslog 端口绑定、Kafka topic 绑定；通道类型与模板在注册时落定。
6. 可观测：结构化日志 + 指标（接收速率、解析失败、映射告警、发布延迟/失败）。

明确不做：

- 告警提升、去重、合并、攻击链——correlator 的职责。
- 写 ClickHouse / PostgreSQL 业务数据——ingestor 不碰业务库；connector 注册表属自身模块表。
- OTLP 通道、客户侧 agent/采集器、API 主动拉取（poll/fetch）——后续需求。
- 面向客户的脚本引擎（Lua/JS）或图灵完备 DSL——声明式模板 + Go 内置解析器覆盖第一版全部 18 家厂商；模板语言的表达力边界见行为契约。
- SO_REUSEPORT、syslog-relay 组件——多实例由部署侧 LB 解决。
- raw payload 落对象存储——第一版仅在事件上带 `raw_data` 摘要字段，完整 raw 归档与对象存储对接另立计划。

## 行为契约

### 通用

- 统一响应 `{code, message, data, request_id}`；模块前缀 `/api/v1/ingestor`；用户侧管理接口经 Web 同源代理，服务侧与设备侧接口直接暴露。
- 配置遵循仓库声明式规范：`src/apps/ingestor/config.yaml`，`${VAR-default}` 占位，环境变量前缀 `SPX_INGESTOR_`；加载即校验，不热更新。
- 时间一律 UTC；解析出的事件时间失败则置 `null` 并记 warning，绝不回退 `now()`。
- 缺失字段就是 `null`，不写 `"none"` 等哨兵值；`severity_id` 映射不上就是 `0 (Unknown)` 加 warning，不猜。
- 解码顺序 UTF-8 strict → GB18030 → replace（带损），实际编码记在 `metadata.encoding`。

### 通道一：HTTP

- `POST /api/v1/ingestor/ingest/{connector_id}`，鉴权 `Authorization: Bearer <ingest token>`（per-connector，SHA-256 摘要存储，常数时间比较）。
- 请求体上限 10 MiB（可配）；超限 413。数组批量上限 1000 条。
- 限流 per-connector：默认 50 req/s、burst 200（Redis 计数；Redis 故障时**拒绝**不放行，与 IAM 登录限流同语义）。
- 成功返回 `202`，正文为受理计数；解析与发布在响应之后异步完成，失败走指标与隔离区，不回传错误给设备（设备重推由去重层兜底）。
- Content-Type 只接受 `application/json` 与 `text/plain`；其余 415。

### 通道二：syslog

- 监听器在启动时按已注册的 syslog 接入点逐一绑定（UDP 与 TCP 同端口）；一个端口绑定一个 connector，报文内不读任何身份字段。
- 端口绑定关系存 connectors 注册表，来自 Console 管理接口；ingestor 周期刷新（默认 10s）+ SIGHUP 立即重载。端口变更通过热重载完成，不重启进程。
- RFC5424 / RFC3164 / RFC6587（八位组计数与非透明两种分帧）自动识别；单条上限 64 KiB，超限截断记 warning。
- CEF / LEEF 前缀自动识别并路由到对应解析器；纯 syslog 走 KV/文本解析。
- UDP 无连接语义，永不向发送方回错误；TCP 侧解析失败仅计数，不断开正常连接。

### 通道三：Kafka 消费

- 从已注册的 Kafka 接入点消费外部 topic（客户已有总线场景）；一条接入点 = 一组 brokers + topic + 反序列化模板。
- 消费组 `spx-ingestor-<connector_id>`，`offset_reset=earliest`，手动提交：仅当该批事件**已成功发布**到 `spx.ingest.event` 才提交位移。
- 无消息身份，模板与接入方式完全来自接入点注册（与 syslog 同语义）。

### 解析与映射

- 格式探测顺序：JSON → CEF → LEEF → KV（`k=v` 多分隔符）→ 纯文本（整体作 message）。
- 厂商模板（YAML，仓库内置、随二进制分发）：

```yaml
id: sangfor-af
kind: detection          # detection | telemetry，仅声明，供 correlator 使用
class_uid: 2004
severity_field: level
severity_map: {高危: 4, 中危: 3, 低危: 2}
time_field: time
time_formats: [unix_ms, rfc3339]
field_map:
  src_ip: src_endpoint.ip
  dst_ip: dst_endpoint.ip
  src_port: {set: src_endpoint.port, type: int}
  event_desc: message
constants:
  metadata.product.vendor_name: 深信服
  metadata.product.name: AF
```

- `field_map` 值为字符串（直映射）或对象（`set` 目标 + `type` 转换 + 可选 `default`）；两侧 dot-path，数组下标 `x[0]`。
- 模板可 `extends: base-json` 继承公共映射，逐键覆盖。
- 未知字段收集进 OCSF `unmapped`，一条不丢。
- `kind: detection` 的源若携带厂商自有告警语义，映射目标类为 `detection_finding (2004)` 并保留源告警字段到 `finding_info`；**不使用已废弃的 2001**。厂商自带阻断/放行类事件按官方建议带 `security_control` profile 语义（`disposition` 字段），是否成告警由 correlator 决定。
- 模板加载即校验（schema + class_uid/severity 值域），非法模板启动失败，不静默降级。
- 18 家内置模板（10 国产 + 5 国外 + 3 云）作为交付物；每家配 5–10 条真实样本的回归测试。

### 发布

- 单一出口 topic `spx.ingest.event`，value 为 OCSF 事件 JSON；分区键为实体标识（entity 优先级 src_ip → device → user → 域名，无实体回退 `event_id`，禁止回退固定常量造成热分区）。
- 批量发布：满 500 条或 2s 刷出（与 AiSOC 默认一致）；Kafka 不可用时**阻塞接收**（channel 背压），不丢数据、不落本地盘（第一版明确取舍：Kafka 是唯一缓冲层，其保留期即重放窗口）。
- 每条事件确定性 `event_id`（UUIDv5：connector_id + 原始指纹），为下游幂等去重提供键。
- 发布成功前通道层不确认（HTTP 202 已述异步；syslog/Kafka 由批发布与位移提交语义保证）。

### 隔离区

- 解析失败、模板缺失、超限截断的事件进入 `spx.ingest.quarantine` topic（附失败原因码）；不阻塞主流程，有计数指标。

### 数据源注册（connectors）

- 模块自有表 `ingestor.connectors`：`id, name, kind(http|syslog|kafka), template_id, token_digest(仅 http), listen_port(仅 syslog), kafka_source(仅 kafka), status, created_at`。
- 管理 API 归 ingestor 自有路由（创建/列表/启停/换 token/换绑定），用户会话鉴权走 IAM；通道类型与模板绑定在创建时落定，之后不可变。
- 权限点：`ingestor.connectors.read` / `ingestor.connectors.manage`，进 IAM 扁平权限目录（delegable 集中评审）。

## 验收

- 单元（表驱动）：格式探测边界；dot-path 读写含数组；severity 值表（含中文档位、数字 0–10 档、大小写归一）；时间解析（unix_ms/rfc3339/失败置 null）；模板继承与覆盖；`unmapped` 收集；`event_id` 确定性。
- 集成（真实组件）：HTTP token 鉴权允许/拒绝两侧；限流触发 429 与 Redis 故障拒绝；syslog UDP/TCP 收包→发布（真实端口绑定、两种分帧）；Kafka 消费位移只在发布成功后提交；`spx.ingest.event` 分区键保序；connector 注册→监听热重载。
- 模板回归：18 家模板各 5–10 条样本，断言关键字段映射与 severity；映射填充率报表生成。
- 不以 mock Kafka/mock Redis 证明发布与限流语义。

## 原型

[prototype.html](prototype.html) 是可交互的静态原型：单文件、内联 CSS/JS、无构建无依赖。它不是示意稿——**spec 里每条可被界面表达的行为契约，下面都能指到具体位置**；指不到的，单独列为"界面不表达"，并说明理由。

### 一、交互依据（为什么这样设计）

| 采纳的做法 | 出处（原文要点） | 落成位置 |
|---|---|---|
| 启用前先用样本验证映射 | Cribl [Data Onboarding](https://docs.cribl.io/stream/data-onboarding)：`Create settings → Verify that settings do the right thing → Iterate`，"before it starts flowing" | 向导第 4 步；未通过验证不能前进 |
| 保存前预览数据形态 | Splunk [Set Source Type](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.2/how-to-get-data-into-your-splunk-deployment/assign-the-correct-source-types-to-your-data)："you can see how Splunk Enterprise will index the data" | 同第 4 步：事件 + 逐字段映射 + 未映射 |
| 创建 ≠ 运行 | Graylog [Set Up an Input](https://go2docs.graylog.org/current/getting_in_log_data/setup_an_input.htm)："not in a running state until after you complete the Input Setup Wizard" | 创建后为「待启动」；启动才绑端口、才计速率 |
| 诊断页的固定结构 | Graylog [Input Diagnosis](https://go2docs.graylog.org/current/getting_in_log_data/input_diagnosis.htm)：Information / State（含 bind 失败原文）/ Received Traffic / Errors / 内建排障建议 | 抽屉「诊断」页签 + 状态相关的「建议」 |
| 能看原始 → 解析的对照 | Graylog Show received messages | 抽屉「收到的消息」：时间、摘要、格式、编码、已发布/隔离+原因码 |
| 模板按版本管理 | Elastic [Agent policies](https://www.elastic.co/docs/reference/fleet/agent-policy)："settings depend on the version of the integration"；Graylog Illuminate packs | 模板目录显示 `v3.2.1`、覆盖率、继承链 |
| 绑定错误提前暴露 | Graylog 把 bind 失败与 <1024 端口列为头号故障 | 绑定步骤即时校验端口占用并禁用下一步 |

### 二、契约 → 界面 对照

| spec 契约 | 原型位置 |
|---|---|
| 统一响应 `{code,message,data,request_id}` | 验证失败提示带 `code=… request_id=…` |
| 权限点 `connectors.read` / `manage` | 工具栏身份切换；只读时隐藏「接入数据源」与行内启停，并提示缺少 manage |
| 通道 = HTTP／syslog／Kafka | 向导第 1 步三张卡 |
| HTTP：Bearer 令牌、摘要存储、只显示一次 | 第 3 步令牌区（带说明）；详情「概览」可轮换，轮换后旧令牌失效 |
| HTTP：限流 50/s、突发 200、Redis 故障拒绝 | 第 3 步两个输入 + 固定策略说明（不提供开关） |
| HTTP：10 MiB、单批 1000、415、202 | 第 3 步上限输入 + Content-Type 多选；说明 413/415/202/异步语义 |
| syslog：UDP+TCP 同端口、一端口一接入点 | 第 3 步端口校验（冲突即错误并禁用下一步） |
| syslog：64 KiB 截断、特权端口 | 第 3 步上限只读说明；<1024 时诊断建议里点名 |
| syslog：周期刷新 + SIGHUP | 第 3 步刷新间隔输入；详情「概览」有「立即重载绑定」 |
| Kafka：消费组、earliest、发布成功才提交 | 第 3 步只读面板三条；详情「概览」与「发布·消费位移」 |
| 格式探测 JSON→CEF→LEEF→KV→文本 | 第 4 步「探测结果」面板显示探测到哪种格式及依据 |
| 编码 UTF-8 → GB18030 → replace | 同面板显示编码结论；有损时提示会被 `metadata.encoding` 记录 |
| 模板 `extends` 继承逐键覆盖 | 第 4 步「继承链」；模板详情「契约」行 |
| 模板 `kind` / `class_uid` / 不用废弃 2001 | 模板目录与详情显示 `class_uid`+`className`+`kind`（detection → 2004） |
| `severity_map`、`time_formats` 失败置 null/0 | 第 4 步映射表逐行给出"未识别档位→0 + warning""时间字段缺失→null" |
| `constants` | 模板详情「常量」表 |
| security_control `disposition` | 模板详情「disposition」行；模板声明时映射表出现 `disposition` 行 |
| 未知字段进 `unmapped` | 第 4 步未映射提示并列名；「映射质量」页签统计未映射 Top |
| 发布：单一出口 topic | 「运行指标」出口卡 + 详情「发布」 |
| 发布：分区键优先级与保序 | 第 4 步「分区键」行（实体值或回退链）；「发布」页签分区键 + 最近批次分区 |
| 发布：500 条 / 2s 批量 | 「运行指标·发布策略」与详情「发布」 |
| 发布：Kafka 不可用则背压不丢 | 「运行指标」出口状态与说明；「发布」页签背压行 |
| 确定性 `event_id`（UUIDv5） | 第 4 步与「发布」页签均标注来源 |
| 隔离区 + 原因码 | 「运行指标」原因码表；「收到的消息」行内原因码 |
| 可观测：接收速率／解析失败／隔离积压／发布延迟 | 「运行指标」五个 KPI + 按通道表 + 分区键分布 |
| connectors 列表/启停/换令牌/换绑定 | 列表行内启停；详情概览可轮换令牌、重载绑定 |
| 模板加载即校验，非法模板启动失败 | 模板详情「校验：通过」并注明校验范围 |

### 三、界面不表达的（与理由）

- **配置加载与环境变量**：属于部署面，界面不呈现。
- **迁移与启动同步**：运维命令，不进产品界面。
- **字段级错误计数**：原型只给到"哪条消息失败、什么原因码"；精确到字段的计数需要后端聚合，留到实施时再定粒度。
- **多实例状态**：v1 单实例，"逐节点"退化为一行；多副本时再展开。

### 四、刻意不抄

- Elastic 的 agent policy：我们的接入对象是通道不是主机，没有 agent，多一层空 policy。
- Cribl 的 devnull pipeline 抓样：依赖 pipeline 概念，本计划没有。
- 图灵完备的映射编辑器：模板保持声明式 YAML，原型只让人看见映射结果。

### 四点五、第二轮源码对照（Wazuh / Graylog / OpenCTI）

对三家开源实现的前端源码逐文件核对后补齐的交互：

| 缺口 | 出处（源码/文档证据） | 落成 |
|---|---|---|
| 状态机过于简单（4 态） | Graylog `InputStateBadge.tsx`：RUNNING/STARTING/FAILED/FAILING/STOPPED+未运行；Wazuh agent：never_connected/pending | 扩到 8 态：draft/starting/running/pending（从未收到）/silent/degraded（隔离率超阈）/failed（保留 bind 原文）/stopped |
| 横幅只是提示，不可操作 | Graylog `InputsNotifications.tsx`：横幅条目点击即给表格加 `runtime_status` 过滤 | 横幅每个条目可点，点击即设状态过滤器 |
| 设备侧配置靠人拼 | Wazuh `agent-deploy-one-liner.md`：向导生成可直接复制的部署命令 | 向导第 3 步按通道生成 rsyslog/syslog-ng/curl/producer 配置片段，带复制按钮 |
| 缺接入点元数据 | OpenCTI `ingestion-types.ts`：每个源带 description/user_id/last_execution_date+status | 概览页显示描述、创建人、状态持续时间；token 显示最近使用时间 |
| 删除无确认 | Graylog `InputsActions.tsx`：DeleteMenuItem + ConfirmDialog | 删除需二次确认，文案写明后果（历史事件保留） |
| 无克隆 | Graylog/Wazuh 均有同配置复制 | 克隆按钮：syslog 端口 +1 避冲突、http 令牌重生成、状态回待启动 |
| 消息流无自动刷新 | Graylog Show received messages 轮询 | 「收到的消息」页签加自动刷新开关（5 秒） |
| 健康结论没有派生 | OpenCTI `ingestionHealth-types.ts`：status/status_since/health_checks/notification_delay | 概览显示派生结论（隔离率 >2% → 质量降级）+ 状态起始时间 |

不采纳的：Wazuh 的 enrollment token 有效期/次数（我们的 HTTP token 是长期凭证，轮换即撤销，模型不同）；Graylog 的多节点状态矩阵（v1 单实例）；OpenCTI 的 notification_delay 防抖（后端指标聚合才有意义，界面层不做）。

### 五、验收口径

原型定死一条验收标准：**自检必须显示样本被解析成什么**——映射到哪些 OCSF 字段、哪些没映射上、severity 判成几、分区键取到哪个实体。只显示"连通成功"不足以判断模板能否用，这也是 Cribl 与 Splunk 都把预览放在保存之前的原因。

## 后续（不在本计划）

- raw payload 完整归档到对象存储与 `raw_uri`/`raw_hash` 引用。
- correlator：消费 `spx.ingest.event` → 写 ClickHouse（批量 fail-soft）→ D1/D2/D3 → 告警进 PG。
- Console 页面：数据源接入向导（选通道 → 选模板 → 出地址/token → 连通性自检）。
- AI 辅助生成模板：给定样本日志产出候选 YAML，人工评审入库；表达式逃生舱（CEL）在出现真实需求时评估。
