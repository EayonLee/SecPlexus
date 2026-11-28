# Ingest｜安全事件接入与存储

Ingest 通过 HTTP、Syslog 和 Kafka 接收安全日志，用解析规则把每条日志转换为标准 Event。Event 先发布到内部 Kafka，再由独立进程写入 ClickHouse。IAM 负责管理鉴权与审计，Console 负责运营交互。平台检测、关联和 Alert 生成属于后续模块。[Event 与 Alert](../../../docs/plans/event-contract/spec.md)

## Event 字段与存储设计

修改字段时，先看 `schema/events/` 下这四个文件。

| 文件 | 用途 |
| --- | --- |
| [event-contract.json](schema/events/event-contract.json) | **人工维护的契约源。** 固定 OCSF 和编译器版本，选定支持的事件类与字段。中文类别名在 `category_labels`，中文事件类型名在 `classes`，字段、释义及平台补充约束也在此维护。 |
| [generate.py](schema/events/generate.py) | **构建脚本。** 读取契约源和锁定版本的 OCSF 编译产物，合成字段目录，计算内容摘要。`--check` 检查生成结果与仓库一致。 |
| [catalog.json](schema/events/catalog.json) | **生成的字段目录。** 包含各事件类的字段路径、类型、必填条件及枚举。修改从契约源开始，再重新生成。 |
| [catalog.go](schema/events/catalog.go) | **Go 运行时加载入口。** 将 `catalog.json` 嵌入二进制，首次使用时加载并建立按类和字段路径的索引，供规则校验、字段查询 API 和存储投影使用。 |

当前平台契约 `v1` 的唯一领域标准是锁定的 OCSF `1.10.0`。厂商日志经规则映射到这套事件类、字段、枚举和约束。生成时将上游源码下载到临时目录。[`upstream/`](schema/events/upstream) 保存生成目录所用 OCSF 内容的 [Apache 2.0 许可](schema/events/upstream/LICENSE)及 [NOTICE](schema/events/upstream/NOTICE)。部署和运行读取内嵌目录，可在离线环境使用。

用 Python 3.14 和固定编译器版本生成或核对目录，命令在仓库根目录执行。

```sh
python -m pip install ocsf-schema-compiler==1.0.1
python src/apps/ingest/schema/events/generate.py
python src/apps/ingest/schema/events/generate.py --check
```

| 存储 | 内容 |
| --- | --- |
| PostgreSQL | 数据接入、规则、规则修订和待投递审计。 |
| Redis | HTTP 数据接入限流。 |
| Kafka | 标准 Event 和独立的写入失败主题。失败记录含稳定原因和原消息坐标。 |
| ClickHouse `spx.events` | 类型化查询列及完整的 `event_json`。[物理投影](internal/repository/event/projection.json)与字段目录核对路径、类型和值域。 |

Event 的 OCSF 字段在 JSON 根级。通过 `metadata.uid` 查事件，通过 `metadata.spx` 查数据接入、规则修订、契约版本和解析状态。更新字段时一并检查 ClickHouse 投影。发布后的物理变更走前向迁移，运行时检查列、注释、约束、表引擎、排序键、分区和永久保留策略。[存储与变更设计](../../../docs/plans/event-contract/storage.md)

## 本地启动与验证

按 [根目录环境模板](../../../.env.example)填写凭据后运行 `make compose-up`。Compose 启动自带 PostgreSQL、Redis、Kafka 与 ClickHouse，初始化事件主题后启动写入进程。独立运行时须填写实际的 `SPX_KAFKA_BROKERS`，并按配置指南填写其他外部连接地址。Ingest 默认在 `127.0.0.1:8082` 监听。[配置指南](../../../docs/development/configuration.md)

```sh
make config MODULES="iam console ingest web"
make dev MODULES="iam console ingest web"
```

在「解析规则」新建 JSON 规则，事件类型选择「网络活动」，将 `src` 和 `dst` 映射到源 IP 和目的 IP。再在「数据接入」新建并启用绑定这条规则的 HTTP 数据接入，用一次性显示的设备令牌发送样本。源码直连用 Ingest 监听地址，默认 `http://127.0.0.1:8082`。整栈 Compose 用 Web 同源入口，默认 `http://127.0.0.1:5173`。

```sh
INGEST_BASE_URL=http://127.0.0.1:8082  # 整栈 Compose 默认使用 http://127.0.0.1:5173
curl --fail-with-body -X POST "${INGEST_BASE_URL}/api/v1/ingest/events/${INPUT_ID}" \
  -H "Authorization: Bearer ${INPUT_TOKEN}" \
  -H 'Content-Type: application/json' \
  --data '{"src":"192.0.2.12","dst":"198.51.100.4"}'
```

HTTP 返回的 `accepted` 是 Kafka 已确认的条数。完整链路由 `make compose-up` 启动，数据接入、Kafka 确认和 ClickHouse 已写入是三个不同的状态。

Syslog 详情显示 Ingest 服务内的监听状态。设备使用部署主机的地址和可访问端口。容器部署按需发布已登记的 TCP 或 UDP 端口。 [Compose 端口示例](../../../docs/design/architecture/03-deployment-architecture.md#syslog-端口)

### ClickHouse 事件存储

独立部署时设置共用的 `SPX_CLICKHOUSE_URL`，指定 Ingest 所属数据库及三个账号。迁移账号负责 DDL，写入账号拥有事件表的 SELECT、INSERT，查询账号仅 SELECT。内置 Compose 创建数据库和受限账号。

```sh
make event-store
make event-store ACTION=init-event-topics
make event-store ACTION=migrate-event-store
make event-store ACTION=check-event-store
make event-store ACTION=store-events
```

平台事件主题默认保留 7 天，配置键为 `SPX_KAFKA_RETENTION_DAYS`。先检查未消费位移，再显式变更已有主题并核对有效值。ClickHouse 永久保存已写入的事件，按发生时间使用 UTC 月分区。持有 `ingest.events.read` 的用户可按事件 ID 和时间范围回查。

起始位移过期时，写入进程暂停受影响分区并记录 `ingest.events.offset_unavailable`，其他分区继续。保留原消费组和主题身份，核对缺口后再恢复。 [结构、查询和恢复](../../../docs/plans/event-contract/storage.md)

### 修改已有主题的保留期

在根 `.env` 设置 `SPX_KAFKA_RETENTION_DAYS`。已有主题由 Kafka 管理员修改，初始化命令只创建缺少的主题。

1. 检查事件写入消费组的积压和最老未消费记录。缩短之前确认未写入的事件仍落在新的保留范围内。
2. 使用 Kafka 管理工具更新两个平台业务主题的 `retention.ms`。使用默认命名空间时，主题是 `spx.ingest.events` 和 `spx.ingest.event-failures`。天数乘以 86,400,000 得到毫秒数。
3. 回读两个主题，确认 `retention.ms` 等于目标值、`cleanup.policy=delete`、`retention.bytes=-1`。再启动或重新创建 Ingest 与事件写入进程，观察消费进度。

整栈 Compose 可从 Kafka 容器执行管理命令。先通过 `docker compose ls` 确认项目名并设置 `COMPOSE_PROJECT_NAME`。下面以默认命名空间、3 天为例。

```sh
for topic in spx.ingest.events spx.ingest.event-failures; do
  docker compose --project-directory . --env-file .env -p "$COMPOSE_PROJECT_NAME" \
    -f deploy/compose/compose.yaml exec -T kafka /opt/kafka/bin/kafka-configs.sh \
    --bootstrap-server kafka:9092 --entity-type topics --entity-name "$topic" \
    --alter --add-config retention.ms=259200000
done
```

将 `--alter --add-config retention.ms=259200000` 换成 `--describe` 可回读配置。外接集群使用有权限的管理身份连接实际 broker。 [Kafka 主题配置](https://kafka.apache.org/42/configuration/topic-configs/)

## 接入与解析

| 协议 | 关键行为 |
| --- | --- |
| HTTP | Bearer 令牌按数据接入认证。支持 JSON 对象、数组和纯文本。按 `accepted` 和 `Retry-After` 重送未确认后缀。确认未知时重送可能产生重复。 |
| Syslog | 数据接入绑定 UDP/TCP 端口，识别 RFC 3164/5424 和 RFC 6587。UDP 送达量结合设备和主机计数核对。 |
| Kafka | 从外部 broker 与 topic 消费。平台 Kafka 确认发布后提交源位移。同一规则重读时 Event ID 保持稳定。外部接入的 SASL/TLS 由对应凭据配置提供。 |

数据接入引用整份规则 `parsing_rule_id`。按 `parse.mode` 提取字段。多类型规则使用首个匹配分支，单一类型直接使用预先选定的事件类。无分类的通用规则也能生成合规的 Base Event。实际报文按规则声明的来源编码解码，默认 UTF-8。粘贴样本作为 UTF-8 文本试算。

键值对的 `parse.kv.entry`、`parse.kv.delimiter`、`parse.kv.quote` 分别对应字段分隔符、键值分隔符、引号。分隔符解析配置 `parse.delimiter.separator`。省略 `parse.delimiter.columns` 时，源字段按位置产生 `field_1`、`field_2` 等名称。每条样本、Syslog 报文或 Kafka 消息应包含一条记录。引号内的换行属于字段内容。包含额外记录的报文标记为 `multiple_records`，归入解析失败。HTTP 纯文本批量接入按行拆分。

| 解析状态 | 标准化结果 |
| --- | --- |
| `ok` | 输出合规事件。管理员登录失败也可以解析成功。 |
| `partial` | 保留通过校验的字段。缺少类别必填项时输出 Base Event，并记录候选类及原因。 |
| `error` | 无规则、未匹配或无法解码时输出 Base Event，记录失败阶段和原因。 |

管理 API 使用 `/api/v1/ingest`，写入需通过 IAM 授权与 CSRF 校验。

- `/inputs` 管理数据接入、绑定和设备令牌。`/inputs/counts-by-rule` 按规则 ID 统计绑定的数据接入数量。
- `/parsing-rules` 管理解析规则，`parse_type` 用于筛选和摘要，具体配置来自 `definition.parse.mode`。编辑提交 `expected_version`。
- `/parsing-rules/extract` 提取字段与类型，`/parsing-rules/validate` 返回 Event 预览和 `required_branches`。编辑预览传入 `parsing_rule_id` 与 `expected_version`，由服务端按同一修订判断待验证分支。试算和保存传请求顶层 `samples`，最多 20 条，每条最多 64 KiB。样本由当前表单持有，数据库定义和修订保存验证摘要。摘要包含定义校验值、覆盖分支、时间及通过数。规则详情展示最近一次通过数和验证时间。
- `/parsing-rules/:id/revisions` 按修订倒序读取，`/parsing-rules/:id/revisions/:version` 返回指定修订的详情。用 `/parsing-rules/:id/revisions/:version/restore` 将已验证定义恢复为新修订。恢复仍需当前版本，审计保留来源修订。
- `/event-schema` 提供分类与事件类，`/event-schema/classes/:class_uid` 提供类内字段和条件组，`/event-schema/fields` 提供完整字段目录。规则编辑和试算使用同一目录修订。

两个列表始终返回分页结果，默认 20 条。规则的 `q` 匹配名称、描述、厂商和产品，`class_uid` 匹配任意已配置的事件类型，摘要通过 `class_uids` 返回这些类型。数据接入的 `q` 匹配名称、厂商和产品，`rule` 按规则 ID 筛选。事件类型由规则定义派生，数据接入数量按规则绑定外键统计。

新建规则在基础配置选择一个具体事件类型。已有多类型模板仍按配置分流。字段映射按标准字段、源字段、默认值排列。配置了源字段时，默认值只用于该字段缺失的记录。手动添加的样本值留在编辑会话。运行时自动生成事件 ID（`metadata.uid`）。

映射页常驻公共基础行和当前事件类的必配行。缺少必配映射时留在当前步骤。常用选配来自目录的 `recommended_mapping_paths`，可以在当前草稿中移除。标准字段类型由锁定的 OCSF 目录提供，源字段类型来自日志样本。[逐类映射审查](../../../docs/plans/ingest-rule-field-workflow/plan.md#逐类映射审查)

裁剪和解码可用当前日志样本预览结果。HTTP 方法从 OCSF 枚举选择，HTTP 状态码按 HTTP 协议校验为 100–599。事件预览可切换字段结果和 OCSF JSON。[标准与取值依据](../../../docs/plans/ingest-rule-field-workflow/plan.md#字段操作与预览布局)

当前安装无需预置解析规则。迁移会清理旧内置规则及其修订，解除相关数据接入的绑定并停用，保留接入配置和设备令牌。重新配置规则后可恢复接入。[向导与字段契约设计](../../../docs/plans/ingest-rule-field-workflow/spec.md)

规则名称必填，去除首尾空白后最多 100 个 Unicode 码点，使用数据库唯一索引查重。厂商、产品和描述必填。厂商和产品各最多 120 个码点，描述最多 1,000 个码点。保存遇到重名或修订冲突时，API 返回可定位的 `field_errors`。[规则向导](../../../docs/plans/ingest-rule-wizard/spec.md) · [校验规格](../../../docs/plans/ingest-rule-validation/spec.md) · [升级前检查](../../../docs/plans/ingest-rule-validation/plan.md#升级前检查)

Syslog 的 TCP 监听器在每个 Ingest 实例共用 256 条活跃连接的额度。读操作等待 2 分钟后超时，达到连接上限时关闭新连接。部署时可按实际设备量和连接行为分配实例。

### 平台 Kafka 与 ClickHouse

`kafka` 保存平台 Kafka 的地址、传输方式及保留期。事件主题、写入失败主题及消费组从同一个 `SPX_KAFKA_NAMESPACE` 派生，默认是 `spx.ingest.events`、`spx.ingest.event-failures` 和 `spx-ingest-events`。每套共享 Kafka 的部署使用固定且唯一的命名空间。平台主题默认保留 7 天，可通过 `SPX_KAFKA_RETENTION_DAYS` 调整。

独立运行时用 `SPX_KAFKA_BROKERS`、`SPX_KAFKA_SECURITY_PROTOCOL`、`SPX_KAFKA_TLS_CA_FILE` 配置平台连接，发布与写入身份分别用 `SPX_INGEST_KAFKA_PUBLISH_SASL_*` 和 `SPX_INGEST_KAFKA_STORE_SASL_*`。ClickHouse 地址用 `SPX_CLICKHOUSE_URL` 配置。写入、回查和迁移身份分别使用 [模块配置](config.yaml) 中的用户名与密码字段。写入 ClickHouse 的事件永久保存。

### 外部 Kafka 凭据

每条 Kafka 数据接入选择一个部署配置中的凭据名。平台保存名称，口令只从部署环境读取。凭据只能用于该配置登记的引导服务器。只在无认证集群使用空凭据。

```yaml
input:
  kafka:
    credentials:
      site-a:
        brokers: [broker-a.example:9093]
        security_protocol: sasl_ssl
        sasl_mechanism: SCRAM-SHA-256
        sasl_username: ingest-site-a
        sasl_password: "${SPX_INGEST_SOURCE_SITE_A_PASSWORD-}"
        tls_ca_file: /etc/secplexus/ca/site-a.pem
```

轮换时保留 `site-a`，更新 Ingest 的部署凭据并重启副本。缺少配置或引导服务器不匹配时，接入不会使用该凭据连接。Kafka 事件发布者和写入器各自独立认证。 [Kafka 客户端安全配置](https://kafka.apache.org/42/security/)

## 维护与定位

`audit_outbox` 中 `rejected_at` 有时间的记录需要人工核查。IAM 暂时不可用或服务凭据失效会继续重试。内容被确定拒绝时保留原审计事实，并继续投递后续记录。

| 现象 | 从这里核对 |
| --- | --- |
| 请求未确认或返回 `503` | 查看数据接入状态、Kafka 确认时延、`spx_ingest_queue_depth`、`spx_ingest_publish_retry_total` 和 `spx_ingest_dropped_total`。 |
| 样本通过，实际解析失败 | 核对协议、编码、规则绑定与修订。用 `metadata.spx["parse.error"]`、`metadata.spx["parse.issues"]` 定位。配置刷新失败时数据面继续使用最近一次成功快照。 |
| Kafka 有数据，ClickHouse 查不到 | 核对消费位移、写入失败主题、实库结构与写入权限，用事件 ID 和时间窗回查。 |
| 配置已更新，审计暂缺 | 检查 `audit.delivery_failed`、IAM 服务权限和 `ingest.audit_outbox`。 |

管理操作与待投递审计由同一个 PostgreSQL 事务写入。重投沿用出箱记录的事件 ID 和入队时间，IAM 确认后方可移除待投递记录。

`GET /health/ready` 检查就绪，`GET /metrics` 查看运行指标。升级前验证在用规则与代表性样本，记录源位移。首发 PostgreSQL 模式由 [建库基线](migrations/postgresql/00001_init.sql) 定义。规则保存与修订恢复使用同一套校验和归一化定义。样本验证复用保存时生成的解析计划，规则与修订按预期版本写入。[容量与资源预算](../../../docs/plans/event-contract/performance.md)

监控 `spx_ingest_source_snapshot_age_seconds` 检查配置刷新，达到 15 秒时旧快照无法继续受理。`spx_ingest_kafka_offsets_rejected_total` 增长时，从 `kafka.partition.offset_unavailable` 日志定位数据接入和分区，再核对消费组提交位移、主题 UUID 和保留范围。`spx_ingest_audit_outbox_available` 为 0 时检查数据库连接，为 1 时可按最老等待时长和积压定位投递故障。指标按 [Prometheus 文本格式](https://prometheus.io/docs/instrumenting/exposition_formats/#text-based-format) 输出。

创建 Kafka 数据接入时填写 `kafka_start_position`。`earliest` 读取最早可用消息，`since_created` 按 Kafka 记录时间从接入创建时刻开始。两种选项都沿用已有提交位移，位移超出保留范围时暂停对应分区。按创建时间开始时，重启仍使用原时间下界。[franz-go 消费位移](https://pkg.go.dev/github.com/twmb/franz-go@v1.22.3/pkg/kgo#ConsumeStartOffset)

## 源码与验证入口

- 管理与启动：[router](internal/router) · [handler](internal/handler) · [cmd](cmd) · [config.yaml](config.yaml)
- 接收与解析：[channel](internal/channel) · [parser](internal/parser) · [normalizer](internal/normalizer) · [pipeline](internal/pipeline)
- 发布与事件存储：[publish](internal/publish) · [consumer](internal/consumer) · [repository/event](internal/repository/event) · [数据库迁移](migrations)

```sh
go test -race ./src/apps/ingest/...
make integration
make test-e2e ARGS="ingest.spec.ts"
```

集成与浏览器测试使用显式配置的隔离 PostgreSQL、Redis 和 Kafka。事件存储测试用 `make integration ARGS="--clickhouse"`，配置本次测试独占的 `SPX_TEST_CLICKHOUSE_URL`。[测试与隔离](../../../docs/development/testing.md)


### 运行状态与位移

接入配置每 5 秒刷新，读取失败时上一份快照最多可继续受理 15 秒。配置或规则错误只隔离对应来源。实例在 Redis 登记租期并确认实际采用的接入配置、凭据代次及规则修订。管理面只有在仍可受理的节点均确认并有监听或消费入口可用时才显示已生效。Kafka 批次发布前，按来源、主题 UUID 及分区登记已准备的位移和规则边界。已发布但确认不明的记录重试时沿用原配置。Broker 位移缺失或超出可用范围时暂停分区。归一化事件保留独立的来源记录 ID 与事件 ID。

接入详情从 Redis 读取最近五个分钟桶。接收与 Kafka 确认分开统计，进程纪元内按累积值合并。仍可受理的实例均有近期上报时，统计可用。历史确认时间过期且无法确定来源是否曾收到日志时，显示暂不可确认。事件表结构校验按已部署 DDL 核对。`ingest check-kafka --store` 检查写入消费组权限，`ingest check-event-store` 检查事件表及写入账号权限。[Kafka ACL](https://kafka.apache.org/42/security/authorization-and-acls/) · [ClickHouse CHECK GRANT](https://clickhouse.com/docs/reference/statements/check-grant)
