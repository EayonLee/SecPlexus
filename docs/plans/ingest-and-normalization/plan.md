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

- [x] 失败测试先行：格式探测表驱动用例（JSON/CEF/KV/纯文本/畸形输入）。
- [x] 分层接口：`Parser interface` + `Factory` 注册表（构造期预编译、运行期零准备）；线格式解析与规则映射两层职责分离。
  - 与计划的偏差：注册表登记的是**构造函数**而不是现成实例——regex/kv/delimiter 需要规则参数，且正则必须在构造期编译，裸 `map[string]Parser` 无法表达这一点。
- [x] 五个 Parser 实现：json、regex、kv、delimiter、cef。全部标准库实现，**未引入任何新依赖**。
  - json：单对象/数组/NDJSON 统一流式解码（`{` 开头既可能是单对象也可能是 NDJSON，只按首字节判断会漏）。
  - cef：手写（spec 允许的 AiSOC 式方案）；头部 7 段只切 7 次以免截断含 `|` 的扩展值；扩展区条目边界按「后面是否紧跟 key=」判定，否则含空格的值（msg、cmd）会被截断。
  - kv：状态机；默认支持双引号（logfmt 主流写法），转义只对 CEF 开启（Windows 路径里的 `\t` 不能被吃掉）。
  - 延后：`leodido/go-syslog/v4` 与 go-grok 随 syslog 通道（第三组）引入，当前无消费方。
- [x] 工程约束落地（解析层部分）：正则预编译缓存、NDJSON 流式解码、构造期校验。
  - 延后到第三组（通道层）：worker 池、批处理、sync.Pool、panic recover、优雅停机——这些只有在有接收端与发布端之后才成立，现在写是空转。基准显示 56 allocs/op，池化收益需要结合真实管线测，不先猜。
- [x] 容量基准测试：单核实测 CEF 3135 ns/op（319k 条/秒）、JSON 2238 ns/op（447k 条/秒）；5 万 EPS 仅占 0.16 核。端到端 P99 与 GC 指标待第三组有完整管线后测。
- [x] 失败测试先行：五模式表驱动用例——regex 命名分组提取、KV 多分隔符与引号内分隔符、delimiter 列名对齐、多条样本全部命中才通过。
- [x] 失败测试先行：解析引擎——加载校验（severity 越界拒绝加载）、dot-path 读写、`constants` 最后盖章、`unmapped` 收集、`time_formats` 失败记 warning 不写时间。
  - 延后：`extends` 继承与逐键覆盖（只有内置规则用得上，等 18 条规则一起做）；`type: int` 转换（真实样本里暂未出现需要强转的字段，等规则回归时按需加）；数组下标 `x[0]`（无规则需要逐元素映射，整值写入已足够）。
- [x] 失败测试先行：severity 归一——大小写/空白归一、未映射 → 0 + warning。值表由规则声明，中文档位在具体规则里配置。
- [x] 失败测试先行：`event_id` UUIDv5 确定性（同输入同输出、connector 维度隔离）。用原始字节而非归一化结构做指纹：少一次序列化，字节相同就是同一条事件。
- [ ] 内置解析规则 `internal/normalizer/rules/*.yaml` + `go:embed`；解析规则 schema 校验函数。**等用户提供真实设备日志样本**：没有样本的规则按样本驱动校验必然失败，写出来是假的。

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

## 代码审查修复（批次 2 完成后）

对解析与映射做过一轮 P0/P1/P2 审查（10 个探针验证 + 对照测试），以下问题已全部修复并有回归测试钉住：

- **P0**：event_id 只按内容指纹会吞掉内容相同的真实事件 → 改为（接入点, 接收序号, 字节）三元指纹；unmapped 明文承载凭证 → 敏感名单脱敏（[REDACTED]）。
- **P1**：无时区时间串按 UTC 解析导致时间线偏移 → 规则级 time.location；/health/ready 恒真 → Ready 钩子 + 503（依赖探活随批次补入）；channels 死配置 → 删除（端口绑定属 connectors 注册表）；产出缺 class_uid/severity_id/time 的伪 OCSF → class_uid 加载必填 + 引擎恒写级别与时间；常量与映射目标前缀冲突静默覆盖 → 加载拒绝；注册表一致性弱测试 → 真断言。
- **P2**：CEF 头部转义还原、JSON 数组尾随垃圾拒绝、错误信息指引 Split、json.Number 保数字精度（>2^53 不截断）、Modes 返回副本、删除未消费的 Server.IAM 与 serve 期 token 强制（管理 API 批次随路由启用）、JSON 10MiB 解码上限、纯空白输入全模式一致拒绝。
- 已知延后：\-race 本机无 gcc 跑不了（CI Linux runner 会跑）；64KiB/10MiB 边界与超长字段的规模测试随通道层一起做。

### 二轮审查（修复回归审查）

对一轮修复再做一轮探针审查，发现 4 个由修复本身引入的问题，已修复并有回归测试：

- **P1**：恒写保障静默覆盖 field_map 对 severity_id/time 的直接映射（探针：映射值 4 → 实际 0）→ 加载期拒绝这两个目标并指路（TimeSpec/SeveritySpec 是唯一声明方式）；常量路径保留——常量最后盖章，是固定级别源的唯一写法（测试钉住）。
- **P2**：值表键不做归一化，"HIGH" 永远匹配不到 "high" → Compile 时归一化一次 + 归一后冲突检测（迭代顺序不定，必须拒绝）。
- **P2（性能）**：isSensitiveKey 每次调用构造 Replacer（热路径按字段分配）→ 包级构造一次。
- **P3**：class_uid 显式 nil 通过编译 → 值非空校验（缺键与 nil 同判）。

### 持续化测试（语料 + 模糊）

- [x] 语料库 `internal/parser/testdata/corpus/`：六种变体（json/cef/kv/kv-colon/regex/delimiter-comma）+ syslog 三份（rfc5424/rfc3164/cef-over-syslog），走真实 `Split → Parse` 流程。
- [x] 语料测试 `TestCorpusParsesEverySample`（按变体表驱动）与 `TestCorpusArraySplitsPerElement`。
- [x] 原生模糊测试：`FuzzParseEveryMode`、`FuzzSplitNeverPanics`、`FuzzSplitFrames`、`FuzzFlattenNeverPanics`，语料即种子；公共性质为不 panic、零字段不成功、分帧有界。
- [x] 截断语义：`metadata.truncated` + `original_bytes`，且 `parse_status` 强制 error（原因码 `truncated`）——模糊与语料之外的第三条护栏。
- [ ] 从 Loghub / Azure Sentinel Sample Data 导入真实样本子集（需先确认各自许可并在文件头注明来源）。
- [ ] 在 CI 增加定时模糊测试任务（夜间跑 5 分钟，发现新语料即提 issue）。

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
