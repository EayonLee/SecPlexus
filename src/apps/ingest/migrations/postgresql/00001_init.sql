-- +goose Up
-- SecPlexus 首版结构。从空库建立，不写入示例规则和接入。
CREATE SCHEMA IF NOT EXISTS ingest;

-- +goose StatementBegin
CREATE FUNCTION ingest.record_parsing_rule_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO ingest.parsing_rule_revisions (parsing_rule_id, version, name, vendor, product, definition, recorded_at, verification, author_id)
    VALUES (NEW.id, NEW.version, NEW.name, NEW.vendor, NEW.product, NEW.definition, NEW.updated_at, NEW.verification, NEW.author_id);
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE ingest.audit_outbox (
    id uuid NOT NULL,
    event jsonb NOT NULL,
    queued_at timestamptz DEFAULT now() NOT NULL,
    rejected_at timestamptz,
    CONSTRAINT audit_outbox_event_check CHECK (((jsonb_typeof(event) = 'object'::text) AND (COALESCE((event ->> 'request_id'::text), ''::text) <> ''::text)))
);

COMMENT ON TABLE ingest.audit_outbox IS '与管理写操作同事务提交、等待发送给 IAM 的审计事实';

COMMENT ON COLUMN ingest.audit_outbox.id IS '投递标识；在 IAM 审计中使用同一个标识去重';

COMMENT ON COLUMN ingest.audit_outbox.event IS '待投递的审计事实 JSON，不包含用户或设备凭据';

COMMENT ON COLUMN ingest.audit_outbox.queued_at IS '首次入队时间（UTC）';

COMMENT ON COLUMN ingest.audit_outbox.rejected_at IS 'IAM 因内容无效拒绝的时间（UTC），为空表示待投递';

CREATE TABLE ingest.inputs (
    id uuid NOT NULL,
    name text NOT NULL,
    name_normalized text GENERATED ALWAYS AS (lower(btrim(name))) STORED,
    protocol text NOT NULL,
    parsing_rule_id uuid,
    configuration jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    vendor text DEFAULT ''::text NOT NULL,
    product text DEFAULT ''::text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    credential_generation bigint DEFAULT 1 NOT NULL,
    CONSTRAINT inputs_configuration_check CHECK ((jsonb_typeof(configuration) = 'object'::text)),
    CONSTRAINT inputs_credential_generation_check CHECK ((credential_generation > 0)),
    CONSTRAINT inputs_kafka_credential_check CHECK (NOT (configuration ? 'credential') OR (protocol = 'kafka' AND COALESCE((configuration->>'credential') ~ '^[a-z][a-z0-9_-]{0,63}$', false))),
    CONSTRAINT inputs_kafka_start_position_check CHECK (((protocol <> 'kafka'::text) OR ((configuration ? 'start_position'::text) AND COALESCE(((configuration ->> 'start_position'::text) = ANY (ARRAY['earliest'::text, 'since_created'::text])), false)))),
    CONSTRAINT inputs_name_check CHECK (((char_length(btrim(name)) >= 1) AND (char_length(btrim(name)) <= 100))),
    CONSTRAINT inputs_protocol_check CHECK ((protocol = ANY (ARRAY['http'::text, 'syslog'::text, 'kafka'::text]))),
    CONSTRAINT inputs_revision_check CHECK ((revision > 0))
);

COMMENT ON TABLE ingest.inputs IS '数据接入：一个协议加一份配置，决定从哪收数据';

COMMENT ON COLUMN ingest.inputs.id IS '数据接入 ID；事件写入 metadata.spx.input_id';

COMMENT ON COLUMN ingest.inputs.name IS '展示名；同时作为 OCSF metadata.log_name';

COMMENT ON COLUMN ingest.inputs.name_normalized IS '小写去空格的名称，用于唯一索引';

COMMENT ON COLUMN ingest.inputs.protocol IS '接入协议：http / syslog / kafka；创建后不可修改';

COMMENT ON COLUMN ingest.inputs.parsing_rule_id IS '数据接入绑定的解析规则 ID，用于按解析规则统计数据接入数量。NULL 表示未绑定规则';

COMMENT ON COLUMN ingest.inputs.configuration IS '协议专属设置（http: token_sha256；syslog: port；kafka: brokers/topic、消费起点及部署凭据名）';

COMMENT ON COLUMN ingest.inputs.enabled IS '是否启用；停用即停止监听或消费，配置保留';

COMMENT ON COLUMN ingest.inputs.created_at IS '创建时间（UTC）';

COMMENT ON COLUMN ingest.inputs.updated_at IS '最后修改时间（UTC）';

COMMENT ON COLUMN ingest.inputs.vendor IS '实际采集来源的厂商';

COMMENT ON COLUMN ingest.inputs.product IS '实际采集来源的产品';

COMMENT ON COLUMN ingest.inputs.revision IS '数据接入配置修订；管理变更提交时递增';

COMMENT ON COLUMN ingest.inputs.credential_generation IS '设备令牌代次；仅令牌轮换时递增';

COMMENT ON CONSTRAINT inputs_kafka_start_position_check ON ingest.inputs IS 'Kafka 无已提交位移时选择最早可用位置或按接入创建时间读取；恢复旧分区沿用已提交位移';

CREATE TABLE ingest.kafka_execution_boundaries (
    input_id uuid NOT NULL,
    partition integer NOT NULL,
    first_offset bigint NOT NULL,
    source_revision bigint NOT NULL,
    source_name text NOT NULL,
    vendor text NOT NULL,
    product text NOT NULL,
    parsing_rule_id uuid,
    parsing_rule_version integer NOT NULL,
    CONSTRAINT kafka_execution_boundaries_check CHECK (((parsing_rule_id IS NULL) = (parsing_rule_version = 0))),
    CONSTRAINT kafka_execution_boundaries_first_offset_check CHECK ((first_offset >= 0)),
    CONSTRAINT kafka_execution_boundaries_parsing_rule_version_check CHECK ((parsing_rule_version >= 0)),
    CONSTRAINT kafka_execution_boundaries_partition_check CHECK ((partition >= 0)),
    CONSTRAINT kafka_execution_boundaries_source_revision_check CHECK ((source_revision > 0))
);

COMMENT ON TABLE ingest.kafka_execution_boundaries IS 'Kafka 来源在已确认位移边界采用的固定配置及解析规则；用于自动重试和历史追溯';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.input_id IS 'Kafka 数据接入 ID';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.partition IS 'Kafka 分区编号，非负';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.first_offset IS '采用配置的第一个位移，包含该位移';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.source_revision IS '采用的数据接入配置修订，正整数';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.source_name IS '采用时的数据接入名称';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.vendor IS '采用的数据源厂商';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.product IS '采用的数据源产品';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.parsing_rule_id IS '解析规则 ID；无规则时为空';

COMMENT ON COLUMN ingest.kafka_execution_boundaries.parsing_rule_version IS '固定使用的解析规则修订；无规则时为 0';

CREATE TABLE ingest.kafka_partition_heads (
    input_id uuid NOT NULL,
    partition integer NOT NULL,
    topic_id uuid NOT NULL,
    first_offset bigint NOT NULL,
    prepared_through bigint NOT NULL,
    CONSTRAINT kafka_partition_heads_check CHECK ((prepared_through >= first_offset)),
    CONSTRAINT kafka_partition_heads_first_offset_check CHECK ((first_offset >= 0)),
    CONSTRAINT kafka_partition_heads_partition_check CHECK ((partition >= 0))
);

COMMENT ON TABLE ingest.kafka_partition_heads IS 'Kafka 来源分区的准备水位；拒绝主题更换、丢失位移和无提交证明的规则切换';

COMMENT ON COLUMN ingest.kafka_partition_heads.input_id IS '提供数据的数据接入 ID；删除接入不改变已存储事件的来源关联';

COMMENT ON COLUMN ingest.kafka_partition_heads.partition IS 'Kafka 分区编号，非负';

COMMENT ON COLUMN ingest.kafka_partition_heads.topic_id IS 'Kafka 主题的 broker UUID，区分同名重建';

COMMENT ON COLUMN ingest.kafka_partition_heads.first_offset IS '首次确认用于本源的起始位移';

COMMENT ON COLUMN ingest.kafka_partition_heads.prepared_through IS '已锁定归一化配置的最高记录位移，包含该位移';

CREATE TABLE ingest.parsing_rule_revisions (
    parsing_rule_id uuid NOT NULL,
    version integer NOT NULL,
    name text NOT NULL,
    vendor text NOT NULL,
    product text NOT NULL,
    definition jsonb NOT NULL,
    recorded_at timestamptz NOT NULL,
    verification jsonb,
    author_id uuid,
    CONSTRAINT parsing_rule_revision_excludes_samples CHECK ((NOT (definition ? 'samples'::text))),
    CONSTRAINT parsing_rule_revisions_definition_check CHECK ((jsonb_typeof(definition) = 'object'::text)),
    CONSTRAINT parsing_rule_revisions_definition_current CHECK (((definition @> '{"contract_version": "v1"}'::jsonb) AND (NOT (definition ?| ARRAY['multiline'::text, 'definition_version'::text])))),
    CONSTRAINT parsing_rule_revisions_verification_check CHECK ((jsonb_typeof(verification) = 'object'::text)),
    CONSTRAINT parsing_rule_revisions_version_check CHECK ((version > 0))
);

COMMENT ON TABLE ingest.parsing_rule_revisions IS '解析规则在采用时的快照；规则删除后仍可按事件中的规则 ID 和版本查回';

COMMENT ON COLUMN ingest.parsing_rule_revisions.parsing_rule_id IS '规则 ID；历史记录随 Event 保留，不依赖当前规则仍存在';

COMMENT ON COLUMN ingest.parsing_rule_revisions.version IS '规则修订号，正整数';

COMMENT ON COLUMN ingest.parsing_rule_revisions.name IS '采用此修订时的规则名称';

COMMENT ON COLUMN ingest.parsing_rule_revisions.vendor IS '采用此修订时适配的厂商，空字符串表示未设置';

COMMENT ON COLUMN ingest.parsing_rule_revisions.product IS '采用此修订时适配的产品，空字符串表示未设置';

COMMENT ON COLUMN ingest.parsing_rule_revisions.definition IS '当时编译使用的规则 JSON 对象';

COMMENT ON COLUMN ingest.parsing_rule_revisions.recorded_at IS '该修订记录的 UTC 时间';

COMMENT ON COLUMN ingest.parsing_rule_revisions.verification IS '规则发布时采用的验证摘要，不保存样本文本或提取值';

COMMENT ON COLUMN ingest.parsing_rule_revisions.author_id IS '采用此修订的 IAM 用户 ID；跨模块引用不建外键';

CREATE TABLE ingest.parsing_rules (
    id uuid NOT NULL,
    name text NOT NULL,
    name_normalized text GENERATED ALWAYS AS (lower(btrim(name))) STORED,
    definition jsonb NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    builtin boolean DEFAULT false NOT NULL,
    vendor text DEFAULT ''::text NOT NULL,
    product text DEFAULT ''::text NOT NULL,
    verification jsonb,
    author_id uuid,
    CONSTRAINT parsing_rule_definition_excludes_samples CHECK ((NOT (definition ? 'samples'::text))),
    CONSTRAINT parsing_rules_definition_check CHECK ((jsonb_typeof(definition) = 'object'::text)),
    CONSTRAINT parsing_rules_definition_current CHECK (((definition @> '{"contract_version": "v1"}'::jsonb) AND (NOT (definition ?| ARRAY['multiline'::text, 'definition_version'::text])))),
    CONSTRAINT parsing_rules_description_text CHECK (((NOT (definition ? 'description'::text)) OR ((jsonb_typeof((definition -> 'description'::text)) = 'string'::text) AND (char_length((definition ->> 'description'::text)) <= 1000)))),
    CONSTRAINT parsing_rules_name_check CHECK (((char_length(btrim(name)) >= 1) AND (char_length(btrim(name)) <= 100))),
    CONSTRAINT parsing_rules_product_length CHECK ((char_length(product) <= 120)),
    CONSTRAINT parsing_rules_vendor_length CHECK ((char_length(vendor) <= 120)),
    CONSTRAINT parsing_rules_verification_check CHECK ((jsonb_typeof(verification) = 'object'::text)),
    CONSTRAINT parsing_rules_version_check CHECK ((version > 0))
);

COMMENT ON TABLE ingest.parsing_rules IS '解析规则：从日志提取源字段并映射至 OCSF 标准字段';

COMMENT ON COLUMN ingest.parsing_rules.id IS '映射标识';

COMMENT ON COLUMN ingest.parsing_rules.name IS '解析规则名称；全局唯一，数据接入按规则 ID 绑定';

COMMENT ON COLUMN ingest.parsing_rules.name_normalized IS '小写去空格的名称，用于唯一索引';

COMMENT ON COLUMN ingest.parsing_rules.definition IS '解析规则定义 JSON；branches[].class_uid 表示配置的事件类型，描述可省略且最多 1000 个 Unicode 码点';

COMMENT ON COLUMN ingest.parsing_rules.version IS '版本号，每次改动自增；事件按版本归因解析质量';

COMMENT ON COLUMN ingest.parsing_rules.created_at IS '创建时间（UTC）';

COMMENT ON COLUMN ingest.parsing_rules.updated_at IS '最后修改时间（UTC）';

COMMENT ON COLUMN ingest.parsing_rules.builtin IS '是否为平台内置的解析规则；内置规则只读，可复制为自定义规则';

COMMENT ON COLUMN ingest.parsing_rules.vendor IS '解析规则适配的厂商；可留空，最多 120 个 Unicode 码点';

COMMENT ON COLUMN ingest.parsing_rules.product IS '解析规则适配的产品；可留空，最多 120 个 Unicode 码点';

COMMENT ON COLUMN ingest.parsing_rules.verification IS '最近采用修订的验证摘要，仅含定义摘要、覆盖和通过情况，不含日志样本';

COMMENT ON COLUMN ingest.parsing_rules.author_id IS '最近采用修订的 IAM 用户 ID；空值表示安装或开发夹具';

ALTER TABLE ONLY ingest.audit_outbox
    ADD CONSTRAINT audit_outbox_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ingest.inputs
    ADD CONSTRAINT inputs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ingest.kafka_execution_boundaries
    ADD CONSTRAINT kafka_execution_boundaries_pkey PRIMARY KEY (input_id, partition, first_offset);

ALTER TABLE ONLY ingest.kafka_partition_heads
    ADD CONSTRAINT kafka_partition_heads_pkey PRIMARY KEY (input_id, partition);

ALTER TABLE ONLY ingest.parsing_rule_revisions
    ADD CONSTRAINT parsing_rule_revisions_pkey PRIMARY KEY (parsing_rule_id, version);

ALTER TABLE ONLY ingest.parsing_rules
    ADD CONSTRAINT parsing_rules_pkey PRIMARY KEY (id);

CREATE INDEX audit_outbox_queue_idx ON ingest.audit_outbox USING btree (queued_at, id) WHERE (rejected_at IS NULL);

CREATE INDEX inputs_parsing_rule_idx ON ingest.inputs USING btree (parsing_rule_id) WHERE (parsing_rule_id IS NOT NULL);

CREATE UNIQUE INDEX inputs_name_normalized_key ON ingest.inputs USING btree (name_normalized);

CREATE INDEX inputs_protocol_enabled_idx ON ingest.inputs USING btree (protocol, enabled);

CREATE UNIQUE INDEX inputs_syslog_port_key ON ingest.inputs USING btree (((configuration ->> 'port'::text))) WHERE ((protocol = 'syslog'::text) AND (configuration ? 'port'::text));

CREATE UNIQUE INDEX parsing_rules_name_normalized_key ON ingest.parsing_rules USING btree (name_normalized);

CREATE TRIGGER record_parsing_rule_revision AFTER INSERT OR UPDATE OF name, vendor, product, definition, version ON ingest.parsing_rules FOR EACH ROW EXECUTE FUNCTION ingest.record_parsing_rule_revision();

ALTER TABLE ONLY ingest.inputs
    ADD CONSTRAINT inputs_parsing_rule_id_fkey FOREIGN KEY (parsing_rule_id) REFERENCES ingest.parsing_rules(id) ON DELETE RESTRICT;

ALTER TABLE ONLY ingest.kafka_execution_boundaries
    ADD CONSTRAINT kafka_execution_boundaries_parsing_rule_id_parsing_rule_version_fkey FOREIGN KEY (parsing_rule_id, parsing_rule_version) REFERENCES ingest.parsing_rule_revisions(parsing_rule_id, version);

-- +goose Down
DROP TABLE ingest.kafka_execution_boundaries, ingest.kafka_partition_heads, ingest.inputs,
    ingest.parsing_rule_revisions, ingest.parsing_rules, ingest.audit_outbox;
DROP FUNCTION ingest.record_parsing_rule_revision();
