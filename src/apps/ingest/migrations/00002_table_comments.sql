-- +goose Up
-- 表与业务列的中文说明：数据库是长期资产，含义必须留在库内，而不是只写在代码里。

COMMENT ON SCHEMA ingest IS '数据接入：接入点与解析规则。';
COMMENT ON TABLE ingest.sources IS '接入点：一条接入登记，描述设备从哪里进来、用哪条规则解析。';
COMMENT ON COLUMN ingest.sources.id IS '接入点标识；事件回填到 metadata.spx.source_id。';
COMMENT ON COLUMN ingest.sources.name IS '展示名，唯一；同时作为 OCSF metadata.log_name。';
COMMENT ON COLUMN ingest.sources.transport IS '传输方式：http / syslog / kafka；创建后不可修改。';
COMMENT ON COLUMN ingest.sources.parse_rule_id IS '绑定的解析规则；为空表示通用接入，原文透传且 class_uid=6003。';
COMMENT ON COLUMN ingest.sources.token_digest IS 'HTTP 令牌的 SHA-256 摘要；明文只在创建响应中出现一次。';
COMMENT ON COLUMN ingest.sources.syslog_port IS 'syslog 监听端口，UDP 与 TCP 同端口，全表唯一，取值范围 1024-65535。';
COMMENT ON COLUMN ingest.sources.kafka_brokers IS '待消费的外部 Kafka broker 列表。';
COMMENT ON COLUMN ingest.sources.kafka_topic IS '待消费的外部 Kafka topic。';
COMMENT ON COLUMN ingest.sources.enabled IS '是否启用；停用后不再监听或消费，配置保留。';
COMMENT ON COLUMN ingest.sources.created_at IS '创建时间（UTC）。';
COMMENT ON COLUMN ingest.sources.updated_at IS '最后修改时间（UTC）。';

COMMENT ON TABLE ingest.parse_rules IS '解析规则：线格式、字段映射、severity 与时间声明。';
COMMENT ON COLUMN ingest.parse_rules.id IS '解析规则标识。';
COMMENT ON COLUMN ingest.parse_rules.name IS '展示名，唯一。';
COMMENT ON COLUMN ingest.parse_rules.spec IS '规则定义的 JSON 形态；加载时按三层校验编译。';
COMMENT ON COLUMN ingest.parse_rules.version IS '规则版本，每次改动自增；事件按它归因解析质量。';
COMMENT ON COLUMN ingest.parse_rules.created_at IS '创建时间（UTC）。';
COMMENT ON COLUMN ingest.parse_rules.updated_at IS '最后修改时间（UTC）。';

-- +goose Down
COMMENT ON SCHEMA ingest IS NULL;
