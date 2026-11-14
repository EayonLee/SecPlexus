# 实施与验收

依据 [spec.md](spec.md)。按测试先行推进：每项先写失败测试，再实现。测试分层按 [testing.md](../../development/testing.md) 判定表执行。

## 依赖关系

四条主线，骨架先行：

1. 服务骨架 + 配置 + IAM 接入是其余一切的地基，必须最先。
2. 解析层（格式探测 + 解析引擎）不依赖任何外部组件，可与骨架并行开发、独立交付。
3. 三通道（HTTP / syslog / Kafka）依赖骨架与解析层，三者在骨架就绪后可并行。
4. connectors 注册表与热重载依赖骨架；syslog 通道依赖注册表。

```
骨架/配置/IAM ──┬── HTTP 通道 ──┐
               ├── Kafka 通道 ──┼── 发布(parse_status 语义) ── 集成验收
               └── 注册表 ── syslog 通道
解析层(探测+解析规则) ────────────────┘
```

## 任务

### 第一组：服务骨架与配置

- [ ] 失败测试先行：`check-config` 对缺失必填项（Kafka brokers、IAM token）报字段级错误；未知字段拒绝。
- [ ] `src/apps/ingest` 骨架：`cmd`（check-config/serve）、`internal/{config,bootstrap,router,handler}`，复用 `httpx`/`logging`/`result`/`iamclient`/`configuration`。migrate 命令与第一张迁移表一起在第五组落地——没有表的 migrate 是空壳。
- [ ] `src/apps/ingest/config.yaml`：监听（默认端口 8082）、Kafka、Redis、IAM `service_clients`、通道开关，全部白话注释。
- [ ] 根 `.env.example` 增加 `SPX_INGEST_*` 分区；`Makefile`/根启动脚本纳入 ingest。
- [ ] `deploy/docker/go.Dockerfile` 的 `ARG APP=ingest` 构建路径验证。
- [ ] IAM 不新增权限点（全员可见决策）；数据接入页面挂在主导航「数据源」分组。

### 第二组：解析层（无外部依赖）

- [ ] 失败测试先行：格式探测表驱动用例（JSON/CEF/KV/纯文本/畸形输入）。
- [ ] 分层接口：`Parser interface { Parse(raw []byte) (map[string]any, error) }` + `map[string]Parser` 注册表（O(1) 分发）；线格式解析与规则映射两层职责分离（AiSOC 同构）。
- [ ] 五个 Parser 实现：json（encoding/json，单对象/数组/NDJSON 按首字节分发+降级）、regex（regexp 命名捕获组，加载时预编译缓存；复杂场景 elastic/go-grok）、kv（状态机，键值/条目/引号分隔符可配）、delimiter（encoding/csv 自定义分隔符+列名）、cef（ubyte-source/go-cef 或 AiSOC 式解析器）；引入 `leodido/go-syslog/v4`。
- [ ] 工程约束落地（5w EPS 容量目标）：固定 worker 池（默认=CPU 核数）从 channel 消费，无 per-event goroutine；批处理贯穿全链（500 条/批或 2s）；sync.Pool 复用扁平 map 与 OCSF map；NDJSON 流式解码；每批 panic recover + worker 自动重启；worker/buffer 可配（SPX_INGEST_WORKERS/BUFFER）；优雅停机（停接收→排空→发布→提交位移→退出）。
- [ ] 容量基准测试：5 万 EPS sustained 压测（go test -bench 或独立 bench 工具），断言 P99 端到端延迟 < 2s、GC CPU < 10%、无 OOM；结果记录进 spec 容量模型节作为实测校准。
- [ ] 失败测试先行：五模式表驱动用例——regex 命名分组提取、KV 多分隔符与引号内分隔符、delimiter 列名对齐、多条样本全部命中才通过。
- [ ] 失败测试先行：解析引擎——加载校验（非法 class_uid、severity 越界拒绝启动）、dot-path 读写（含 `x[0]`）、`type: int` 转换失败置 null+warning、`extends` 继承与逐键覆盖、`constants`、`unmapped` 收集、`time_formats` 失败置 null。
- [ ] 失败测试先行：severity 归一——中文档位（高危/中危/低危）、CEF 0–10 数字档、大小写/空白归一、未映射 → 0 + warning。
- [ ] 失败测试先行：`event_id` UUIDv5 确定性（同输入同输出、connector 维度隔离（UUID 命名空间））。
- [ ] 内置解析规则 `internal/normalizer/rules/*.yaml` + `go:embed`；解析规则 schema 校验函数。

### 第三组：通道

- [ ] 失败测试先行（集成）：HTTP——token 允许/拒绝、413 超限、415 类型、429 限流、202 计数、Redis 故障拒绝。
- [ ] HTTP 通道实现：per-connector token、请求体/数组上限、Redis 限流、异步解析发布。
- [ ] 可信代理网段（app.trusted_proxy_cidrs）随 HTTP 通道一起落地：路由按 iam 的方式启用 X-Forwarded-For 解析并拒绝 0.0.0.0/0——骨架阶段删除了无消费方的死配置。
- [ ] 失败测试先行（集成）：syslog——真实 UDP/TCP 端口收包、RFC6587 两种分帧、RFC3164/5424 识别、CEF 前缀路由、64KiB 截断。
- [ ] syslog 通道实现：按注册表绑定监听、周期刷新 + SIGHUP 热重载、UDP 无回错、TCP 解析失败仅计数。
- [ ] 失败测试先行（集成）：Kafka 消费——earliest 起消费、发布成功才提交位移、发布失败不提交（重启后重消费）。
- [ ] Kafka 通道实现：接入点驱动、独立消费组、手动位移提交。

### 第四组：发布与解析状态

- [ ] 失败测试先行（集成）：分区键按实体标识保序（同实体同分区）、无实体回退 event_id、不回退固定常量；解析失败事件同样保序发布且携带 parse_status。
- [ ] 发布器实现：channel 背压、批量 500 条/2s、确定性 event_id。
- [ ] 解析状态：失败事件照常发布到 `spx.ingest.event`，带 `metadata.parse_status=error` 与原因码数组；解析失败率指标（无独立隔离区 topic）。

### 第五组：connectors 注册表与管理 API

- [ ] 失败测试先行（集成）：创建（通道不可变；解析规则可换且换后从下一条消息起生效）、列表、启停、换 token、换解析规则、syslog 端口唯一性约束、删除接入点物理删除、删除解析规则有关联时 409。
- [ ] 失败测试先行（集成）：向导样本验证失败时提供「换解析规则」与「通用接入（原文透传 message，class_uid 6003）」两条出路；通用回退创建的接入点后续可换成正式解析规则。
- [ ] 迁移 `ingest.connectors` 表（goose，对齐 iam 的 repository.Migrate 模式）并落地 `cmd migrate` 命令；管理路由挂 `/api/v1/ingest/connectors`，IAM 用户会话 + 权限点校验。
- [ ] 解析规则回归：18 家解析规则 × 5–10 条真实样本，断言关键字段与 severity；填充率报表脚本。

## 验证入口

```sh
go run ./src/apps/ingest/cmd check-config --config src/apps/ingest/config.yaml
go test ./src/apps/ingest/...
go test -tags=integration ./src/apps/ingest/...   # 需 SPX_TEST_POSTGRES_URL/SPX_TEST_REDIS_URL + Kafka（--docker）
pnpm catalogs:check
make build
gofmt -l src/
go vet ./src/...
```

集成测试需要 Kafka：`--docker` 模式扩展测试脚本创建临时 Kafka（KRaft 单节点）容器，与既有 PG/Redis 隔离容器同生命周期管理；仅清理本次创建的资源。

界面按 [prototype.html](prototype.html) 实施，视觉以 DESIGN.md 为准。

浏览器/接口实测项：HTTP 通道用真实 token 推送样本告警，观察 `spx.ingest.event` 收到 OCSF 事件、`event_id` 确定性、限流触发 429；syslog 用 `logger`/`nc` 向绑定端口发 RFC5424 与 CEF 样本，核对发布结果。

## 验收证据

实施后在此记录。未完成前不得标记完成。

## 风险与约束

- Kafka 是第一版唯一缓冲层：`spx.ingest.event` 保留期即重放窗口，容量规划必须显式给出（部署文档），不可默认无限期。
- `ubyte-source/go-cef` 要求 Go 1.25+，仓库 1.27.3 满足；升级 Go 时注意其兼容线。
- syslog UDP 端口 <1024 需要特权或 `cap_net_bind_service`，文档写明推荐 ≥1024 的端口段。
- 18 家内置解析规则中 10 家国产厂商格式无公开规范，样本依赖真实设备日志；首版允许部分解析规则标记 `experimental`，解析失败率暴露映射质量。
- 客户现场已有的 syslog 转发链路（rsyslog 中继）会把来源 IP 变成中继 IP——端口绑定不受影响，但文档必须写明不要依赖源 IP 做归属。
- 本计划不引入 ClickHouse 依赖；correlator 计划评审时再定存储写入细节，避免两个计划互相阻塞。
