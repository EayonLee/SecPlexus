# 实施与验收

依据 [spec.md](spec.md)。按测试先行推进：每项先写失败测试，再实现。测试分层按 [testing.md](../../development/testing.md) 判定表执行。

## 依赖关系

四条主线，骨架先行：

1. 服务骨架 + 配置 + IAM 接入是其余一切的地基，必须最先。
2. 解析层（格式探测 + 模板引擎）不依赖任何外部组件，可与骨架并行开发、独立交付。
3. 三通道（HTTP / syslog / Kafka）依赖骨架与解析层，三者在骨架就绪后可并行。
4. connectors 注册表与热重载依赖骨架；syslog 通道依赖注册表。

```
骨架/配置/IAM ──┬── HTTP 通道 ──┐
               ├── Kafka 通道 ──┼── 发布(与隔离区) ── 集成验收
               └── 注册表 ── syslog 通道
解析层(探测+模板) ────────────────┘
```

## 任务

### 第一组：服务骨架与配置

- [ ] 失败测试先行：`check-config` 对缺失必填项（Kafka brokers、IAM token）报字段级错误；未知字段拒绝。
- [ ] `src/apps/ingestor` 骨架：`cmd`（check-config/serve/migrate）、`internal/{config,bootstrap,router,handler}`，复用 `httpx`/`logging`/`result`/`iamclient`/`configuration`。
- [ ] `src/apps/ingestor/config.yaml`：监听（默认端口 8082）、Kafka、Redis、IAM `service_clients`、通道开关，全部白话注释。
- [ ] 根 `.env.example` 增加 `SPX_INGESTOR_*` 分区；`Makefile`/根启动脚本纳入 ingestor。
- [ ] `deploy/docker/go.Dockerfile` 的 `ARG APP=ingestor` 构建路径验证。
- [ ] IAM 权限目录：新增 `ingestor.connectors.read/manage`，跑 `pnpm catalogs:generate` 同步前端常量。

### 第二组：解析层（无外部依赖）

- [ ] 失败测试先行：格式探测表驱动用例（JSON/CEF/LEEF/KV/纯文本/畸形输入）。
- [ ] 引入 `leodido/go-syslog/v4`、`ubyte-source/go-cef`；KV 解析器自写（多分隔符探测）。
- [ ] 失败测试先行：模板引擎——加载校验（非法 class_uid、severity 越界拒绝启动）、dot-path 读写（含 `x[0]`）、`type: int` 转换失败置 null+warning、`extends` 继承与逐键覆盖、`constants`、`unmapped` 收集、`time_formats` 失败置 null。
- [ ] 失败测试先行：severity 归一——中文档位（高危/中危/低危）、CEF 0–10 数字档、大小写/空白归一、未映射 → 0 + warning。
- [ ] 失败测试先行：`event_id` UUIDv5 确定性（同输入同输出、connector 维度隔离）。
- [ ] 内置模板目录 `internal/normalizer/templates/*.yaml` + `go:embed`；模板 schema 校验函数。

### 第三组：通道

- [ ] 失败测试先行（集成）：HTTP——token 允许/拒绝、413 超限、415 类型、429 限流、202 计数、Redis 故障拒绝。
- [ ] HTTP 通道实现：per-connector token、请求体/数组上限、Redis 限流、异步解析发布。
- [ ] 失败测试先行（集成）：syslog——真实 UDP/TCP 端口收包、RFC6587 两种分帧、RFC3164/5424 识别、CEF/LEEF 前缀路由、64KiB 截断。
- [ ] syslog 通道实现：按注册表绑定监听、周期刷新 + SIGHUP 热重载、UDP 无回错、TCP 解析失败仅计数。
- [ ] 失败测试先行（集成）：Kafka 消费——earliest 起消费、发布成功才提交位移、发布失败不提交（重启后重消费）。
- [ ] Kafka 通道实现：接入点驱动、独立消费组、手动位移提交。

### 第四组：发布与隔离区

- [ ] 失败测试先行（集成）：分区键按实体标识保序（同实体同分区）、无实体回退 event_id、不回退固定常量。
- [ ] 发布器实现：channel 背压、批量 500 条/2s、确定性 event_id。
- [ ] 隔离区：解析失败/模板缺失 → `spx.ingest.quarantine`（原因码），指标计数。

### 第五组：connectors 注册表与管理 API

- [ ] 失败测试先行（集成）：创建（通道与模板绑定不可变）、列表、启停、换 token、syslog 端口唯一性约束、越权访问拒绝。
- [ ] 迁移 `ingestor.connectors` 表；管理路由挂 `/api/v1/ingestor/connectors`，IAM 用户会话 + 权限点校验。
- [ ] 模板回归：18 家模板 × 5–10 条真实样本，断言关键字段与 severity；填充率报表脚本。

## 验证入口

```sh
go run ./src/apps/ingestor/cmd check-config --config src/apps/ingestor/config.yaml
go test ./src/apps/ingestor/...
go test -tags=integration ./src/apps/ingestor/...   # 需 SPX_TEST_POSTGRES_URL/SPX_TEST_REDIS_URL + Kafka（--docker）
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
- 18 家内置模板中 10 家国产厂商格式无公开规范，样本依赖真实设备日志；首版允许部分模板标记 `experimental`，隔离区计数暴露映射质量。
- 客户现场已有的 syslog 转发链路（rsyslog 中继）会把来源 IP 变成中继 IP——端口绑定不受影响，但文档必须写明不要依赖源 IP 做归属。
- 本计划不引入 ClickHouse 依赖；correlator 计划评审时再定存储写入细节，避免两个计划互相阻塞。
