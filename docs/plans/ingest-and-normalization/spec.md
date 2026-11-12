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

[prototype.html](prototype.html) 是可交互的静态原型（内联 CSS/JS，无构建、无依赖）。它不是凭感觉画的：下面每条交互都能对上行业实现，来源逐条列出。

### 交互依据

| 采纳的做法 | 出处（原文要点） | 在原型里落成 |
|---|---|---|
| **先验证映射，再启用数据源** | Cribl [Data Onboarding](https://docs.cribl.io/stream/data-onboarding)：官方建议的循环是 "Create configuration settings → **Verify that settings do the right thing** → Iterate"，并强调 "before it starts flowing" 用真实样本预览 | 向导第 4 步「样本验证」：贴一条设备真实日志，当场给出解析出的 OCSF 事件、逐字段映射表与未映射字段 |
| **索引/发布前预览数据形态** | Splunk [Assign the correct source types](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.2/how-to-get-data-into-your-splunk-deployment/assign-the-correct-source-types-to-your-data)：Set Source Type 页 "you can see how Splunk Enterprise will index the data" | 同上；未通过验证不能进入下一步（按钮禁用，不是只给提示） |
| **创建 ≠ 运行** | Graylog [Set Up an Input](https://go2docs.graylog.org/current/getting_in_log_data/setup_an_input.htm)："it is not in a running state until after you complete the Input Setup Wizard" | 创建后状态是「待启动」，必须在列表或抽屉里显式启动；启动才绑定端口、才计入接收流量 |
| **诊断页的信息结构** | Graylog [Input Diagnosis](https://go2docs.graylog.org/current/getting_in_log_data/input_diagnosis.htm)：Information（bind 地址、端口、traffic type）/ State（逐节点，含 `bind(..) failed: Cannot assign requested address`）/ Received Traffic（Network I/O）/ Errors During Processing / 内建排障建议（特权端口 <1024、地址不可达、TLS 过期） | 抽屉「诊断」页签：信息／状态／收到流量／处理错误／实例，并按当前状态给「建议」列表 |
| **能看原始 → 解析的对照** | Graylog 每个输入页的 Show received messages | 抽屉「收到的消息」页签：时间、摘要、已发布／隔离标记 |
| **模板要带版本** | Elastic [Agent policies](https://www.elastic.co/docs/reference/fleet/agent-policy)："The available settings in an integration depend on the version of the integration in use"；Graylog 的 Illuminate processing packs 同样按版本分发 | 模板行显示 `v3.2.1` 与「覆盖率」（最近 24 小时真实样本的填充率），实验性模板单独标注 |
| **端口绑定错误就地暴露** | Graylog Input Diagnosis 把 bind 失败与 <1024 特权端口列为最常见故障 | 绑定步骤实时校验端口是否已被其它数据源占用，冲突时直接给出错误提示并禁用下一步 |

### 刻意不做（以及为什么）

- **不抄 Elastic 的 agent policy 绑定**：Elastic 的接入对象是装了 agent 的主机，我们没有 agent，通道本身就是接入点，多一层 policy 只会增加空概念。
- **不做 Graylog 的多节点状态矩阵**：第一版只有一个 ingestor 实例，"逐节点"退化成一行；等多副本部署时再加。
- **不抄 Cribl 的 devnull pipeline 抓样**：那依赖 pipeline 概念，本计划没有；样本验证用"贴一条真实日志"覆盖同样目的。
- **不引入图灵完备的映射编辑器**：模板仍是声明式 YAML，原型只负责让人看见映射结果。

### 验收口径

原型同时定死一条验收标准：**自检必须显示样本被解析成什么**——映射到哪些 OCSF 字段、哪些字段没映射上、severity 判成几。只显示"连通成功"不足以判断模板能否用，这正是 Cribl 与 Splunk 都把预览放在保存之前的原因。

## 后续（不在本计划）

- raw payload 完整归档到对象存储与 `raw_uri`/`raw_hash` 引用。
- correlator：消费 `spx.ingest.event` → 写 ClickHouse（批量 fail-soft）→ D1/D2/D3 → 告警进 PG。
- Console 页面：数据源接入向导（选通道 → 选模板 → 出地址/token → 连通性自检）。
- AI 辅助生成模板：给定样本日志产出候选 YAML，人工评审入库；表达式逃生舱（CEL）在出现真实需求时评估。
