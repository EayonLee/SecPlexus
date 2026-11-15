-- +goose Up
-- 数据接入基线：接入点（sources）与解析规则（parse_rules）。
--
-- 为什么分两张表：Splunk 按数据管道阶段拆配置（inputs.conf 管输入、props.conf/transforms.conf 管解析），
-- Vector 的 sources 与 transforms 也是两个配置段并用 inputs 引用 source。
-- 因此"从哪收"与"怎么解析"是两类实体：换规则不改接入，一条规则可被多个接入点复用。
--
-- 命名：接入点叫 source；connector 一词保留给后续的厂商拉取适配器，两者不得混用。

CREATE SCHEMA IF NOT EXISTS ingest;

CREATE TABLE ingest.parse_rules (
    id uuid PRIMARY KEY,
    -- 展示名，全局唯一，便于在接入点里按名字引用。
    name text NOT NULL UNIQUE CHECK (char_length(name) BETWEEN 1 AND 100),
    -- normalizer.Rule 的 JSON 形态；加载时用同一套三层校验编译，非法规则进不了注册表。
    spec jsonb NOT NULL CHECK (jsonb_typeof(spec) = 'object'),
    -- 每次改动自增；事件带 metadata.spx.rule_version，解析质量可按规则版本归因。
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ingest.sources (
    id uuid PRIMARY KEY,
    -- 展示名，同时作为 OCSF metadata.log_name。
    name text NOT NULL UNIQUE CHECK (char_length(name) BETWEEN 1 AND 100),
    -- 传输方式；创建后不可变，改协议意味着换一条接入点。
    transport text NOT NULL CHECK (transport IN ('http','syslog','kafka')),
    -- 空表示通用接入：原文透传、class_uid=6003，规则可后续补上。
    parse_rule_id uuid REFERENCES ingest.parse_rules (id) ON DELETE RESTRICT,
    -- HTTP 令牌的 SHA-256 摘要；明文只在创建响应里出现一次，之后无从取回。
    token_digest bytea CHECK (token_digest IS NULL OR octet_length(token_digest) = 32),
    -- syslog 端口：UDP 与 TCP 同端口；<1024 需要特权，因此下限 1024。
    syslog_port integer CHECK (syslog_port IS NULL OR syslog_port BETWEEN 1024 AND 65535),
    kafka_brokers text[] CHECK (kafka_brokers IS NULL OR cardinality(kafka_brokers) > 0),
    kafka_topic text CHECK (kafka_topic IS NULL OR char_length(kafka_topic) BETWEEN 1 AND 249),
    -- 停用即不再监听/消费，配置保留，重新启用无需重启进程。
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- 每种传输方式各自必需的字段：缺了就不允许入库，而不是留给应用层判断。
    CONSTRAINT sources_transport_fields CHECK (
        (transport = 'http' AND token_digest IS NOT NULL AND syslog_port IS NULL AND kafka_topic IS NULL)
        OR (transport = 'syslog' AND syslog_port IS NOT NULL AND token_digest IS NULL AND kafka_topic IS NULL)
        OR (transport = 'kafka' AND kafka_topic IS NOT NULL AND kafka_brokers IS NOT NULL
            AND token_digest IS NULL AND syslog_port IS NULL)
    )
);

-- 一个端口只能绑一个接入点；多个接入点抢同一端口会让监听器相互覆盖。
CREATE UNIQUE INDEX sources_syslog_port_key ON ingest.sources (syslog_port) WHERE syslog_port IS NOT NULL;

-- 管理面按传输方式与启用状态列表过滤，这条索引覆盖最常见的两种筛选。
CREATE INDEX sources_transport_enabled_idx ON ingest.sources (transport, enabled);

-- 删除解析规则前要查引用；外键本身会拒绝，但查引用要在报错前给出可读提示。
CREATE INDEX sources_parse_rule_idx ON ingest.sources (parse_rule_id) WHERE parse_rule_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS ingest.sources;
DROP TABLE IF EXISTS ingest.parse_rules;
