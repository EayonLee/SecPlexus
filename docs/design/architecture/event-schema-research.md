# 平台事件字段契约调研

SecPlexus 应发布一份基于指定 OCSF 版本的事件契约。平台维护业务可以依赖的字段路径、逻辑类型、枚举值和适用类别，接入规则负责把厂商数据转换到这份契约。接入内部可以用动态对象处理原始报文，向下游发布标准事件时需要验证实际输出。以下区分源码观察与设计建议。

## 三条从接入到业务的证据

**AiSOC 把事件类别和严重度用于告警决策。** 在核查的版本中，TypeScript 定义基础事件、网络活动子类型，以及类别、严重度和状态的枚举。Go 接入代码按连接器选择映射，在 map 中构造事件。内置连接器和模板的类别编号有测试对照一个已声明的类别集合。Fusion 的实际晋升条件是整数类别属于 Findings，或整数严重度至少为 4。若严重度是字符串 `"4"`，严重度分支不会命中。还有一个值得避免的边界：类型包列出 `99 = Other`，如果该整数进入此函数，也会满足数值阈值。这里描述函数语义，不代表线上观察到了这种输入。[基础类型与枚举](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/packages/ocsf/src/types.ts#L258) · [Go 归一化](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/services/ingest/internal/normalizer/normalizer.go#L783) · [模板与类别契约测试](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/services/ingest/internal/normalizer/ocsf_classes_test.go#L318) · [晋升逻辑](https://github.com/beenuar/AiSOC/blob/b079731462d968fa8b3ebe5e9dedaac51c4156d2/services/fusion/app/services/promoter.py#L196)

**Microsoft Sentinel 让同一条检测规则跨数据源工作。** VMConnection 解析器将 `SourceIp` 和 `DestinationIp` 投影为 ASIM 的 `SrcIpAddr` 和 `DstIpAddr`。微软的一条威胁情报规则查询统一的网络会话视图，再用这两个字段匹配已知恶意 IP。ASIM 规定 IP 的含义、逻辑类型和条件必填项。这个例子通过查询视图提供标准字段，允许不同产品保留各自的原始表结构。[VMConnection 解析器](https://github.com/Azure/Azure-Sentinel/blob/master/Parsers/ASimNetworkSession/Parsers/vimNetworkSessionVMConnection.yaml) · [情报关联规则](https://github.com/Azure/Azure-Sentinel/blob/master/Solutions/Threat%20Intelligence/Analytic%20Rules/IPEntity_imNetworkSession.yaml) · [网络会话字段规范](https://learn.microsoft.com/en-us/azure/sentinel/normalization-schema-network)

**Google SecOps 按事件类型校验 UDM 输出。** 官方文档展示了解析成功、输出因缺少该类必需字段而被判无效的情况。官方仓库收录的跨地域登录检测使用统一的登录事件类别、动作和用户标识。这里核查了公开检测代码和官方校验行为说明，服务端校验器内部实现未公开核实。[登录检测规则](https://github.com/chronicle/detection-rules/blob/dbc9beb79597b519be086fce082214494d75b6a8/rules/community/authentication/geoip_user_login_from_multiple_states_or_countries.yaral) · [解析输出校验](https://docs.cloud.google.com/chronicle/docs/event-processing/parser-tips-troubleshooting)

## 对照 SecPlexus 当前实现

| 核查项 | 当前行为及影响 |
|---|---|
| 字段路径与类型 | 规则校验路径结构和写入冲突，普通字段投影将源值直接写入目标路径。Kafka 分区已经依赖字符串路径 `src_endpoint.ip`。名称拼错时，分区逻辑会继续尝试其他实体键，即使样本预览中有输出，也可能失去按源 IP 分区的行为。[规则编译](../../../src/apps/ingest/internal/normalizer/rule.go) · [字段投影](../../../src/apps/ingest/internal/normalizer/apply.go) · [分区键选择](../../../src/apps/ingest/internal/pipeline/pipeline.go) |
| 类别语义 | Ingest 声明 OCSF 1.1.0，并用 `6003` 作为未解析事件的兜底。该版本中应用活动类别编号为 6，API Activity 在类内编号为 3，要求提供 actor 和 src_endpoint 等属性。把任意报文标为 API Activity 会影响下游按类别统计和路由。[本地兜底逻辑](../../../src/apps/ingest/internal/pipeline/pipeline.go) · [OCSF 类别](https://github.com/ocsf/ocsf-schema/blob/v1.1.0/categories.json) · [API Activity](https://github.com/ocsf/ocsf-schema/blob/v1.1.0/events/application/api.json) |
| 枚举值域 | 本地严重度映射表接受 0–6，OCSF 1.1.0 字典另外定义了 99 表示 Other。平台可以声明支持的子集，但必须让接入端和业务端对特殊值有一致解释。活动编号还要按事件类别解释，例如 API Activity 的 1 表示 Create。[本地严重度校验](../../../src/apps/ingest/internal/normalizer/rule.go) · [OCSF 字典](https://github.com/ocsf/ocsf-schema/blob/v1.1.0/dictionary.json) · [类别专属活动值](https://github.com/ocsf/ocsf-schema/blob/v1.1.0/events/application/api.json) |

## 建议的契约边界

1. **平台发布版本化字段目录。** 明确上游 OCSF 版本、平台契约版本和支持的事件类别。每个目标字段定义路径、业务含义、逻辑类型、单值或数组、必填条件。枚举保留标准编号和含义，标明作用域以及 Unknown、Other 等特殊值的处理。平台扩展集中在受控命名空间。
2. **用户配置来源与转换。** 以网络活动为例，两个设备可以分别将 `src` 和 `client_ip` 映射到同一个源 IP 字段。规则编辑器从该类的目录选择目标字段，样本预览同时展示原始值、转换后的值和校验结果。用户可以维护厂商枚举到标准枚举的对应关系。
3. **后端对实际发布结果负责。** 保存或导入时检查目标路径、常量、枚举映射和类别。运行时检查每条事件的实际类型、值域和必填对象。已识别类别中的额外厂商字段可保留在 `unmapped`；整条记录无法分类或违反必填约束时，应保留可诊断的原始记录，并与通过校验的标准事件区分。
4. **检测、关联、检索共享同一份定义。** 编辑器、Go 校验和下游类型使用同一份机器可读契约。事件类别决定可用字段，存储引擎可以单独选择列、索引或 JSON 表示。升级时用“不同来源、相同业务含义”的契约用例验证同一条查询或检测仍能命中。

第一条有区分度的验收用例是让两种厂商样本经不同规则产生同一个有效源 IP，并由同一条情报匹配规则命中。再用拼错的目标路径、非法 IP、越界枚举和未分类报文分别验证错误发生在哪一层。这样可以同时验证用户能接入新来源，以及下游能安全依赖平台字段。

## 标准字段如何约束存储

| 一手资料 | 可核实的做法 | 可用于 SecPlexus 的部分 |
|---|---|---|
| [Elastic ECS 生成工具](https://github.com/elastic/ecs/blob/main/USAGE.md) | 同一份字段定义可以选取子集，生成 Elasticsearch 模板、Beats 配置、CSV 和文档。 | 用已发布的契约派生机器可读产物，允许不同消费者有不同的物理表示。 |
| [Atlas 版本化迁移漂移检查](https://atlasgo.io/versioned/drift-detection) | 以目标库**已应用的版本**计算预期状态，比较实际结构，可在执行下一次迁移前报告并阻断差异。 | 合并检查验证仓库，发布和巡检验证环境；按环境的当前版本比对，适应滚动升级。采用这项做法无需引入 Atlas 服务。 |
| [ClickHouse 系统列目录](https://clickhouse.com/docs/reference/system-tables/columns) | 可读列名、具体类型、默认表达式、注释、键标记等数据库实况。 | 对模块拥有的表做只读检查，配合建表定义核实引擎、完整键表达式和 TTL。行数据与正文的一致性通过写入回读验证。 |

这些例子分别说明标准字段派生和数据库实况核验。SecPlexus 的 OCSF 子集、中文目录、ClickHouse 投影、角色及发布顺序由项目决定。具体设计见[字段与数据库漂移控制](../../plans/event-contract/storage.md#字段与数据库漂移控制)。

## 归一化结果的命名依据

OCSF 1.10 定义事件类、对象、属性及枚举。Base Event 定义属性对该类的必填性，字段字典提供类型与枚举含义。核查的定义没有规定规则引擎必须返回一个名为 `MappingResult` 的 Go 类型。Microsoft Sentinel ASIM 的开发文档把来源解析器的输出称为归一化字段，并分别验证输出结构和字段取值；这为处理阶段的语义提供了参照。[OCSF Base Event](https://github.com/ocsf/ocsf-schema/blob/1.10/events/base_event.json) · [OCSF 字典](https://github.com/ocsf/ocsf-schema/blob/1.10/dictionary.json) · [ASIM 解析器开发指南](https://learn.microsoft.com/en-us/azure/sentinel/normalization-develop-parsers)

本项目的 `ApplyFields` 做字段映射、严重度和时间折算、未映射字段收集与警告记录。归一化比映射更完整地描述其职责。拟采用 Go 限定名 `normalizer.Result`，中文称“归一化结果”。这是平台实现决策。现有 `pipeline.Outcome` 描述处理状态或失败原因，最终领域 Event 表示已校验的安全观测。不同阶段分别命名，有助于避免把含警告的中间数据误用为可发布事件。[ApplyFields](../../../src/apps/ingest/internal/normalizer/apply.go) · [Pipeline](../../../src/apps/ingest/internal/pipeline/pipeline.go) · [Go 包与导出标识符命名](https://go.dev/blog/package-names)
