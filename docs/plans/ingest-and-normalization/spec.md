# 数据接入与归一化（ingest）

状态：待审核，未实施。范围是 ingest 服务的第一版：三种通道接收、解析映射为 OCSF 事件、发布 Kafka。告警提升与合并归 correlator（另一项规则），事件落 ClickHouse 归 correlator/存储方案，均不在本计划。

## 背景与调研依据

ingest 是一切外部安全数据的唯一入口。前一轮调研（AiSOC、Vector、Fluent Bit、Logstash、Elastic ingest pipelines、Sentinel ASIM、Chronicle、Splunk、Wazuh、Graylog、Cribl、OCSF 官方）收敛出以下事实，本设计据此定型：

- **通道与格式是两个维度**。通道 = syslog / HTTP / Kafka；格式 = JSON / CEF / KV / 厂商私有。行业命名一致：Vector `http_server`、Fluent Bit `http`、Logstash `http`；webhook 是用法不是协议，对外文档与配置一律叫 HTTP。
- **映射层没有行业标准 DSL**。`ocsf/examples` 原文 "by no means a 'gold standard'"，按各自 DSL 分目录；`ocsf-java-tools` 的 `@move/@copy` JSON 规则文档仍标 DRAFT。各项目的实际形态是「声明式内核 + 表达式逃生舱」：Elastic processors + Painless、Fluent Bit modify + Lua、Logstash filters + ruby、Cribl Eval + Code。纯声明式（Splunk props/transforms、Wazuh XML）也存在，但 Wazuh 是 GPLv2 不能抄。
- **OCSF 字段命名是标准，提升规则不是**。`severity_id`/`class_uid` 是 OCSF required 字段（枚举 0–6 + 99 Other）；但 OCSF 官方在 1.4 专门加了 `is_alert`（recommended）让**生产者声明**「这条要人看」，并明确反对从 severity 推断（原文反例：恶意软件被拦截，severity 可以很低但仍是可告警信号）。AiSOC 的 `class_uid//1000==2 || severity_id>=4` 是其本地策略，且其 17 个模板（AiSOC 自己的叫法）中 12 个把 `class_uid` 写死为已废弃的 2001——「类别」实际是模板作者手写的常量，不是从数据推导的。结论：告警性声明放数据源配置，不放在接入层猜。本计划不实现提升，只保证事件携带可供 correlator 使用的字段。
- **接收路径不写库**。ClickHouse 官方文档明确 Kafka 表引擎存在静默丢行窗口（offset 先于 fsync 提交，"rows are lost with no error"），且 `kafka_handle_error_mode=default` 下一条坏消息可阻塞消费者引发 rebalance 风暴。因此 ingest 只做「收 → 解析 → 发 Kafka」，不碰任何数据库；ClickHouse 由下游消费方写入。
- **syslog 无身份，绑定必须来自服务端**。syslog 没有认证与 header，报文内任何 tenant 字段都是可伪造的（AiSOC `syslog/sink.go` 注释：tenant 只能来自预配置 token）。绑定方式 = 一个监听端口绑一个数据源，与「接入时由用户选择/登记接入点」的交互一致，不需要独立组件，也不依赖 SO_REUSEPORT（跨 OS/内核不可控，已否决）。
- **解析库用现成的，不自研**：`leodido/go-syslog/v4`（MIT，RFC5424/3164 + RFC6587 两种分帧）、`ubyte-source/go-cef`（Apache 2.0，零分配，35+ 厂商样本）、`golang.org/x/text`（已在依赖树，GBK/GB18030/Big5/Shift-JIS）。Kafka 客户端 `segmentio/kafka-go`（AiSOC 同款，纯 Go）。

## 范围

本计划实现：

1. `ingest` 服务骨架：`src/apps/ingest`，Go + Gin，模块自有 `config.yaml`、迁移与启动，经 `iamclient` 接入 IAM。
2. 三种通道：HTTP 接收、syslog 监听（UDP+TCP）、Kafka 消费转发，全部产出 OCSF 事件。
3. 解析层：格式探测（JSON/CEF/KV/纯文本）+ 厂商 YAML 解析规则映射（dot-path、severity 值表、常量、时间/消息字段、`kind` 声明），未映射字段进 `unmapped`。
4. 发布层：批量写唯一出口 topic `spx.ingest.event`，分区键为实体标识，背压阻塞不丢数据。**没有独立的隔离区 topic**：解析失败的事件照常发布到同一 topic，带 `metadata.parse_status = ok | error` 与原因码，已解析字段与原文都保留，下游（CK/correlator）按 parse_status 过滤即可。
5. 数据源注册（connectors）最小闭环：HTTP token、syslog 端口绑定、Kafka topic 绑定；通道类型与解析规则在注册时落定。
6. 可观测：结构化日志 + 指标（接收速率、**解析失败率**、映射告警、发布延迟/失败）。

明确不做：

- 告警提升、去重、合并、攻击链——correlator 的职责。
- 写 ClickHouse / PostgreSQL 业务数据——ingest 不碰业务库；connector 注册表属自身模块表。
- OTLP 通道、客户侧 agent/采集器、API 主动拉取（poll/fetch）——后续需求。
- 面向客户的脚本引擎（Lua/JS）或图灵完备 DSL——声明式解析规则 + Go 内置解析规则覆盖第一版全部 18 家厂商；解析规则语言的表达力边界见行为契约。
- SO_REUSEPORT、syslog-relay 组件——多实例由部署侧 LB 解决。
- raw payload 落对象存储——第一版仅在事件上带 `raw_data` 摘要字段，完整 raw 归档与对象存储对接另立计划。

## 行为契约

### 通用

- 统一响应 `{code, message, data, request_id}`；模块前缀 `/api/v1/ingest`；用户侧管理接口经 Web 同源代理，服务侧与设备侧接口直接暴露。
- 配置遵循仓库声明式规范：`src/apps/ingest/config.yaml`，`${VAR-default}` 占位，环境变量前缀 `SPX_INGEST_`；加载即校验，不热更新。
- 时间一律 UTC；解析出的事件时间失败则置 `null` 并记 warning，绝不回退 `now()`。
- 缺失字段就是 `null`，不写 `"none"` 等哨兵值；`severity_id` 映射不上就是 `0 (Unknown)` 加 warning，不猜。
- 解码顺序 UTF-8 strict → GB18030 → replace（带损），实际编码记在 `metadata.encoding`。

### 通道一：HTTP

- `POST /api/v1/ingest/events/{connector_id}`，鉴权 `Authorization: Bearer <ingest token>`（per-connector，SHA-256 摘要存储，常数时间比较）。
- 请求体上限 10 MiB（可配）；超限 413。数组批量上限 1000 条。
- 限流 per-connector：默认 50 req/s、burst 200（Redis 计数；Redis 故障时**拒绝**不放行，与 IAM 登录限流同语义）。
- 成功返回 `202`，正文为受理计数；解析与发布在响应之后异步完成，解析失败不改写事件流（事件带 parse_status=error），不回传错误给设备（设备重推由去重层兜底）。
- Content-Type 只接受 `application/json` 与 `text/plain`；其余 415。

### 通道二：syslog

- 监听器在启动时按已注册的 syslog 接入点逐一绑定（UDP 与 TCP 同端口）；一个端口绑定一个 connector，报文内不读任何身份字段。
- 端口绑定关系存 connectors 注册表，来自 Console 管理接口；ingest 周期刷新（默认 10s）+ SIGHUP 立即重载。端口变更通过热重载完成，不重启进程。
- RFC5424 / RFC3164 / RFC6587（八位组计数与非透明两种分帧）自动识别；单条上限 64 KiB，超限截断记 warning。
- CEF 前缀自动识别并路由到对应解析规则；纯 syslog 走 KV/文本解析。
- UDP 无连接语义，永不向发送方回错误；TCP 侧解析失败仅计数，不断开正常连接。

### 通道三：Kafka 消费

- 从已注册的 Kafka 接入点消费外部 topic（客户已有总线场景）；一条接入点 = 一组 brokers + topic + 反序列化解析规则。
- 消费组 `spx-ingest-<connector_id>`，`offset_reset=earliest`，手动提交：仅当该批事件**已成功发布**到 `spx.ingest.event` 才提交位移。
- 无消息身份，解析规则与接入方式完全来自接入点注册（与 syslog 同语义）。

### 解析与映射

解析分两部分，职责不重叠：

- **解析引擎 = 代码**（`internal/normalizer` 包，随二进制发布）：格式探测、dot-path 求值、severity 归一、时间解析、`unmapped` 收集。引擎不含有任何厂商知识。
- **解析规则 = 数据**（per-vendor YAML，`go:embed` 内置，加载即 schema 校验）：描述某家厂商的字段怎么映射。换规则不动代码；引擎升级，全部规则受益。

引擎的**解析方式四模式**（每条解析规则声明一种，对齐 Fluent Bit parser 类型与 Graylog extractor 类型）：

解析分**两层**（AiSOC 同构分层，职责不混）：

```
原始字节 → 【第 1 层：线格式解析 wire parser】→ 扁平 map
         → 【第 2 层：规则映射 rule mapping】  → OCSF 事件 + parse_status
```

第 1 层把字节变成扁平 map，不含任何 OCSF 知识；第 2 层把扁平 map 投影到 OCSF，不含任何线格式知识。两层各自独立测试、独立复用。

**五种解析模式**（第 1 层，每条解析规则声明一种，对齐 Fluent Bit parser 类型与 Graylog extractor 类型）：

| 模式 | 规则参数 | 标准场景 | 实现（三方优先，不手搓） |
|---|---|---|---|
| `json` | 无 | 结构化 JSON/NDJSON 推送 | 标准库 `encoding/json`（单对象/数组/NDJSON 按首字节分发，失败降级，AiSOC `decodeJSONEvents` 同款） |
| `regex` | `regex`（命名捕获组） | syslog 传统格式、非结构化文本 | 标准库 `regexp`；复杂模式用 `elastic/go-grok`（Apache 2.0）展开 grok 宏 |
| `kv` | `kv_delimiter`/`kv_entry`/`kv_quote` | CEF 扩展、logfmt、自定义键值 | 状态机约 50 行（AiSOC `parseCEFExtension` 同款——无通用库适配任意分隔符组合） |
| `delimiter` | `delimiter`/`columns` | CSV、制表符、空格切列 | 标准库 `encoding/csv`（自定义分隔符 + 引号转义） |
| `cef` | 无 | ArcSight/Fortinet 行业格式 | `ubyte-source/go-cef` 或 AiSOC 式解析器（头部 7 段 + 扩展区状态机）；字段名行业约定（src/dst/spt…）开箱即用 |

裸 syslog 走 `leodido/go-syslog/v4`（RFC5424/3164/6587）解析为扁平 map 后同样进第 2 层。

**性能、稳定性与资源约束**（容量目标：单机 5 万 EPS，sustained）：

- **容量模型（5w EPS 的算术）**：5w 事件/秒 = 每秒 5 万次 解析+映射+发布。按 Go 单核约 5-10 万次轻量 map 操作/秒，解析+映射在 4 核上绰绰有余；瓶颈在 JSON 序列化与 Kafka 发布——批量 500 条/批 = 100 批/秒，单分区顺序写，Kafka 单机可承 10 万+ msg/s。**8C16G 单机支撑 5w EPS 是工程上宽裕的配置**（AiSOC 同构架构按同容量设计）。
- **goroutine 模型**：固定 worker 池（SPX_INGEST_WORKERS，默认 = CPU 核数），从 channel 消费事件，无 per-event goroutine 创建。channel 缓冲 = SPX_INGEST_BUFFER（默认 10000，约 0.2 秒的量）。
- **批处理贯穿全链**：接收→解析→发布全程按批流转（batch 500 条或 2 秒先到为准）。5w EPS 下每批 500 条 = 100 批/秒，每批一次 Kafka produce 调用（摊薄 syscall 与网络往返）。
- **零分配热路径**：sync.Pool 复用扁平 map 与 OCSF map；字符串字段引用原始字节切片（[]byte → string 零拷贝转换 unsafe 仅限标准模式已支持的场合，默认用 string(bytes) 让编译器优化）；正则预编译缓存 *regexp.Regexp，运行期零编译。GC 目标：稳态下 < 10% CPU 花在 GC。
- **正则安全**：Go regexp 是 RE2 线性引擎，无灾难性回溯——恶意样本不会拖垮 worker（结构性优势，不需要超时机制）。
- **panic 隔离**：每批（非每条）包 recover——单批坏消息只丢该批的 parse_status，worker 不崩。批内逐条解析失败已由 error 返回值覆盖，panic 是引擎 bug 兜底。
- **流式解码**：NDJSON 用 json.Decoder 逐条解码，不整包 Unmarshal；大 body 不翻倍占内存。
- **分发 O(1)**：Parser 注册为 map[string]Parser（Parser interface { Parse(raw []byte) (map[string]any, error) }），新增模式实现接口注册即可。
- **优雅停机顺序**：停接收（通道关闭）→ 排空在途解析 → 发布剩余批次 → 提交 Kafka 位移 → 退出。禁止直接退出丢在途数据。
- **资源上限**：worker goroutine 数与 channel 缓冲可配（`SPX_INGEST_WORKERS`/`SPX_INGEST_BUFFER`）；单条上限 64 KiB（syslog）/10 MiB（HTTP）已定；**不落盘原则不变**——Kafka 是唯一缓冲层。
- **背压语义不变**：Kafka 不可用阻塞接收，不丢数据。
- **可用性**：Kafka 不可用时接收阻塞（已有）；解析引擎 bug 只影响 parse_status 不丢事件（已有）；单 worker panic 由 recover 兜底并重启该 worker（worker 池模式：panic 后向池重新提交 worker）。健康检查暴露 workers/busy/batchBacklog 三个指标。
- **可维护性**：五模式各自一个文件（parser/json.go、parser/cef.go、parser/kv.go、parser/regex.go、parser/delim.go）+ registry.go 注册 + parser_test.go 表驱动；新增模式 = 新文件 + 一行注册，不改既有代码（开闭原则）。

**样本驱动校验**：解析规则必须携带多条真实样本（规则内嵌 `samples` 数组），保存与加载时全部样本逐条过解析引擎，任何一条失败即拒绝——规则质量由样本守护，不靠人眼。「通用接入」= 不挂规则运行引擎：只做格式探测与原文透传，事件带 parse_status=error（原因码 format_mismatch），可查询、后续可换正式解析规则。

厂商解析规则（YAML，仓库内置、随二进制分发）：

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
- 解析规则可 `extends: base-json` 继承公共映射，逐键覆盖。
- 未知字段收集进 OCSF `unmapped`，一条不丢。
- `kind: detection` 的源若携带厂商自有告警语义，映射目标类为 `detection_finding (2004)` 并保留源告警字段到 `finding_info`；**不使用已废弃的 2001**。厂商自带阻断/放行类事件按官方建议带 `security_control` profile 语义（`disposition` 字段），是否成告警由 correlator 决定。
- 解析规则加载即校验（schema + class_uid/severity 值域），非法解析规则启动失败，不静默降级。
- 18 家内置解析规则（10 国产 + 5 国外 + 3 云）作为交付物；每家配 5–10 条真实样本的回归测试。

### 发布

- 单一出口 topic `spx.ingest.event`，value 为 OCSF 事件 JSON；分区键为实体标识（entity 优先级 src_ip → device → user → 域名，无实体回退 `event_id`，禁止回退固定常量造成热分区）。
- 批量发布：满 500 条或 2s 刷出（与 AiSOC 默认一致）；Kafka 不可用时**阻塞接收**（channel 背压），不丢数据、不落本地盘（第一版明确取舍：Kafka 是唯一缓冲层，其保留期即重放窗口）。
- 每条事件确定性 `event_id`（UUIDv5：connector_id + 原始指纹），为下游幂等去重提供键。
- 发布成功前通道层不确认（HTTP 202 已述异步；syslog/Kafka 由批发布与位移提交语义保证）。

### 解析状态（无隔离区）

- **所有事件进同一个 `spx.ingest.event`**。事件携带 `metadata.parse_status = ok | error` 与 `metadata.parse_errors[]`（字段 + 原因码：`format_mismatch`/`source_missing`/`severity_unmapped`/`time_invalid`/`oversize_truncated`）。
- 解析异常的事件**不是废数据**：能解析出哪些字段就有哪些字段，原文保留在 message/unmapped，照样可查询；只是解析状态为 error，大部分业务按 `parse_status=ok` 过滤即可。
- ClickHouse 落库（correlator 计划）后 parse_status 是普通可过滤字段，不单独建库表。
- 指标：**解析失败率**（parse_status=error 的事件占比），按接入点与原因码维度可下钻。

### 数据源注册（connectors）

- 模块自有表 `ingest.connectors`：`id, name, kind(http|syslog|kafka), rule_set_id, token_digest(仅 http), listen_port(仅 syslog), kafka_source(仅 kafka), status, created_at`。
- 管理 API 归 ingest 自有路由（创建/列表/启停/换 token/），用户会话鉴权走 IAM；通道类型创建后不可变（端口/令牌/消费组是基础设施绑定），解析规则可更换——换后从下一条消息起生效，已发布事件不变（依据 Graylog 在运行中的 input 上管理 extractors/pipelines；Splunk 覆盖只影响新数据）。
- 权限：数据接入与解析规则**全员可见可管理**，不设独立权限点（项目决策：内部平台，全员运维）。IAM 用户会话仍然要求登录，但不做 ingest 专属授权检查。

## 验收

- 单元（表驱动）：格式探测边界；dot-path 读写含数组；severity 值表（含中文档位、数字 0–10 档、大小写归一）；时间解析（unix_ms/rfc3339/失败置 null）；解析规则继承与覆盖；`unmapped` 收集；`event_id` 确定性。
- 集成（真实组件）：HTTP token 鉴权允许/拒绝两侧；限流触发 429 与 Redis 故障拒绝；syslog UDP/TCP 收包→发布（真实端口绑定、两种分帧）；Kafka 消费位移只在发布成功后提交；`spx.ingest.event` 分区键保序；connector 注册→监听热重载。
- 解析规则回归：18 家解析规则各 5–10 条样本，断言关键字段映射与 severity；映射填充率报表生成。
- 不以 mock Kafka/mock Redis 证明发布与限流语义。

## 原型

[prototype.html](prototype.html) 是可交互的静态原型：单文件、内联 CSS/JS、无构建无依赖。它不是示意稿——**spec 里每条可被界面表达的行为契约，下面都能指到具体位置**；指不到的，单独列为"界面不表达"，并说明理由。

### 一、交互依据（为什么这样设计）

| 采纳的做法 | 出处（原文要点） | 落成位置 |
|---|---|---|
| 启用前先用样本验证映射 | Cribl [Data Onboarding](https://docs.cribl.io/stream/data-onboarding)：`Create settings → Verify that settings do the right thing → Iterate`，"before it starts flowing" | 向导第 2 步；未通过验证不能前进 |
| 保存前预览数据形态 | Splunk [Set Source Type](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.2/how-to-get-data-into-your-splunk-deployment/assign-the-correct-source-types-to-your-data)："you can see how Splunk Enterprise will index the data" | 同第 2 步：事件 + 逐字段映射 + 未映射 |
| 创建 ≠ 运行 | Graylog [Set Up an Input](https://go2docs.graylog.org/current/getting_in_log_data/setup_an_input.htm)："not in a running state until after you complete the Input Setup Wizard" | 创建后为「待启动」；启动才绑端口、才计速率 |
| 诊断页的固定结构 | Graylog [Input Diagnosis](https://go2docs.graylog.org/current/getting_in_log_data/input_diagnosis.htm)：Information / State（含 bind 失败原文）/ Received Traffic / Errors / 内建排障建议 | 抽屉「诊断」页签 + 状态相关的「建议」 |
| 能看原始 → 解析的对照 | Graylog Show received messages | 抽屉「收到的消息」：时间、摘要、格式、编码、parse ok/parse error+原因码 |
| 版本与来源 | Elastic [Agent policies](https://www.elastic.co/docs/reference/fleet/agent-policy)："settings depend on the version of the integration"；Graylog Illuminate packs | 解析规则显示 `v3.2.1`、来源（内置/自定义）、继承链 |
| 绑定错误提前暴露 | Graylog 把 bind 失败与 <1024 端口列为头号故障 | 绑定步骤即时校验端口占用并禁用下一步 |

### 二、契约 → 界面 对照

| spec 契约 | 原型位置 |
|---|---|
| 统一响应 `{code,message,data,request_id}` | 验证失败提示带 `code=… request_id=…` |
| 全员可见（无权限点） | 项目决策：内部平台全员运维，数据接入与解析规则不做 ingest 专属授权 |
| 通道 = HTTP／syslog／Kafka | 向导第 3 步三张卡 |
| 向导顺序：解析验证在基础设施配置之前 | Splunk Add Data：Set Source Type（预览）→ Input Settings → Review | 向导第 1→5 步：解析规则→验证→通道→绑定→启动 |
| 验证失败有出路，不堵死 | Splunk："If none of the existing source types fits…create a new one" | 验证失败出现「」「通用接入（原文透传）」两个按钮 |
| 解析规则创建后可换 | Graylog extractors 在运行 input 上增删改，下一条消息起生效 | 详情概览「更」；通道不可换 |
| HTTP：Bearer 令牌、摘要存储、只显示一次 | 第 4 步令牌区（带说明）；详情「概览」可轮换，轮换后旧令牌失效 |
| HTTP：限流 50/s、突发 200、Redis 故障拒绝 | 第 4 步两个输入 + 固定策略说明（不提供开关） |
| HTTP：10 MiB、单批 1000、415、202 | 第 4 步上限输入 + Content-Type 多选；说明 413/415/202/异步语义 |
| syslog：UDP+TCP 同端口、一端口一接入点 | 第 4 步端口校验（冲突即错误并禁用下一步） |
| syslog：64 KiB 截断、特权端口 | 第 4 步上限只读说明；<1024 时诊断建议里点名 |
| syslog：周期刷新 + SIGHUP | 第 4 步刷新间隔输入；详情「概览」有「立即重载绑定」 |
| Kafka：消费组、earliest、发布成功才提交 | 第 4 步只读面板三条；详情「概览」与「发布·消费位移」 |
| 格式探测 JSON→CEF→KV→文本 | 第 2 步「探测结果」面板显示探测到哪种格式及依据 |
| 编码 UTF-8 → GB18030 → replace | 同面板显示编码结论；有损时提示会被 `metadata.encoding` 记录 |
| 解析规则 `extends` 继承逐键覆盖 | 第 2 步「继承链」；解析规则详情「契约」行 |
| 解析规则 `kind` / `class_uid` / 不用废弃 2001 | 解析规则与详情显示 `class_uid`+`className`+`kind`（detection → 2004） |
| `severity_map`、`time_formats` 失败置 null/0 | 第 2 步映射表逐行给出"未识别档位→0 + warning""时间字段缺失→null" |
| `constants` | 解析规则详情「常量」表 |
| security_control `disposition` | 解析规则详情「disposition」行；解析规则声明时映射表出现 `disposition` 行 |
| 未知字段进 `unmapped` | 第 2 步未映射提示并列名；「映射质量」页签统计未映射 Top |
| 发布：单一出口 topic | 详情「发布」页签（独立指标页已并入数据接入页） |
| 发布：分区键优先级与保序 | 第 2 步「分区键」行（实体值或回退链）；「发布」页签分区键 + 最近批次分区 |
| 发布：500 条 / 2s 批量 | 详情「发布」页签 |
| 发布：Kafka 不可用则背压不丢 | 「发布」页签背压行；背压发生时列表横幅报警 |
| 确定性 `event_id`（UUIDv5） | 第 2 步与「发布」页签均标注来源 |
| 解析状态 parse_status + 原因码（无隔离区） | 数据接入页 KPI「解析失败率」；「收到的消息」行内 parse ok/parse error 与原因码 |
| 可观测：接入点运行状况／事件吞吐／解析失败率／24h 事件量环比 | 数据接入页顶部指标卡（沿用独立页时的 stat+sparkline 布局）+ 列表「最近收到」子行 |
| connectors 列表/启停/换令牌/换绑定 | 列表行内启停；详情概览可轮换令牌、重载绑定 |
| 解析规则加载即校验，非法解析规则启动失败 | 解析规则详情「校验：通过」并注明校验范围 |

### 三、界面不表达的（与理由）

- **配置加载与环境变量**：属于部署面，界面不呈现。
- **迁移与启动同步**：运维命令，不进产品界面。
- **字段级错误计数**：原型只给到"哪条消息失败、什么原因码"；精确到字段的计数需要后端聚合，留到实施时再定粒度。
- **多实例状态**：v1 单实例，"逐节点"退化为一行；多副本时再展开。

### 四、刻意不抄

- Elastic 的 agent policy：我们的接入对象是通道不是主机，没有 agent，多一层空 policy。
- Cribl 的 devnull pipeline 抓样：依赖 pipeline 概念，本计划没有。
- 图灵完备的映射编辑器：解析规则保持声明式 YAML，原型只让人看见映射结果。

### 五、第二轮源码对照（Wazuh / Graylog / OpenCTI）

对三家开源实现的前端源码逐文件核对后补齐的交互：

| 缺口 | 出处（源码/文档证据） | 落成 |
|---|---|---|
| 状态机过于简单（4 态） | Graylog `InputStateBadge.tsx`：RUNNING/STARTING/FAILED/FAILING/STOPPED+未运行；Wazuh agent：never_connected/pending | 定稿 3 态（用户要求简单）：运行中 / 已停止 / 启动失败（保留 bind 原文）；断流与解析质量不是状态，由 KPI 暴露 |
| 横幅只是提示，不可操作 | Graylog `InputsNotifications.tsx`：横幅条目点击即给表格加 `runtime_status` 过滤 | 横幅每个条目可点，点击即设状态过滤器 |
| 设备侧配置靠人拼 | Wazuh `agent-deploy-one-liner.md`：向导生成可直接复制的部署命令 | 向导第 4 步按通道生成 rsyslog/syslog-ng/curl/producer 配置片段，带复制按钮 |
| 缺接入点元数据 | OpenCTI `ingestion-types.ts`：每个源带 description/user_id/last_execution_date+status | 概览页显示描述、创建人、状态持续时间；token 显示最近使用时间 |
| 删除无确认 | Graylog `InputsActions.tsx`：DeleteMenuItem + ConfirmDialog | 删除需二次确认，文案写明后果（历史事件保留） |
| 无克隆 | Graylog/Wazuh 均有同配置复制 | 克隆按钮：syslog 端口 +1 避冲突、http 令牌重生成、状态回待启动 |
| 消息流无自动刷新 | Graylog Show received messages 轮询 | 「收到的消息」页签加自动刷新开关（5 秒） |
| 健康结论没有派生 | OpenCTI `ingestionHealth-types.ts`：status/status_since/health_checks/notification_delay | 概览显示派生结论（解析失败率 >2% → 质量降级）+ 状态起始时间 |

不采纳的：Wazuh 的 enrollment token 有效期/次数（我们的 HTTP token 是长期凭证，轮换即撤销，模型不同）；Graylog 的多节点状态矩阵（v1 单实例）；OpenCTI 的 notification_delay 防抖（后端指标聚合才有意义，界面层不做）。

### 六、第三轮对照：向导链路与解析规则可换（用户质疑驱动）

用户质疑原链路「通道→解析规则→绑定→验证」：选了解析规则但样本验证不通过怎么办？没匹配解析规则是不是就接不进来了？核对三家后的修正：

| 问题 | 出处 | 修正 |
|---|---|---|
| 验证放在最后，解析规则不匹配时基础设施配置已白做 | Splunk Add Data 步骤顺序（Select Source → Set Source Type 预览验证 → Input Settings → Review），原文 "If you're satisfied with the results, click Next to proceed to the Inputs Settings page" | 向导重排：解析规则 → 样本验证 → 通道 → 绑定 → 启动 |
| 验证硬挡，18 家不匹配即死路 | Splunk："If none of the existing source types fits the needs of your data, create a new one" | 验证失败给出路：，或「通用接入」回退——原文透传 message、不做字段映射（class_uid 6003），接入永不被堵死 |
| 解析规则绑定创建后不可变，选错只能删了重建 | Graylog [Extractors](https://docs.graylog.org/docs/extractors)：在运行中的 input 上管理，"The next messages coming in should already include the extracted fields"；Splunk 覆盖只影响新数据 | 通道不可变（基础设施绑定）；解析规则可换，从下一条消息起生效，已发布事件不变 |
| 解析规则能否替代通道 | Elastic 的 integration 捆绑采集与解析，但那依赖发 agent；我们无 agent，传输必须人选（Vector/Splunk/Graylog 均两维度分离） | 解析规则与通道保持两个独立维度；解析规则决定解析，通道决定传输 |

### 七、导航与模板基线（第四轮：用户指出）

**数据接入在主导航，不进设置。** 设置导航（/settings/*：个人资料、用户管理、角色与权限、审计日志）属于系统管理页面；数据接入是业务功能，与「工作台」并列放在主导航：

```
主导航（shell.tsx，非设置页时）
├─ 工作台
│   └─ 安全智驾工作台（/dashboard）
└─ 数据（本计划新增分组）
    ├─ 数据接入（接入点列表/KPI/向导/详情）
    ├─ 解析规则
    └─ 解析规则
```

**原型对齐 Web 模板基线**（此前手搓了一套与平台不一致的 Shell）：

| 基线项 | 依据文件 | 原型落点 |
|---|---|---|
| 侧栏 17rem / 图标收起 3rem，SidebarMenuButton h-7 | `src/components/ui/sidebar.tsx`（SIDEBAR_WIDTH 常量与 menu-button 变体） | 可收起侧栏，收起后仅图标 |
| 顶栏 h-12：Trigger + 竖分隔线 + 语言/主题/头像菜单 | `src/spx/components/shell.tsx` header 结构 | 同构顶栏，主题按钮真实切换明暗 |
| 侧栏令牌（sidebar 与画布同色、发丝线分隔） | `src/app/globals.css` --sidebar-* | 令牌同名同值 |
| PageHeader：text-3xl tracking-tight | `src/spx/components/page-header.tsx` | 页面标题同字号 |
| Card：rounded-xl + ring-1 ring-foreground/10 | `src/components/ui/card.tsx` | 表格卡/面板/指标卡全部改 ring |
| Button：rounded-full、hover:bg-primary/80 | `src/components/ui/button.tsx` default 变体 | 按钮行为一致 |
| 主题切换走 .dark 类 | ThemeSwitcher 行为 | 初始值取系统偏好，可手动切换 |

###析引擎与解析规则管理（第六轮：用户质询驱动，第七轮定稿术语）

用户问得对：只有 YAML 规则没有引擎设计，规则怎么管理也没定。调研结论：

**解析引擎（代码，不自研 DSL）**：

- 引擎 = Go 包 `internal/normalizer` 的求值器：格式探测 → dot-path 读写 → severity 值表归一 → 时间格式解析 → unmapped 收集 → parse_status/parse_errors 标注。
- **不引入 VRL/Lua/表达式语言**。行业事实：Vector 用 VRL（Rust 编译进二进制，Go 无法复用）；Fluent Bit 用 Lua（图灵完备，审计面大）；Logstash filter 是 JRuby。我们的声明式 YAML + Go 求值器覆盖全部 18 家场景（spec 已排除图灵完备 DSL），这是**有意的能力边界**，不是缺失。
- 库依赖维持原判：`leodido/go-syslog/v4`（RFC 解析）、`ubyte-source/go-cef`（CEF）、KV 自写（多分隔符探测）。grok 场景（`elastic/go-grok`）仅在真实样本证明需要时引入。

**解析规则管理（内置=文件，自定义=数据库，同一 schema 统一加载）**：

| | 内置解析规则 | 自定义解析规则 |
|---|---|---|
| 存储 | 仓库 `internal/normalizer/rules/*.yaml` + go:embed | 数据库表 `ingest.rules`（id、name、yaml text、created_by、created_at） |
| 可变性 | 只读（升级走发版） | 查看/编辑/删除 |
| 加载 | 启动时嵌入读取 | 启动 + 保存后从库读取，**与内置合并成同一规则集**，求值器不区分来源 |
| 改内置 | 「编辑」= 复制为自定义副本（id 加 `-custom`），原内置保留 | — |

依据 Wazuh custom decoders 的同款策略（复制到 `/etc/decoders` 并排除原文件，避免升级覆盖用户改动）；Graylog content packs 证明"规则即数据可导入导出"。**统一的代价是明确的**：内置与自定义同名时内置被自定义覆盖（最后加载者胜），spec 固定此语义。

**UI 落点**：两个列表均对齐平台 DataTable 规范（users/roles 页：SearchField 形态搜索、rounded-xl 边框表格、底部「共 N 条 · 第 x/y 页 + 每页行数 + 上一页/下一页」分页）。

- **数据接入列表**：数据源名称、通道、解析规则、接入地址、状态、操作——只展示接入配置信息；搜索匹配数据源名称与解析规则名称。
- **解析规则列表**：规则名称、规则描述、厂商、产品、来源、操作——展示规则基础信息，与数据接入列表差异化；搜索匹配规则名称、厂商、产品。

原记录：解析规则列表（非卡片）——名称/厂商/分组/kind/OCSF 类/来源/操作（查看、编辑、删除[仅自定义]），对齐平台 DataTable 列定义模式（users/roles 页）。搜索按厂商/名称/OCSF 类，分组筛选。**覆盖率列已删除**：该数字依赖真实样本回流，样本到位前是编造的。

### 八点五、状态表与删除语义（第七轮：用户定稿）

**数据接入状态表（定稿 3 态，无复合状态）**：

| 状态 | 含义 | 进入 | 退出 |
|---|---|---|---|
| 运行中 | 正常接收并发布数据 | 启动成功 | 手动停止 |
| 已停止 | 不接收数据（端口释放/令牌失效/消费组停止）；含"创建后未启动" | 手动停止 / 创建 | 手动启动 |
| 启动失败 | 最近一次启动失败，bind 错误原文保留展示 | 端口冲突等启动错误 | 修复后手动启动 |

断流（15 分钟无流量）、解析质量降级**都不是状态**——由数据接入页 KPI（24h 事件量环比、解析失败率）与详情「最近收到」暴露。解析规则**没有状态**（规则是数据，不存在运行概念）。

**删除语义（都是物理删除）**：

- 数据接入删除：直接物理删除表行；syslog 端口立即释放、HTTP 路由立即失效、Kafka 消费组停止。删除前二次确认，写明后果（历史事件保留）。
- 解析规则删除：物理删除，**前置条件是无任何数据接入引用**。有关联时阻止并列出关联数据源（标注哪些在运行中），须先停止并删除它们。内置解析规则不可删除（随二进制分发），要改就创建自定义副本。
- 数据流一致性：规则被删除前必须先摘除所有消费方，否则正在消费的接入点会立即失去映射。

**解析规则编辑器（表单模式，样本驱动）**——行业标准做法（Graylog extractor 向导、Elastic Grok Debugger 都是表单+即时试算，没有人让用户直接写 YAML）：

1. 贴一条真实样本（JSON）→ 引擎探测字段，以 chip 列出
2. 基础信息：规则名称、描述、厂商、产品
3. 字段映射：行式编辑器「源字段（下拉，从样本探测）→ OCSF 目标（dot-path 输入）」，可加行/删行
4. 时间字段、severity 字段：从探测到的字段下拉选择
5. 用样本试算：立即显示映射 N/M 与未映射字段
6. 保存前置条件：试算通过；内置解析规则「编辑」= 复制为自定义副本（id 加 -custom）

YAML 不再是用户界面——它是存储与导出格式（自定义解析规则在数据库中仍以 YAML 文本保存，与内置文件同 schema）。

### 九、服务命名与页面结构（第五轮：用户定稿）

- **服务名 ingest**（原 ingestor）：目录 `src/apps/ingest`、环境变量前缀 `SPX_INGEST_`、API 前缀 `/api/v1/ingest`（设备推送 `/api/v1/ingest/events/{connector_id}`、管理 `/api/v1/ingest/connectors`）、权限点 `ingest.connectors.read/manage`、表 `ingest.connectors`、消费组 `spx-ingest-<connector_id>`。Kafka 主题原本就是 `spx.ingest.*`，更名后全链路一致。
- **页面结构**：独立「运行指标」页删除，核心可观测并入数据接入页。主导航两个入口：数据接入、解析规则。
- **「解析规则」命名依据**：华为云安全云脑 SecMaster 对同一概念（对 Logstash filter 的无码化规则封装）即称「解析规则」；Wazuh decoders（解码器）、Graylog extractors（提取器）、Splunk sourcetype（源类型）为各家变体，「解析规则」是中文 SIEM 最通用的叫法。
- **KPI 砍留依据**：Microsoft Sentinel 数据连接器健康监控关注三个问题——数据是否完整流入、最后收到时间、异常；加上 24h 事件量环比（Splunk notable changes 模式：数据量骤降是断流最强信号）共四卡。分区键分布、发布批量等工程细节不进总览。

### 十、验收口径

原型定死一条验收标准：**自检必须显示样本被解析成什么**——映射到哪些 OCSF 字段、哪些没映射上、severity 判成几、分区键取到哪个实体。只显示"连通成功"不足以判断解析规则能否用，这也是 Cribl 与 Splunk 都把预览放在保存之前的原因。

## 后续（不在本计划）

- raw payload 完整归档到对象存储与 `raw_uri`/`raw_hash` 引用。
- correlator：消费 `spx.ingest.event` → 写 ClickHouse（批量 fail-soft）→ D1/D2/D3 → 告警进 PG。
- Console 页面实施：接入向导已定稿为五步（选解析规则 → 样本验证 → 选通道 → 绑定 → 启动与诊断），按 prototype.html 落地。
- AI 辅助生成解析规则：给定样本日志产出候选 YAML，人工评审入库；表达式逃生舱（CEL）在出现真实需求时评估。
