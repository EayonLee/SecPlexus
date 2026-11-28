# Event 存储

Ingest 将标准 Event 发布到平台 Kafka。独立的 `store-events` 进程批量写入 ClickHouse，确认写成或写入失败主题后，再提交各分区的连续消费位移。Kafka 默认保留 7 天，可通过 `SPX_KAFKA_RETENTION_DAYS` 修改。[启动与配置](../../../src/apps/ingest/README.md#clickhouse-事件存储)

## 表、字段与查询

ClickHouse 保存完整 `event_json` 和供检索使用的类型列。事件 ID 为平台生成的 UUID，对应 OCSF `metadata.uid`。原 Kafka 记录重读时沿用同一事件 ID、类别和发生时间。重新发送一条设备报文仍可能产生新事件。

- 库和表默认为 `spx.events`。引擎为 `ReplacingMergeTree`，排序键是 `(class_uid, event_time, event_id)`。逻辑计数和按 ID 回查在查询中使用 `FINAL`。
- `event_time` 取 OCSF `time`，存储为 UTC 毫秒。固定使用 `toYYYYMM(event_time)` 分区，历史补采进入事件发生月。已写入事件永久保留。
- 通过 `GET /api/v1/ingest/events/{event_id}` 回查。使用 RFC 3339 `from`、`to` 指定最长 31 天的半开时间范围。IAM 要求 `ingest.events.read`，ClickHouse 查询账号只授予事件表 `SELECT`。查询限制执行时间和线程数，日常列表或扫描场景应实测后再确定索引。

[ClickHouse 分区](https://clickhouse.com/docs/concepts/best-practices/partitioning-keys) · [ClickHouse 的 FINAL 查询](https://clickhouse.com/docs/reference/statements/select/from#final-modifier) · [HTTP 查询参数](https://clickhouse.com/docs/sql-reference/syntax#defining-and-using-query-parameters)

## 字段与数据库漂移控制

[事件字段目录](../../../src/apps/ingest/schema/events/catalog.json) 定义字段语义、类型、单位和枚举。 [物理投影](../../../src/apps/ingest/internal/repository/event/projection.json) 引用目录路径，定义 ClickHouse 列、物理类型和转换。首版迁移使用冻结的 `00001_columns.json` 和 SQL 模板，并校验两者的摘要。发布后调整物理结构使用新的前向迁移。

`check-event-store` 使用写入账号检查 `SELECT`、`INSERT` 和表结构。列、注释、约束、主键、排序键、分区及保留策略必须与基线一致。发现漂移或无法确认结构时停止写入和提交位移，修复后再用原组继续。

## 重试与恢复

| 现象 | 操作 |
| --- | --- |
| ClickHouse 暂时无法写入 | 保留未确认批次的消费位移。恢复 ClickHouse 后用原消费组重启，检查积压、最老未写记录和逻辑事件计数。 |
| 单条消息无法投影 | 将稳定原因、消息字节数和来源主题、分区、位移写入独立失败主题。隔离失败时位移保持未提交。 |
| 已提交位移早于 Kafka 可用位置 | 写入进程暂停受影响的分区，其余分区继续。先记录主题身份、消费组及位移范围，再核查已经落库的事件，按确认的缺口恢复。 |
| 同版本恢复 | 停止写入器并记录每个分区确认的连续位移。备份事件表、迁移账本和业务 PostgreSQL。恢复前核对主题身份和实际可消费范围。先撤销恢复库的旧会话，再验证历史事件、接续写入及逻辑计数。 |

[真实组件集成用例](../../../src/apps/ingest/internal/consumer/worker_integration_test.go) · [ClickHouse 备份与恢复](https://clickhouse.com/docs/concepts/features/backup-restore/overview) · [性能验收](performance.md)
