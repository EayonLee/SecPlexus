# 接入与解析规则走查修复计划

1. 在现有 CSV reader 后检查消息边界，提取试算和事件发布使用相同的诊断原因。解析单测覆盖额外记录、尾部空行和引号内换行。浏览器验证错误定位和修正后继续。 [Go CSV Reader](https://pkg.go.dev/encoding/csv#Reader.Read)
2. 保存和恢复先同步详情缓存，列表与历史继续失效重取。规则和接入详情沿用 Web 模板的分组详情样式，使用现有章节、字段和折叠帮助。浏览器验证保存后无需刷新、验证摘要以及 Syslog 监听说明。 [TanStack Query 变更响应](https://tanstack.com/query/latest/docs/framework/react/guides/updates-from-mutation-responses)
3. 对齐模块 README、部署文档和实现。验证宿主机通过 Compose 映射端口实际发送 Syslog TCP 与 UDP。回读事件库验证五种格式、四条接入路径和可选事件类型。 [Compose 端口配置](https://docs.docker.com/reference/compose-file/services/#ports)

## 验证结果

- Go 单测、真实依赖集成测试、Compose 配置检查、135 项 Web 单测和 12 项 Ingest 浏览器用例通过。API 创建的无条件分支可在浏览器查看和修改描述，保留原验证时间。
- 隔离 Compose 中，宿主机 Syslog TCP 与 UDP 均可接入，HTTP 一次提交两行返回已接收两条。单条 Syslog 报文和单条 Kafka 消息包含多条分隔符记录时落库为 `multiple_records` 错误事件，Syslog 引号内换行保留。
- 五种解析格式在四条接入路径上的 20 个正常组合均通过写入与回查。11 个可选事件类型的正常和缺字段样本均符合预期。

[交付规格](spec.md)
