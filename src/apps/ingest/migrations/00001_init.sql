-- +goose Up
-- 数据接入基线：解析映射与接入点。
--
-- 协议专属设置放在 configuration jsonb 里，形状由 Go 侧按 protocol 校验：
-- 每加一种协议都要改表结构的设计会把这个模块锁死在三种协议上。

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
    -- 不建协议专属列：加一种协议就要改表，属于可避免的耦合。
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

-- 表与业务列注释：数据库是运维与排障的第一现场，
-- 没有注释的列只能靠读代码猜含义，改配置时尤其危险。
COMMENT ON TABLE ingest.mappings IS '解析映射：把原始报文翻译成 OCSF 字段的规则';
COMMENT ON COLUMN ingest.mappings.id IS '映射标识';
COMMENT ON COLUMN ingest.mappings.name IS '展示名；全局唯一，接入点按名字引用';
COMMENT ON COLUMN ingest.mappings.name_normalized IS '小写去空格的名称，用于唯一索引';
COMMENT ON COLUMN ingest.mappings.definition IS '映射定义 JSON；加载时按结构与样本试跑校验';
COMMENT ON COLUMN ingest.mappings.version IS '版本号，每次改动自增；事件按版本归因解析质量';
COMMENT ON COLUMN ingest.mappings.created_at IS '创建时间（UTC）';
COMMENT ON COLUMN ingest.mappings.updated_at IS '最后修改时间（UTC）';

COMMENT ON TABLE ingest.inputs IS '接入点：一个协议加一份配置，决定从哪收数据';
COMMENT ON COLUMN ingest.inputs.id IS '接入点标识；事件写入 metadata.spx.input_id';
COMMENT ON COLUMN ingest.inputs.name IS '展示名；同时作为 OCSF metadata.log_name';
COMMENT ON COLUMN ingest.inputs.name_normalized IS '小写去空格的名称，用于唯一索引';
COMMENT ON COLUMN ingest.inputs.protocol IS '接入协议：http / syslog / kafka；创建后不可修改';
COMMENT ON COLUMN ingest.inputs.mapping_id IS '解析映射；为空表示通用接入，原文透传且 class_uid=6003';
COMMENT ON COLUMN ingest.inputs.configuration IS '协议专属设置（http: token_sha256；syslog: port；kafka: brokers/topic）';
COMMENT ON COLUMN ingest.inputs.enabled IS '是否启用；停用即停止监听或消费，配置保留';
COMMENT ON COLUMN ingest.inputs.created_at IS '创建时间（UTC）';
COMMENT ON COLUMN ingest.inputs.updated_at IS '最后修改时间（UTC）';

-- +goose Down
DROP TABLE IF EXISTS ingest.inputs;
DROP TABLE IF EXISTS ingest.mappings;
DROP SCHEMA IF EXISTS ingest;
