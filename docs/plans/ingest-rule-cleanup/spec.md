# 清理旧版解析规则与多行合并

只接受当前的有序规则定义和 Event 契约 1.1.0。Web 统一使用现行编辑器，删除旧规则导入与多行合并配置。单类数据仍可用一条无条件分支。规则修订号继续用于并发校验与事件追溯。

规则不决定多行合并。HTTP 的批量格式、Kafka 消息边界和 TCP Syslog 的长度分帧由协议接收端处理。多行日志由上游采集器组装为一条消息。参见 [RFC 6587](https://www.rfc-editor.org/rfc/rfc6587.html#section-3.4) 和 [Filebeat 多行采集](https://www.elastic.co/guide/en/beats/filebeat/8.19/multiline-examples.html)。

PostgreSQL 前向迁移删除旧定义及旧格式修订，暂停仍绑定旧定义的接入点并解除引用。现行规则若配置了多行合并，则移除配置、记录新修订并将受影响接入点停用，待确认设备发送格式再启用。保留现行模板及其它规则、接入点和审计记录。迁移不直接修改 Kafka 和 ClickHouse 存量数据。归档消费者只接受现行契约，仍用归档失败通道记录无法投影的消息。
