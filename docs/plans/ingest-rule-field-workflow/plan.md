# 类型驱动的解析规则实施计划

行为与验收见[规格](spec.md)。本次实施覆盖事件类型、目标优先映射、字段处理、规则校验和规则列表。后续来源与 Profile 的范围见覆盖表。

## 规则详情与列表联调

1. 使用同一向导渲染详情与编辑，记录地址默认打开详情，`edit=1` 请求编辑并由权限控制。移除页面的 JSON 下载，复用记录读取接口。 [Next.js 导航](https://nextjs.org/docs/app/getting-started/linking-and-navigating)
2. `GET /mappings/:id/preview` 只接受规则 ID，用已保存的定义运行受限样本并复用提取、事件预览及敏感值遮盖。读取仅访问主键对应的记录，不获取写锁或保存试算结果。点击后按规则版本缓存，失败可以重试。草稿试算仍由写权限保护。
3. 列表摘要以 `class_uids` 表示可配置的事件类型，过滤参数为 `class_uid`。从既有 JSONB 分支派生，查询复用同一结果计算筛选和摘要。「数据接入」列由 `GET /inputs/counts-by-rule` 返回规则对应的接入数量。查询在 `ingest.inputs.mapping_id` 索引上按当前页规则统计，结果使用 `input_count`，不落库。两张表均为管理配置，按已有名称顺序读取和限制分页。搜索和总数仍遍历符合基本条件的管理记录，事件筛选需要读取规则的少量分支。若配置规模使实际查询超出延迟目标，再以实际执行计划决定索引。不复制类编号或引用数到表字段。 [PostgreSQL JSON 函数](https://www.postgresql.org/docs/current/functions-json.html)
4. 在真实数据库验证搜索字段、事件筛选和绑定过滤。在浏览器核查四步只读与编辑、引用跳转、两个筛选器和默认分页。在真实路由验证预览读权限及草稿试算权限。

5. 搜索下拉继续复用模板的紧凑筛选布局。使用 Base UI 的弹层内输入和触发器定位，浏览器验证选项单行、窄屏定位、空输入搜索、取消恢复选中值及键盘筛选。 [Base UI Combobox](https://base-ui.com/react/components/combobox)

## 契约依据

Event 的唯一领域标准是 **OCSF 1.10.0**，锁定提交为 `2d209039d9c476ab76e83fb58b6f99374d40a994`。类别、事件类、活动、字段定义、枚举、继承、对象约束和 Profile 都由该提交提供。公共映射和条件组从同一版本的编译结果派生。每项纳入目录的标准定义须能定位到该版本的类、字典、对象或 Profile。 [上游版本](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/version.json) · [Base Event](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/base_event.json) · [Network 基类](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/network.json#L116-L121) · [Authentication 类](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/authentication.json#L144-L154)

各数据源的格式、字段和编码按来源版本在接入模板中转换。外部产品资料用于找样本、确认原始日志用法、评估覆盖优先级和交互。第一步选类型、第三步先列目标字段及使用 Shadcn Dashboard 组件属于 SecPlexus 界面设计。

## 事件类型命名与覆盖范围

### 标准类、具体活动和运营名称

OCSF 将类别、事件类、具体活动及结果分别编码。类别编号和英文名取自锁定版本，中文名统一在 `event-contract.json` 的 `category_labels` 中维护。已发布的类别为系统活动、安全发现、身份与访问管理、网络活动、应用活动。系统活动参照国内厂商的表述。Findings 涵盖漏洞、检测及其他安全产品发现，平台中文采用「安全发现」。OCSF 类别 4 和事件类 4001 的英文名都叫 Network Activity，选择器按类别标题和事件选项区分。 [OCSF 类别源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/categories.json) · [Network Activity 源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/network_activity.json) · [火山引擎分类示例](https://docs.volcengine.com/docs/multi-cloud-security-platform/log-management-overview?lang=zh)

| 操作者常用叫法 | 标准表示及界面用词 |
|---|---|
| 登录事件、登录成功、登录失败 | 事件类「认证活动」对应 Authentication `3002`。登录是本类 `activity_id=1`、`type_uid=300201`，成功和失败用 `status_id=1/2`。登出与票据相关行为也在本类。允许搜索「登录事件」「身份认证」，列表或预览显示「登录」「登录失败」等动作与结果。 [OCSF Authentication 源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/authentication.json) |
| 防火墙流量、连接被拒绝 | 可归为网络活动 `4001`。连接建立、关闭和流量等由本类活动编码表达，连接被允许或拦截由安全控制处置表达。设备名、拒绝动作或严重度单独出现时，证据不足以认定攻击。 [OCSF Network Activity 源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/network_activity.json) |
| 网络攻击、入侵检测、WAF 告警 | 作为场景名称，从实际报文判断。安全设备直接记录的一次 HTTP 或网络活动，可在活动类上叠加 Security Control Profile，表达检测证据与处置。上游分析产品明确输出独立检测发现时使用 Detection Finding `2004`，提供发现 ID 和对应上下文。只记录 HTTP 请求或处置的 WAF 日志按 HTTP 活动映射。来源提供的攻击类型、签名、证据和处置按契约保留。平台 Alert 的生成仍属于告警模块。 [Security Control Profile 源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/profiles/security_control.json) · [Detection Finding 源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/findings/detection_finding.json) |

### 当前交付与下一批

当前 `v1` 字段目录包含 **11 个业务类与基础事件**。已发布类可用于自定义映射。新安装从空清单开始，此前内置规则及其修订由迁移移除，未来的来源模板另行定义。迁移按旧规则主键和接入绑定索引定位记录，在事务内解除绑定、停用对应接入并删除规则与修订。其他接入和自定义规则照常可用。重复运行不会重新创建模板。

账号变更 `3001` 继续可读，用户管理 `3007` 已纳入同一目录。两类分别使用上游枚举，活动码 `2` 在 `3001` 表示启用，在 `3007` 表示更新。迁移既有规则时需明确重映射。 [Account Change 弃用条目](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/account_change.json) · [User Management 定义](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/user_management.json)

| 优先顺序 | 场景与交付内容 |
|---|---|
| 来源格式依据 | Nginx 默认 combined 日志的请求路径、方法及响应状态码，输出 HTTP 活动 `4002`。Winlogbeat 的 Windows Security 4624 和 4625 输出认证活动 `3002`，按上游含义区分成功和失败。OpenSSH 的已完成认证结果同样输出 3002，保留源地址和端口。 [OpenSSH 日志实现](https://github.com/openssh/openssh-portable/blob/V_9_9_P2/auth.c) [Nginx combined 格式](https://nginx.org/en/docs/http/ngx_http_log_module.html#log_format) · [Winlogbeat 字段](https://www.elastic.co/docs/reference/beats/winlogbeat/exported-fields-winlog) · [Windows 登录审计](https://learn.microsoft.com/en-us/troubleshoot/windows-server/windows-security/kerberos-authentication-log-analysis-test-scenario) |
| 后续来源模板 | Sysmon 进程和文件操作、DNS 查询、WAF、CloudTrail、EDR 与漏洞扫描。Suricata 8 的 DNS v3 把查询改为数组，模板需先验证数组形状和查询对象如何对应。 [Suricata DNS 格式变化](https://github.com/OISF/suricata/blob/suricata-8.0.0/doc/userguide/upgrade/8.0-dns-logging-changes.rst) · [Sentinel Sysmon 解析器](https://github.com/Azure/Azure-Sentinel/blob/aff6947fe7f115f553bbf420aef45a5712cda36c/Parsers/ASimProcessEvent/Parsers/ASimProcessCreateMicrosoftSysmon.yaml) · [Security Lake 来源与 OCSF 类](https://docs.aws.amazon.com/security-lake/latest/userguide/open-cybersecurity-schema-framework.html) |
| 第一批字段补齐 | 核对安全控制 Profile 需要的攻击信息、检测证据和处置。目前本地目录在网络类可写 `action_id`，检测发现主要支持 `finding_info.uid/title/desc`，还无法表达完整的 IDS/IPS 证据。先用 Suricata 的普通流量与独立检测样本验证类和 Profile 的边界。 [Security Control Profile 源码](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/profiles/security_control.json) |
| 下一批类 | 身份与权限优先核对用户管理 `3007`、组管理 `3006` 和会话授权 `3003`。按数据源引入 DHCP `4004`、SSH `4007`、邮件 `4009`、数据存储活动 `6005` 和合规发现 `2003`。SSH 连接遥测与 SSH 登录仍各按其实际语义映射。Windows 注册表和资产清单随专项数据源评审。 [锁定的 IAM 类源码](https://github.com/ocsf/ocsf-schema/tree/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam) · [锁定的网络类源码](https://github.com/ocsf/ocsf-schema/tree/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network) |

验收分别记录「目录可映射」「已有来源模板」「端到端已验证」。每项新增类或模板配置先核对官方类、活动枚举、字段要求和来源版本，再用一条正常样本和一条缺项、字段冲突或易误判样本验收。需要告警或分析用途时补可复验查询。目录与落库使用 Ingest 的同一发布契约验证。

## 现状审查与核心字段底表

本表从当前生成目录读取，额外核对了 normalizer 的 `incompleteClass` 和测试。表中记录的是当前平台执行的**具体类字段组**，公共输出字段另列于规格。平台局部约束及本地发布子集需要在实施前逐项复核上游出处。

| 当前事件类 | 当前目录的核心约束 |
|---|---|
| 基础事件 `0` | 无额外类级必填组。 |
| 文件系统活动 `1001` | 设备主机名或设备标识、设备类型、操作者账号名称或账号 ID、文件名和文件类型。 |
| 进程活动 `1007` | 设备主机名或设备标识、设备类型、操作者账号名称或账号 ID、进程 PID 或标识。 |
| 漏洞发现 `2002` | 发现 ID 和非空漏洞列表。列表项按契约校验。 |
| 检测发现 `2004` | 发现 ID。 |
| 账号变更 `3001` | 发生变更的账号名称或账号 ID。上游已弃用，后继类为用户管理 `3007`。 |
| 认证活动 `3002` | 认证用户，认证服务或目的端点。 |
| 用户管理 `3007` | 被管理的账号名称或账号 ID。 |
| 网络活动 `4001` | 至少一个有效端点。当前实现接受 IP、MAC、主机名、域名或端点 ID 作为端点身份。 |
| HTTP 活动 `4002` | HTTP 请求或响应，有响应时要求 HTTP 状态码。 |
| DNS 活动 `4003` | 至少一个有效端点，有查询对象时要求查询主机名。目录中叶子的 `required` 必须放到父对象语境下解释。 |
| API 活动 `6003` | API 操作、操作者账号名称或账号 ID、源 IP，有云对象时要求云提供商。后者同样依赖父对象。 |

重点修复四处可能产生漂移的边界。

1. 目前 Web 的 `groupOf` 按路径前缀硬分组。第三步共享映射查询全局字段，分类区另取类字段。改为通过目录取得事件类、可写路径、约束和推荐项。本类选择器用类视图，全局路径索引用于检索，后者的 `usages` 保留类内语义。
2. Web 类详情 DTO 当前只解析 `fields`，未保留 Go 已返回的 `constraints`。生成器目前把上游字段定义和本地 `constraints` 拼装在一起，运行时还有端点身份及枚举伴随字段检查。建立可消费的结构化约束，将文案和规则来源一起生成。复核上游继承、对象身份、数组元素及可选 Profile 后再变更判定；条件文案专用于解释。
3. 维护同路径多类的语义差异。例如 `activity_id=1` 在认证活动表示登录，在网络活动表示打开连接；`status_id` 在常见活动中表示操作结果，在安全发现中可表示发现状态。显示、默认值与转换均由所选类型取得枚举，禁止沿用全局去重视图的首个同名字段值域。
4. `metadata.uid` 当前由 pipeline 写入，目录已标记为平台生成，可作归属检查样例。逐项核对其他字段的 `owner`、受控扩展、时间回退和自动生成规则，避免向用户提供实际会覆盖的输入，也避免把可选择来源的时间行隐藏。补齐当前全部支持类型的最小正反样本，再调整推荐清单。新事件类按有实际数据源、可满足条件和有下游用途纳入。

## 业务、技术和数据边界

### 业务与向导

新建自定义规则一条规则对应一个 OCSF 事件类，在第一步与解析类型同排配置。来源可能承载多种日志时，可在上游按已有的来源类型或流分开接入。历史规则和内置模板继续使用已保存的匹配分支。日志分流只在这些配置需要时呈现。选择器按上游 `category_uid` 分组，显示发布目录的类别名。网络活动类别列出独立的网络活动、HTTP 活动和 DNS 活动三个事件类型。

调研采用同一 OCSF 版本的类别和输出编码。火山引擎文档同时列出分类 4 和事件类型 4001，并将后者解释为网络连接和流量。阿里云按目标标准化结构组织规则，腾讯云在接入层选择日志类型和解析策略组。AiSOC 在源配置和 normalizer 内判定输出类，使用按来源维护的事件目录。源端分流方式用于接入参考，SecPlexus 的标准字段与枚举仍由锁定的 OCSF 目录决定。 [火山引擎日志字段](https://docs.volcengine.com/docs/Multi-cloudsecurityplatform/LogManagementOverview-1?lang=zh) · [阿里云标准化规则](https://help.aliyun.com/zh/security-center/user-guide/standardized-log-access-rules) · [腾讯云接入流程](https://cloud.tencent.com/document/product/1011/112763) · [AiSOC normalization](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/services/ingest/internal/normalizer/normalizer.go)

分类配置切换之间保留当前草稿。切换到别的类时先核对映射路径、归属、类型、单位、基数、约束和枚举含义，用户确认后才应用。跨分支共享映射只配置一次，适用于被分配的类，且在那些类里语义、类型和值域兼容。公共路径存在于多类也可能需要各类单独配置，例如活动类型和发现状态。

### 契约、接口和存储

| 层 | 责任 |
|---|---|
| Ingest 契约源 | `schema/events/event-contract.json` 固定唯一上游版本与提交。支持的类、字段、值域和标准约束从该 OCSF 版本选取。双语标签、推荐映射和平台补充限制各自标注来源和用途。 |
| 发布目录 | `generate.py` 生成 `catalog.json`。类详情提供 `class_uid`、字段归属、标签、类型、单位、基数、用途、类内枚举和映射工作区所需约束。共同的输出条件只定义一次，再为各视图派生。CI 校验生成可复现、目录引用完整、代码与文案对应，并检查常见有效路径可配置。 |
| Ingest API | 沿用 `/event-schema`、`/event-schema/classes/:class_uid` 和 `/event-schema/fields`。目录按 `contract_version` 和 `catalog_revision` 区分；同一草稿用一致修订。后端在预览和写入分别执行相同的结构检查，返回字段或约束位置及稳定原因码。遇到目录漂移刷新后重验，规则并发仍用 `expected_version`。 |
| Web | 保留现有 IAM 权限、Query 单一数据入口和 Zod DTO。RHF 根据目录构建可写、自动、分组条件等行，能即时标注简单错误；服务器是完整校验的裁决方。业务文案留在目录和统一术语，组件中不维护事件类型、枚举、核心路径或本地别名清单。国际化搜索允许别名命中，保存只存标准路径和值。 |
| PostgreSQL | 继续保存规则和修订中的解析器、共享映射和分支定义。单一类型可由一条无匹配条件的现有分支表达，基础事件沿用无分类分支，混合来源沿用有序分支。升级识别已存同类多分支实例时仍显示分流。写入现有作用域内保持目标唯一及源到目标的协议约束。字段中文名、展示分类和枚举字典不复制进规则或独立数据库表。 |
| Kafka、ClickHouse | 同一条记录输出一个 Event，正文按 OCSF 路径、数字枚举和 UTC 语义发布。解析状态、时间来源和候选类继续在 `metadata.spx` 或既有投影中表达。归档维持现有物理列和 JSON 投影；与新字段绑定时按路径、类型、范围核对后再批准 DDL。 |
| 查询、检测和后续模型 | 从 Ingest 公共契约获取有效字段，按版本、`class_uid`、标准路径和枚举解析。模型上下文只提供本任务所需类、路径、单位、枚举和缺失或回退语义；输出结构和引用再用同一契约验证。模型建议要经过样本和规则校验，不能创建新编码。平台调用统一经 llm-gateway。 |

平台契约 `v1` 对应一个锁定的 OCSF 版本。显示名、搜索别名及排序变更采用目录修订。升级 OCSF 时以完整目标版本重新生成目录并评估契约差异，统一验证模板、规则校验、归档和消费者，再按批准的兼容方案发布。模板推荐来源与规则定义有审计和修订；用户临时手补示例继续留在 Web 会话。跨服务通过 Ingest API 访问目录。

### 术语与枚举审核

本次按 [AGENTS.md 的行业标准与领域语义规则](../../../AGENTS.md#industry-standards-and-domain-semantics)评审。评审记录注明采纳版本、具体条款或源码、适用范围和选择理由。标准、行业惯例及平台补充约束分别标明依据。这样可以让操作者按熟悉的术语配置，让接入、查询和检测使用一致的语义，也能为模型提供可核验的字段、单位和枚举。

上游类名、稳定命名和编码按锁定 OCSF 对照。中文结合标准原义、国内安全运营与常用查询用法审定。例 `dst_endpoint`「目的端点」与 `dst_endpoint.ip`「目的 IP」统一，现存「目标 IP」可用作搜索别名；`metadata.uid`「事件 ID」与 `metadata.spx.input_id`「接入点 ID」区分。认证活动描述实际认证行为，启用「登录失败」时真值仍来自活动和操作结果。审核表逐类覆盖英文名、中文名、字段含义、逻辑与 JSON 类型、枚举编码和含义、条件、单位、归属、上游出处及平台扩展依据。去掉复述同义或只差标点的展示说明。

验收时首先核对类 UID、活动、路径、类型、单位、枚举和标准约束均来自同一锁定的 OCSF 编译结果，再检查目录生成、API、向导、编译与运行校验、ClickHouse 投影及下游读取。重点用同路径不同作用域的枚举、条件必填和单位转换做正反例。只改中文展示名时验证调用方仍使用同一标识。语义、值域或约束变更先核对对在用规则、已有数据和下游消费者的影响，再确认兼容与迁移方式。

## 次序

1. **先核对契约。** 对现行类编译上游约束、枚举及父对象语义，逐项审定中文和平台限制。确定默认展示的公共与类专属字段，优先保证所有必填组在页面上有可选路径。用目录与 DTO 的回归夹具检查生成和 Web 读取均保留条件约束，先验证只读接口。
2. **类型和目标优先的向导。** 新建规则在第一步选一个事件类型。已有多类型规则在第二步处理日志分流。第三步实现固定标准行、源字段选择、类内添加和字段详情。按当前选择加载类字段，复用已查询的类详情。保留复制和读取现有定义。补充源字段批量抽屉复用 RHF、Zod、`useFieldArray` 和 Field 控件。
3. **配置校验和运行校验。** 配置阶段确认字段组可满足、同作用域目标和路径无冲突、类型和默认值合法、分流可达。编译期保留现有运行时完整性行为，逐条样本和真实运行校验实际值，缺失和非法结果走既有诊断。服务端复用验证路径并返回位置。先清点在用规则和模板能否通过新的写入检查，必要时提供就地修正入口；已存在的修订继续按当前格式可读。
4. **字段处理和默认值。** 扩展 Go 有序处理计划和既有规则 DTO。保留 `trim`、`json` 行为；新处理优先复用 Go 标准库。默认值编译到映射投影，按源路径存在性选择，在类型、值域及类作用域通过校验后可用。核查现有 PG 顶层 JSON 约束，沿用修订事务。
5. **前向发布。** 先交付验证和回读能力，再打开新界面及规则写入。提前核对模板、修订与草稿的来回转换。新操作和默认值启用前保证所有 Ingest 实例已能读取其定义。回退部署前核对新定义是否已被使用，沿用支持读新规则的运行版本和历史规则修订恢复路径。已应用迁移保留。

## 性能与交付

- 目录规模以发布类和字段规模为上限，运行使用不可变内存索引。类详情按需查询，不全量拉取供每行使用。每条日志解码一次，沿用有界分支和映射计划；默认值与约束在刷新时编译。热路径没有新增数据库查询、锁或模型请求。队列、Kafka 重试和 ClickHouse 归档沿用现有边界。
- 管理面沿用版本冲突、事务和审计，试算沿用样本与报文大小上限，限制解码输入和输出。配置刷新失败保留上一个可用快照。目录版本冲突或试算失败保留草稿并提示恢复，重复验证无持久化副作用。
- Go 单测覆盖类内枚举、条件组、缺失、空值、非法值、零值、数组和敏感值隔离。Web 单测覆盖目录解码、切换、动态必配及字段错误定位。真实浏览器和隔离组件验证单一类型与既有混合模板，包含键盘选择、添加额外映射、发布、Kafka 与 ClickHouse 回查和修订重开。基础事件作为运行时诊断验证。目录及约束变更前后用同组公开样本对照。

字段目录、解析器和测试共享 `v1`，规则保存记录选用的目录修订。常用映射的展示顺序由类详情提供，条件组仍按完整契约校验。已持久化规则加载时仍按自身定义编译。新增模板通过 PostgreSQL 前向迁移注册。ClickHouse 前向迁移统一目的端点的两处列注释，物理列名和类型沿用已有设计。

## 字段操作与预览布局

预览参考 Dashboard 模板的 Profile 线型页签和主内容加侧区、File Manager 的紧凑表格、Invoice 的独立预览区。字段结果是默认视图，JSON 通过同一预览的页签访问。侧区保留当前日志样本和切换入口，窄屏按阅读顺序纵向排列。裁剪与解码沿用管理抽屉和提取 API。点击预览提交一条当前样本，复用请求上限和脱敏流程，无写入或额外数据库查询。

OCSF HTTP 活动的 `activity_id` 包括基础与扩展方法，常用组的顺序属于界面偏好。OCSF 的 `http_response.code` 是整数。目录以 RFC 9110 第 15 节的范围校验源值与配置默认值，从 IANA 登记表提供一组常用值，扩展码仍可按整数输入。此变更随已有 `v1` 更新目录修订。 [OCSF HTTP 活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/http_activity.json) · [OCSF HTTP 响应](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/http_response.json) · [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#section-15) · [IANA 状态码登记表](https://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml)

隔离 Chromium 已验证裁剪和解码预览、末尾新增映射、认证枚举及 HTTP 状态码。实际样本中的 `42` 在对应行报告错误，缺失状态码的样本使用配置的 `499`。窄屏可在预览标题处换样本，点「返回修改」聚焦相关源字段。桌面明暗主题已走查，Go、Web 单测和目录生成一致性检查已通过。

## 逐类映射审查

按锁定提交编译继承定义，对 11 个业务类和基础事件的 381 条类内字段定义逐项核对路径、逻辑类型、数组和枚举编码。平台元数据保留在 `metadata.spx`。生成器检查所有上游必填根字段在目录中有覆盖。目录只发布已经支持的字段。字段嵌套条件以对应对象的定义为准。 [OCSF Base Event](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/base_event.json)

公共预置为活动、严重度和时间，由规则映射或平台补齐。「事件消息」及结果 ID 默认作为可移除的推荐。安全发现使用该类的发现状态枚举。桌面和窄屏沿用 Dashboard 的 Invoice 行内映射结构及平台已有值类型标签。标准字段显示 `integer`、`IP`、`timestamp` 或数组元素类型，源字段标签表达样本的实际类型。可立即识别的样本类型冲突显示在所在行，试算继续检查值域和转换。

| 事件类 | 核心条件与默认选配 |
| --- | --- |
| 文件系统活动 `1001` | 设备、操作者和文件身份。默认选配文件路径、文件大小。 [OCSF 文件活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/system/file_activity.json) |
| 进程活动 `1007` | 设备、操作者与 PID／UID／CPID 之一。默认选配进程名称和命令行，父进程 PID 可添加。映射可执行文件对象时需要同时提供文件名称和类型。旧定义若只映射可执行文件路径，需要补齐这两项。 [OCSF 进程](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/process.json) · [OCSF 文件对象](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/file.json) · [Sentinel Sysmon 映射](https://github.com/Azure/Azure-Sentinel/blob/aff6947fe7f115f553bbf420aef45a5712cda36c/Parsers/ASimProcessEvent/Parsers/ASimProcessCreateMicrosoftSysmon.yaml) |
| 漏洞发现 `2002` | 发现 ID 与漏洞数组。默认选配标题、置信度。漏洞条目当前支持 CVE，advisory 和 CWE 为目录扩展缺口。 [OCSF 漏洞发现](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/findings/vulnerability_finding.json) · [漏洞对象](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/vulnerability.json) |
| 检测发现 `2004` | 发现 ID。默认选配标题、置信度与告警标记。资源与检测证据数组仍在扩展计划中。 [OCSF 检测发现](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/findings/detection_finding.json) |
| 账号变更 `3001` | 保留历史账号类型和枚举，至少配置账号名称或 ID。默认选配操作者、源 IP。 [OCSF 历史账号变更](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/account_change.json) |
| 认证活动 `3002` | 认证用户与服务／目的端点之一。默认选配源 IP、登录类型，认证协议和 MFA 可添加。 [OCSF 认证活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/authentication.json) · [Winlogbeat 安全日志映射](https://github.com/elastic/beats/blob/083cd5e5501cbe24195a6f52a3214b13b0448a81/x-pack/winlogbeat/module/security/ingest/security_standard.yml) |
| 用户管理 `3007` | 至少配置被管理用户名称或 ID。默认选配操作者、源 IP。 [OCSF 用户管理](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/iam/user_management.json) |
| 网络活动 `4001` | 源或目的至少一侧有标识。默认选配主机名、端口、协议。输出连接对象时补齐缺省的「未知」方向。既有端点允许用域名作为平台补充识别方式，上游 Endpoint 的身份字段组合更广。 [OCSF 网络活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/network_activity.json) · [连接对象](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/network_connection_info.json) · [端点对象](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/endpoint.json) |
| HTTP 活动 `4002` | 请求或响应至少一项。响应必须有状态码。启用 URL 对象时，必须提供请求路径或完整 URL。默认选配请求主机名、源 IP、用户代理。仅有请求主机名的旧定义需要补齐 URL 映射。 [OCSF HTTP 活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/http_activity.json) · [URL 对象](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/objects/url.json) · [Nginx 日志格式](https://github.com/nginx/nginx/blob/0e0c6edd90593a43d05ef921c046cf284035b1ed/src/http/modules/ngx_http_log_module.c) |
| DNS 活动 `4003` | 源或目的至少一侧有标识，出现查询对象时配置查询域名。默认选配记录类型和响应码。响应记录数组待扩展。 [OCSF DNS 活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/network/dns_activity.json) · [Suricata EVE DNS 格式](https://github.com/OISF/suricata/blob/2dd24e7818f996b797e3cd280d5ff7e5d9e2a8a3/doc/userguide/output/eve/eve-json-format.rst) |
| API 活动 `6003` | 操作者、API 操作和源端点，源端点可用 IP 或主机名。默认选配服务名、云提供商、来源结果代码。 [OCSF API 活动](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/application/api_activity.json) |

完整的 OCSF 扩展对象、结构化证据以及嵌套对象的所有备选键仍按来源样本逐步发布。当前规则编辑保留已映射字段，已移除的推荐行保留在当前草稿状态，可通过添加映射找回。事件目录和字段条件使用同一契约修订。
