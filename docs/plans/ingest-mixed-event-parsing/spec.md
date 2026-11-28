# 混合日志解析与数据接入

旧契约和多行合并的后续处理见 [解析规则清理](../ingest-rule-cleanup/spec.md)。

同一接入点绑定一份有序解析规则。每条日志解码一次，按结构化字段匹配分支并映射到版本化 OCSF 契约。网络流量和管理员认证可由同一规则的不同分支识别。认证失败是有效业务事件。已受理而解析不完整或无法分类的 v2 日志仍发布 Event，标记解析质量和原因。Base Event 承载无法满足具体类别契约的记录。异常回退只保留经过验证和允许存储的字段。

Web 保留数据接入与解析规则两个菜单。接入向导按来源、连接、样本、确认排列。规则工作区允许维护分支顺序、匹配条件、输出类别和字段转换，在同页对照样本和实际输出。使用项目的 Dashboard 模板及共享标题、列表、表单和状态组件。

参考 [Elastic FortiGate pipeline](https://github.com/elastic/integrations/blob/main/packages/fortinet_fortigate/data_stream/log/elasticsearch/ingest_pipeline/default.yml#L369-L389)、[阿里云日志标准化](https://help.aliyun.com/zh/security-center/user-guide/standardized-log-access-rules) 和 [OCSF Base Event](https://github.com/ocsf/ocsf-schema/blob/2d209039d9c476ab76e83fb58b6f99374d40a994/events/base_event.json)。

## 规则与事件契约

| 边界 | 约定 |
|---|---|
| 接入点 | 引用一份 `mapping_id`。来源厂商与产品单独保存。未绑定规则的已受理记录生成基础事件。 |
| 规则定义 | `definition_version=2` 指明格式。规则内声明编码、分帧和有序分支。分支条件读源字段，使用等于或属于，所有条件都必须满足。空条件处理此前未命中的记录。编译拒绝重复和可证明被前序分支完全覆盖的条件，最多分析 4,096 种组合状态。 |
| 规则修订 | 创建、编辑、删除沿用规则管理路由。修改 v2 规则须提交 `expected_version`，修订冲突返回 409，与写入及 IAM 审计同一事务。 |
| 事件契约 | 平台契约 `1.1.0` 锁定 OCSF `1.10.0`。基础事件、认证活动、网络活动分别使用 `0`、`3002`、`4001`。归一化正文字段位于 JSON 根级。 |
| 解析质量 | `ok` 表示通过事件类校验。部分解析使用 `partial`，能满足所属类约束时保留原类，否则用 Base Event，并注明候选类。未匹配与解码失败用 Base Event 和 `error`。 |
| 元数据 | `metadata.spx` 保存规则修订、命中分支、时间来源和稳定诊断代码。通过转换的值保留，无效值省略。异常记录不携带原始报文。 |
| 字段目录 | HTTP 按契约版本和事件类提供字段路径、双语名称、含义、逻辑类型、基数、必填条件、枚举、归属。时间和严重度通过专属转换配置。 |
| ClickHouse | 同一标准主题存放三种质量的事件。查询列 `parse_outcome` 采用低基数字符串及受控值域，新旧契约共用 `spx.events`。旧事件读为 `ok`。完整正文存入 `event_json`。 |

规则热路径使用不可变已编译快照，每条记录一次解码。PostgreSQL 只在管理请求与刷新时访问。事件发布分区和已确认前缀继续沿用既有保证。对弥补失败而修改事件类别或发生时间的重放，先在隔离目标核对归档键和计数。

## 混合解析的样本场景

混合解析可验证连接关闭、流量放行、流量拒绝、常规流量记录、管理员登录成功和失败。管理员认证使用 FortiOS 日志编号及 CEF 类别、动作。使用脱敏合成样本验证受支持字段。缺少账号和未识别记录作为反例。 [FortiOS 成功编号](https://docs.fortinet.com/document/fortigate/7.6.0/fortios-log-message-reference/32001/32001-log-id-admin-login-succ) · [FortiOS 失败编号](https://docs.fortinet.com/document/fortigate/7.0.18/fortios-log-message-reference/32002/32002-log-id-admin-login-fail) · [CEF 流量样本](https://docs.fortinet.com/document/fortigate/6.4.6/fortios-log-message-reference/949981/traffic-log-support-for-cef)
