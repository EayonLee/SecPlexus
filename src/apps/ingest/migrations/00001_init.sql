-- +goose Up
-- 数据接入基线：解析映射、接入点、拉模式连接器。
--
-- 设计依据：
--   * Splunk 把"从哪收"(inputs.conf)与"怎么解析"(props.conf/transforms.conf)拆成两类配置，
--     并按接入类型分 stanza、各自带专属键 —— 协议专属设置是业界常态。
--   * Graylog 的 Input 用 `type` + `configuration: Map<String,Object>` 表达这些专属设置，
--     而不是给每个输入类型加一批固定列。
-- 因此这里用 `configuration jsonb` 承载协议专属设置，形状由 Go 侧按 protocol 校验；
-- 新增一种协议不改表结构。

CREATE SCHEMA IF NOT EXISTS ingest;

CREATE TABLE ingest.mappings (
    id uuid PRIMARY KEY,
    -- 展示名，全局唯一，便于在接入点里按名字引用。
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 100),
    name_normalized text GENERATED ALWAYS AS (lower(btrim(name))) STORED,
    -- normalizer.Mapping 的 JSON 形态；加载时用同一套三层校验编译，非法映射进不了注册表。
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    -- 每次改动自增；事件带 metadata.spx.mapping_version，解析质量可按映射版本归因。
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX mappings_name_normalized_key ON ingest.mappings (name_normalized);

CREATE TABLE ingest.inputs (
    id uuid PRIMARY KEY,
    -- 展示名，同时作为 OCSF metadata.log_name。
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 100),
    name_normalized text GENERATED ALWAYS AS (lower(btrim(name))) STORED,
    -- 接入协议；创建后不可修改，换协议意味着换一条接入点。
    protocol text NOT NULL CHECK (protocol IN ('http','syslog','kafka')),
    -- 空表示通用接入：原文透传、class_uid=6003，映射可后续补上。
    mapping_id uuid REFERENCES ingest.mappings (id) ON DELETE RESTRICT,
    -- 协议专属设置（http: token_sha256；syslog: port；kafka: brokers/topic）。
    -- 不建协议专属列：加协议就要改表，而这是 Graylog/Splunk 都没有的约束。
    configuration jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(configuration) = 'object'),
    -- 停用即不再监听/消费，配置保留，重新启用无需重启进程。
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX inputs_name_normalized_key ON ingest.inputs (name_normalized);
-- 管理面按协议与启用状态列表过滤。
CREATE INDEX inputs_protocol_enabled_idx ON ingest.inputs (protocol, enabled);
-- 删除映射前要查引用；外键会拒绝，但查引用要在报错前给出可读提示。
CREATE INDEX inputs_mapping_idx ON ingest.inputs (mapping_id) WHERE mapping_id IS NOT NULL;
-- 一个端口只能绑一个 syslog 接入点；用表达式索引而不是专用列。
CREATE UNIQUE INDEX inputs_syslog_port_key ON ingest.inputs ((configuration->>'port'))
    WHERE protocol = 'syslog' AND configuration ? 'port';

-- +goose Down
DROP TABLE IF EXISTS ingest.inputs;
DROP TABLE IF EXISTS ingest.mappings;
DROP SCHEMA IF EXISTS ingest;
