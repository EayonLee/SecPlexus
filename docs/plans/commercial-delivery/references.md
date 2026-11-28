# 同行方案和技术依据

核对日期：2026-11-27。公开源码按 GitHub 已读取的文件或入口引用。厂商文档用于确认操作行为和术语。涉及各厂商内部实现的结论限于其公开代码。SecPlexus 的条件、阈值和验收是本项目设计。

## 发行、安装与恢复

| 参考 | 已核对的做法 | SecPlexus 采用的做法 |
| --- | --- | --- |
| **AiSOC**，开源 AI SOC | GitHub `release.yml` 先按摘要推送构建产物，再组装和验证发布清单。验签文档有镜像签名、SBOM 和构建溯源。干净安装记录包含真实 Kafka Admin 检查和端到端冒烟，曾出现容器健康而 Kafka 不可用的情况。[发布工作流](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/.github/workflows/release.yml#L484) · [制品验证](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/docs/operations/verifying-releases.md) · [干净安装记录](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/docs/testing/CLEAN_INSTALL.md) | 构建一次，按摘要分发，装包前确认证书、依赖、磁盘和主题。用同一制品从接入走到实际存储。 |
| **Wazuh**，开源 SIEM / XDR | `single-node/docker-compose.yml` 指定各组件镜像、持久卷、证书、凭据和依赖健康条件。Docker 升级说明将配置、证书、卷和有顺序的组件替换分别说明。中心组件备份手册含配置和数据目录。[Compose 源码](https://github.com/wazuh/wazuh-docker/blob/main/single-node/docker-compose.yml) · [Docker 升级](https://documentation.wazuh.com/current/deployment-options/docker/upgrading-wazuh-docker.html) · [中心组件备份](https://documentation.wazuh.com/current/migration-guide/creating/wazuh-central-components.html) | 发行包提供受支持的单机拓扑、重复安装及升级手册。永远按组件所有权核对需保存材料。SecPlexus 的额外恢复边界是规则修订与 Kafka 位移。 |
| **Malcolm**，开源网络流量分析平台 | 安装代码按组件和部署环境导入参数与检查，通过常量区分日志、索引与快照目录。项目提供 Compose 等部署选择，分析 Zeek、Suricata、Arkime 数据。[安装代码](https://github.com/idaholab/Malcolm/blob/main/scripts/install.py) · [项目说明](https://github.com/idaholab/Malcolm/blob/main/README.md) | 安装参数、运行参数和证据留存的归属明确。支持隔离网络时交付导入及验证方式，缓存或可拉取的版本号不能代替清单中的镜像摘要。 |
| **Docker、PostgreSQL、ClickHouse**，组件官方文档 | Compose 支持生产配置与摘要引用。PostgreSQL 备份区分逻辑备份和持续归档。ClickHouse 提供备份与恢复机制。三种组件能力各有独立成功条件。[Docker 部署](https://docs.docker.com/compose/how-tos/production/) · [Docker 摘要和信任](https://docs.docker.com/compose/trust-model/) · [PostgreSQL 16 恢复](https://www.postgresql.org/docs/16/backup.html) · [ClickHouse 恢复](https://clickhouse.com/docs/operations/backup/overview) | 项目交付用一次性迁移、恢复点记录和整套恢复演练，把各组件条件串起来。原始范围、允许损失和实际恢复耗时在演练后才能写进正式规格。 |

对比后的决定：采用 Compose 和版本化发行清单。安装入口负责真实检查，应用只拥有自身配置和数据语义。AiSOC 的多角色基础设施和 Malcolm 的多编排器入口都有各自的使用场景。SecPlexus 本次以一套可交付、可升级、可恢复的部署组合为验收对象。

## 入口信任与数据最小化

| 参考 | 已核对的做法 | SecPlexus 采用的做法 |
| --- | --- | --- |
| **NGINX Real IP、Gin** | NGINX 通过 `set_real_ip_from` 明确可信代理，按配置解释真实客户端地址。Gin 在已知可信代理配置下才依赖其转发的客户端地址。[NGINX Real IP](https://nginx.org/en/docs/http/ngx_http_realip_module.html) · [Gin SetTrustedProxies](https://pkg.go.dev/github.com/gin-gonic/gin#Engine.SetTrustedProxies) | 入口、Web 和 IAM 分别有明确网络边界。Web 转发的来源必须从可信入口进入，在实际 Compose 入口验证限流和审计。 |
| **Elastic ingest、阿里云威胁分析与响应** | Elastic 用输入文档试算管道。阿里云可选已采集日志样本，或在 JSON 编辑器里临时输入并验证。文档描述标准化日志和原生日志的不同用途。[Elastic 试算](https://www.elastic.co/docs/api/elasticsearch/v8/operation/operation-ingest-simulate) · [阿里云日志结构](https://help.aliyun.com/zh/security-center/user-guide/log-integration-overview) · [阿里云样本与规则](https://help.aliyun.com/zh/security-center/user-guide/standardized-log-access-rules) | 试算、规则定义和运行证据分别授权和存储。SecPlexus 的默认生命周期由本项目确定：规则及修订只落结构化定义，试算原文留在请求和当前编辑会话。 |

## 规则试算、字段映射与修订

| 参考 | 已核对的做法 | SecPlexus 采用的做法 |
| --- | --- | --- |
| **阿里云 Agentic SOC / CTDR** | 按数据源加载样本，源字段可选。必须配置的目标字段由标准结构确定，其余按需映射，另有标准字段和映射字典入口。[接入安全厂商日志](https://help.aliyun.com/zh/security-center/add-security-provider-log/) · [解析规则](https://help.aliyun.com/zh/security-center/user-guide/standardized-log-access-rules) | 以平台标准字段为左侧列。源字段选择、枚举默认值、必需项和常用项来自同一目录。页面术语采用「解析规则」「源字段」「标准字段」。 |
| **华为云 SecMaster、腾讯云 SOC、深信服 XDR** | SecMaster 把来源连接、日志管道和解析器分开配置。腾讯云日志接入区分接入方式、端口、解析策略和编码。深信服公开 XDR 手册把 Syslog、Kafka 等列为第三方日志接入方式，后续另核对原始日志和富化结果。[SecMaster 解析器](https://support.huaweicloud.com/intl/zh-cn/usermanual-secmaster/secmaster_01_0228.html) · [腾讯云 SOC 快速入门](https://cloud.tencent.com/document/product/1011/112763) · [深信服 XDR 用户手册，数据接入章节](https://mkp-res.hc-cdn.com/marketplace/public/appv2/attachment/8DE/D43/B48/00000000008DED43B48.20250912022343.f93ee16c8d874da5a545cadec779ff80.pdf#page=98) | 将接入协议、解析类型和事件类型分开看待。保持数据接入绑定解析规则的现有关系，选项名和操作名采用国内运营人员常用的短词。 |
| **Elasticsearch、Elastic Integrations** | 源码 `Pipeline.java` 持有 `version` 及编译后的处理器，官方文档说明 `if_version` 条件更新和输入文档试算。集成开发指南将样本与 pipeline 测试作为回归。[Pipeline.java](https://github.com/elastic/elasticsearch/blob/main/server/src/main/java/org/elasticsearch/ingest/Pipeline.java) · [管道版本与试算](https://www.elastic.co/docs/manage-data/ingest/transform-enrich/ingest-pipelines) · [集成测试](https://www.elastic.co/guide/en/integrations-developer/master/pipeline-testing.html) | 延续预期修订比较，服务使用最终提交的定义验证样本。将手工试算和脱敏夹具纳入同一解析行为验收。 |
| **Cribl Stream** | 版本控制区分提交与部署，分布式模式可按 Worker Group 提交、部署和恢复历史提交。[版本控制和恢复](https://docs.cribl.io/stream/4.15/version-control/) | 保留版本记录与生效观测的区别。当前工作流只需一个「保存并应用」，由 Ingest 记录受影响接入和各自的实际状态。恢复创建新修订并留审计。 |

阿里云的安全域、Elastic 的生态字段和 OCSF 的类别层次解决的问题不同。SecPlexus 只使用 OCSF 定义事件结构和编码。厂商界面用于对齐操作和中文词汇，字段路径、枚举数值与必填约束始终来自本项目锁定的规范及明确的本地规则。前端继续以本项目已有模板组成页面。

## 接入诊断与运行观测

| 参考 | 已核对的做法 | SecPlexus 采用的做法 |
| --- | --- | --- |
| **腾讯云 SOC** | 日志源支持启停和查看日志，系统把最近 15 分钟无上报的来源判断为不活跃。[系统管理](https://cloud.tencent.com/document/product/1011/113921) | 接入启停归配置，活跃度归观测。显示窗口和最近确认时间。对低频来源使用部署配置的窗口。 |
| **Microsoft Sentinel** | Data & connectors 展示按窗口的接入量、异常连接器、失败类型、失败次数和最近成功接入。部分连接器通过 `SentinelHealth` 提供诊断。[连接器健康](https://learn.microsoft.com/en-us/azure/sentinel/monitor-data-connector-health) | 页面区分「正常、最近无数据、暂不可确认、处理故障」，同时给出失败所在阶段和最近成功证据。观测数据过期单独表达。 |
| **AiSOC、Splunk Edge Processor** | AiSOC 的 lag 手册按消费组和分区排查，将处理速率、消费失败和积压分开。Splunk Edge Processor 的指标文档按实例和数据路径暴露指标。[AiSOC 消费积压排障](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/docs/runbooks/kafka-consumer-lag.md) · [Splunk 指标参考](https://help.splunk.com/en/splunk-cloud-platform/process-data-at-the-edge/use-edge-processors-for-splunk-cloud-platform/monitor-system-health-and-activity/edge-processor-metrics-reference) | 复用现有指标和日志。来源详情从服务端汇总得到有界窗口。运行手册先定位收包、解析、Kafka 确认、归档哪一段失去进度，再核对相应分区或实例。 |

## 标准事件与重放

| 参考 | 已核对的做法 | SecPlexus 采用的做法 |
| --- | --- | --- |
| **OCSF 1.10.0、OCSF Toolkit** | 锁定的 Metadata 定义将 `product` 标为 required，`original_event_uid` 表达来源系统分配的标识。网络类下的 `network_activity` 是具体事件类。Toolkit 以编译 schema 独立校验单条事件、版本、枚举和 `type_uid` 关系。[Metadata 1.10](https://raw.githubusercontent.com/ocsf/ocsf-schema/1.10/objects/metadata.json) · [Network Activity 1.10](https://raw.githubusercontent.com/ocsf/ocsf-schema/1.10/events/network/network_activity.json) · [Toolkit 校验](https://github.com/ocsf/ocsf-toolkit/blob/main/docs/validation.md) | 编译目录是映射配置的权威。实际事件样本另用锁定 schema 检验，保证嵌套字段和条件也覆盖。来源原生 ID、平台来源坐标和标准事件 ID 各用对应字段。如果来源只提供 Kafka 坐标，平台把它放进自己的扩展。 |
| **Zeek / Malcolm**，网络遥测 | Zeek `conn.log` 记录 `uid`，可借它关联同一连接上的协议日志。Malcolm 支持以不同类型元数据关联和检索网络遥测。[Zeek conn.log](https://docs.zeek.org/en/current/logs/conn.html) · [Malcolm 项目说明](https://github.com/idaholab/Malcolm/blob/main/README.md) | 网络来源原生关联标识具有自己的作用域。保留发行方的事件或连接标识，平台按独立的 OCSF 事件 ID 标识规范化结果。 |
| **Apache Kafka、franz-go、Splunk** | Kafka 的位移策略在没有位移或已保存位移失效时生效。franz-go 区分起始设置和后续位移重置。Splunk 的确认机制说明在确认未知时重投可以出现重复。Kafka 官网提供现架构基线 4.2.2 的发行物。[Kafka 发行](https://kafka.apache.org/community/downloads/) · [Kafka 消费配置](https://kafka.apache.org/42/configuration/consumer-configs/#auto.offset.reset) · [franz-go Offset API](https://pkg.go.dev/github.com/twmb/franz-go/pkg/kgo#ConsumeResetOffset) · [Splunk 在途数据确认](https://help.splunk.com/en/splunk-enterprise/forward-and-process-data/forwarding-and-receiving-data/9.2/perform-advanced-configuration/protect-against-loss-of-in-flight-data) | 首次位移、延迟、重试、超期和人工回放有不同流程。外部 broker 和平台分属两个确认域。版本切换使用已确认的分区边界，避免自动重试误用新规则。 |
| **ClickHouse**，Context7 核对官方文档 | `ReplacingMergeTree` 根据完整排序键在后台合并时去重，查询需要明确的 `FINAL` 或等价操作才能提供即时准确值。[ReplacingMergeTree](https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/replacingmergetree) | 幂等依赖同一逻辑产物的类别、时间、ID 和归档键不变。背景合并承担存储整理，证据查询继续受时间范围约束。 |
| **IETF / IANA** | HTTP 标准将状态码定义为三位整数并按数值类别解释，IANA 维护已注册值。[RFC 9110 状态码](https://www.rfc-editor.org/rfc/rfc9110.html#name-status-codes) · [IANA HTTP 状态码](https://www.iana.org/assignments/http-status-codes/http-status-codes.xhtml) | HTTP 状态码选择建议由注册表支持，规则按有效协议范围校验。它和 OCSF `activity_id` 的方法枚举属于不同字段。 |

## 调研边界

GitHub 已读取 AiSOC 干净安装记录、Wazuh Compose、Malcolm 安装脚本及 Elasticsearch 管道源码。Exa 用于发现官方文档，Context7 用于核对 ClickHouse 去重与查询语义。中文厂商依据来自阿里云、腾讯云、华为云及深信服公开手册。厂商公开的操作界面可作为参照，交付结论须由 SecPlexus 的隔离验收产生。未找到可核对的 Hexagon 中文 SIEM 解析字段资料，因此本规格中的中文术语依据上述厂商手册和项目领域模型。
