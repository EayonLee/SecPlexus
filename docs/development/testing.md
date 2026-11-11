# 测试分层与用例维护

## 改动类型与必需测试层

下表是判定依据，不是建议。新增或修改功能时按第一列找到对应行，逐项满足「必须具备」，不得以其他层替代。表中没有的行按最接近的类别判断；无法判断时提升一层，不要降级。

| 改动类型 | 必须具备 | 不需要 |
|---|---|---|
| 纯函数、校验、状态转换、配置解析优先级 | 单元测试，表驱动覆盖边界与非法输入 | 集成、E2E |
| HTTP 路由新增或权限语义变更 | 集成测试：真实路由、真实服务、真实数据库，覆盖允许与拒绝两侧 | 为每个字段做 E2E |
| 数据库迁移、事务、外键、并发不变量 | 集成测试：真实 PG，含迁移前后状态与失败回滚 | Mock 数据库「证明」事务正确 |
| 跨服务认证、服务凭证、会话持久化 | 集成测试：真实组件契约 | 单元测试断言 Mock 返回值 |
| 用户可见核心旅程、访问边界 | E2E：真实浏览器与真实后端 | 逐按钮、逐字段穷举 |
| 前端展示逻辑、回落顺序、格式化 | 单元测试 | E2E |
| 纯样式、文案、颜色令牌、文档 | 不新增自动化测试；由 DESIGN.md 与视觉验收覆盖 | 断言 CSS 类名、文案字符串或文档措辞 |
| 定时任务、重试、超时 | 单元测试覆盖计算逻辑，集成测试覆盖真实调度一次 | 依赖 sleep 等待真实时间流逝 |

强制规则：

- 行为变更先有失败测试；未观察到失败就写实现，视为未验证。写了实现才补测试，删除实现重写。
- 禁止用 sleep、固定等待或重试掩盖时序问题；用条件等待并设上限。
- 禁止删除、跳过或放宽失败测试来变绿。
- 禁止断言实现细节（内部调用次数、私有字段）代替断言可观察行为。
- 断言机器可消费的值（解析后的字段、稳定令牌、边界错误码），不断言面向人阅读的散文。
- 集成测试必须使用自有隔离资源并清理；不得连接或改写开发库。
- 缺少 `SPX_TEST_POSTGRES_URL` 或 `SPX_TEST_REDIS_URL` 时，集成测试跳过而非失败；跳过不算覆盖。

## 原则

按可观察行为和最小必要边界选择测试层次，越高层越少、越聚焦，不规定机械比例。单元测试覆盖规则和边界；集成测试覆盖真实组件契约；E2E覆盖关键用户旅程。一个断言能在低层可靠验证，就不在每层重复穷举。

| 层次 | 覆盖内容 | 不应承担 |
|---|---|---|
| 单元/组件测试 | 纯规则、状态转换、输入边界、配置优先级、界面交互；外部I/O隔离 | 真实数据库约束或整个系统是否可用的证明 |
| 集成测试 | PG事务/FK/迁移、会话持久化、服务间认证及错误契约、真实Redis限流；含核心失败/并发不变量 | 每个分支、一次性排障步骤、工单复现记录、UI逐按钮验证 |
| E2E | 浏览器通过真实Web/API/数据库完成少量完整核心流程，验证最终用户结果与访问边界 | 重复覆盖所有字段校验、排列组合、模板演示或Mock登录 |

“核心流程”包含有价值的拒绝与恢复路径，不仅成功路径；例如越权访问必须拒绝属于长期安全契约。只有实际组件边界才能暴露的问题，不能用Mock替代后仍声称集成已验证。

## 缺陷修复验证

- 临时排障脚本、浏览器操作记录、某次工单复现不直接加入长期集成/E2E套件。
- 回归优先放能稳定定位原因的最低层，命名描述长期行为，不以bug编号、修复日期或实现细节归类测试。
- 如果缺陷暴露了核心组件契约或用户旅程的缺口，补充/更新该契约或现有流程测试；不要为同一问题在多个层次各复制一套复现步骤。
- 一次验证通过不代表需要永久保存脚本；长期保留的用例必须说明其保护的核心行为，范围以需求验收为依据。

## 结构与工具

- Go单元测试随被测包放置，使用标准testing、清楚的子测试名和有价值的表驱动；集成测试采用`*_integration_test.go`和`integration`构建标签，显式选跑。
- 项目前端单元测试放`src/apps/web/tests/unit/`，使用Vitest验证导航、身份边界和国际化消息/组件；与生产页面分开维护。
- 平台浏览器E2E统一放`src/apps/web/tests/e2e/`，采用Playwright；角色、租户和会话按测试隔离。当前已接入真实IAM、Console和数据库，不Mock认证。
- 工具选型和测试数据都属于测试，不在生产逻辑增加绕过认证、强制管理员或跳过迁移的开关。

## 可执行入口

```sh
make test
make catalogs
make typecheck
make build
make integration ARGS="--docker --race"
make install-browsers
make test-e2e ARGS="--docker"
```

以上入口在macOS、Linux、PowerShell与`cmd.exe`中统一通过GNU Make执行；Makefile配方不依赖POSIX shell，平台差异由Node脚本承担。`make test`先由[scripts/test-scripts.mjs](../../scripts/test-scripts.mjs)枚举脚本测试，再运行Go与Web单测，不依赖shell通配符展开。

集成测试也支持显式提供`SPX_TEST_POSTGRES_URL`与`SPX_TEST_REDIS_URL`，不加`--docker`。CI 的 `Integration (ubuntu-latest)` job 用 `make integration ARGS="--docker"` 跑同一套用例：临时 `postgres:16-alpine` 与 `redis:7-alpine` 容器、随机端口与临时凭据，结束时按名称清理；宿主机另装 `postgresql-client-16`，让备份恢复用例的 `pg_dump` 与服务端同主版本。
外部Redis优先专用测试库；当前用户明确允许远程 DB0，测试仅清理随机测试账号/会话拥有的精确键，共享来源桶保留并按原 TTL 到期，禁止 FLUSH 或前缀清理。PG测试连接需要创建独立测试库的权限；测试只删除自己创建的`spx_test_<UUID>`数据库。
备份恢复用例使用与服务端兼容的 pg_dump/pg_restore；外部PG无需构建应用容器，测试仍创建独立随机数据库。

`--docker`明确授权测试脚本创建独立的临时PG/Redis容器、随机端口和临时凭据，结束后清理自有资源。
浏览器套件构建真实服务，启动两个IAM及两个Console实例，并通过根启动脚本启动第一个实例及Web。CI 的 `E2E (ubuntu-24.04)` job 用 `make test-e2e ARGS="--docker"` 跑同一套用例（9 个场景），并用 `TZ=Asia/Shanghai` 让浏览器不在 UTC，从而覆盖展示时区偏好。
浏览器环境先本地构建 Next.js，再通过 preview 入口启动并等待页面与 API 就绪；不连接正常开发库、不部署服务。它验证默认管理员初始化登录、新用户直接登录、主动改密、管理中心角色编辑、五种权限组合、跨标签退出、SSE撤权及越权访问拒绝。
不读取或改写开发数据库，也不使用开发登录凭据。

Playwright使用单worker、有界条件等待和零重试。禁用trace与视频，避免收集密码、Cookie；
必要截图写入被忽略的`test-results/`，不进入仓库。

依赖漏洞检查：

```sh
GOTOOLCHAIN=go1.27.4 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./src/...
```

当前执行本地分层检查和业务目录产物一致性检查；不开展部署或远程 CI，实际验收证据记录在 Plan。

## 安全与供应链检查

前三道闸门以退出码阻断且不依赖 GitHub Advanced Security；CodeQL 用 advanced setup 工作流实现，公开仓库免费，私有化后需要 GitHub Code Security。CI 的 `security` job 与
`test`/`quality` 并行，不在关键路径上。

| 闸门 | 工具与版本 | 覆盖 | 本地入口 |
| --- | --- | --- | --- |
| 工作流与 action 用法 | zizmor（官方 action，仅高危+高置信阻断） | 未固定 action、脚本注入、权限过宽、缓存投毒 | `uvx zizmor .`（需 uv） |
| 提交内容中的密钥 | gitleaks v8.28.0 | 工作树全量扫描（`.env.example` 等模板也在扫描范围） | `make security` |
| 依赖已知漏洞 | osv-scanner v2.7.1 | `go.mod` 与 `pnpm-lock.yaml` | `make security` |
| 源码语义分析（SAST） | CodeQL（`codeql.yml`，Go + TypeScript） | 注入、路径穿越、SSRF、硬编码凭据等可数据流追踪的缺陷 | 无法本地运行，仅在 CI |

**action 固定到 commit SHA。** 依据 [GitHub 安全加固指引](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions)
与 OpenSSF Scorecard 的 Pinned-Dependencies 检查：浮动标签（`@v4`）可被上游改写，SHA 不能。
升级由 Dependabot 的 `github-actions` 生态跟进，它会同时更新 SHA 与行尾的 `# vX.Y.Z` 注释。

**忽略与白名单的规则**（两个配置文件都在仓库根目录；被 dismiss 的告警登记在
[安全例外登记](security-exceptions.md)，无登记视为未处理）：

- `.gitleaks.toml` 只允许排除本地/构建产物（`node_modules`、`.next`、`logs`、`.spx`、`.env`）。
  禁止在 `allowlist.paths` 里加业务代码路径；误报用 `[[rules.allowlists]]` 精确到规则与正则。
- `osv-scanner.toml` 的每条 `[[IgnoredVulns]]` 必须写明 `reason` 与 `effectiveUntil`（不超过 6 个月），
  到期后 osv-scanner 会重新报出，强制复审。只允许忽略构建期依赖，运行期依赖必须修或升级。

## 环境和稳定性

真实集成与E2E使用显式测试连接、独立数据库/schema、账号、实例命名空间与端口；不能默认读开发/生产DSN后执行清理。测试启动可显式创建隔离容器，也可使用已提供测试环境，不要求所有开发者安装Docker。缺少集成依赖应明确失败或被明确标记为未执行，CI验收任务不能静默skip后变绿。

每个用例可独立运行、可重复，准备和回收只作用于本次拥有的数据。失败也执行受控清理，避免依赖顺序、固定账号或共享Cookie。依赖就绪和业务完成使用健康检查、条件断言与有界等待，不使用固定sleep或提高重试次数掩盖不稳定。

E2E优先语义定位（role/label等）和用户可见结果，避免内部CSS、DOM结构和截图尺寸耦合；业务旅程不Mock自己负责的认证或授权。外部第三方可在明确边界使用受控替身，报告说明真实验证到哪里。

失败保留必要诊断，密码/Cookie/API Key与个人数据脱敏；截图、trace和测试产物放gitignored目录。覆盖率是发现缺口的辅助指标，不通过镜像实现、只断言调用次数或重复测试抬高数字。

## 参考

[The Practical Test Pyramid](https://martinfowler.com/articles/practical-test-pyramid.html)讨论分层及组合；[微服务测试策略](https://martinfowler.com/articles/microservice-testing/)区分单元、组件边界与E2E；[Playwright最佳实践](https://playwright.dev/docs/best-practices)强调用户行为、隔离及稳定定位；[Go测试文档](https://go.dev/doc/tutorial/add-a-test)给出标准testing入口。本项目不机械套用某种比例或把所有回归归为E2E。

## Web 模块测试

前端测试和工具配置由 `src/apps/web/` 维护：`vitest.config.mts`、`playwright.config.ts`、`tests/unit/`、`tests/e2e/`。根命令只编排 Go/脚本检查并转发 Web 命令，不持有前端测试依赖。

构建与类型检查覆盖全部 Next.js 路由。E2E 使用真实 Cookie、CSRF、主动改密、角色权限、越权拒绝及 SSE 撤权。通过 `make test-e2e ARGS="--docker --grep <名称>"` 可定向运行。安装浏览器使用 `make install-browsers`。原作者站点统计、仓库星数和托管配置已移除；测试不依赖这些外部服务。

国际化回归还覆盖中英文词典键、ICU变量和复数、Dashboard真实组件的双语渲染。既有IAM浏览器用例在登录前显式选择中文，独立于默认语言；语言切换浏览器检查覆盖 Cookie 持久化、html lang、元信息和桌面菜单不换行。

## 本地框架检查与人工浏览器验收

```sh
make catalogs-generate
make catalogs
make test
make typecheck
make lint
make vet
make lint-go
make vuln-go
make build
make integration ARGS="--docker --race"
make test-e2e ARGS="--docker"
make test-e2e ARGS="--docker --manual"
```

`make sqlc-vet` 需要可连接的 PostgreSQL（`SPX_TEST_POSTGRES_URL`，允许建库）：它建一个临时库、应用迁移、把每条生成查询 PREPARE 一次后删库；CI 的 `Static checks` job 带 `postgres:16` 服务跑同一步。

`catalogs:check` 同时执行迁移边界检查（`scripts/migrations-check.mjs`）：每个模块的迁移只能引用自己 schema 内的对象，跨 schema `REFERENCES` 直接失败；其表驱动测试随 `make test`（`scripts/test-scripts.mjs`） 运行。

`lint-go`固定golangci-lint v2.15.0，`vuln-go`固定govulncheck v1.9.0，均使用项目Go 1.27.4工具链；纯本地命令，不触发部署或远程CI。漏洞扫描的“已调用符号”与仅模块/包引用的结果分别报告，不把工具运行失败当作零漏洞。

manual模式创建相同的独立双副本栈，在权限0600的临时文件中提供仅本次测试的URL/账号，保持运行等待标准输入结束，再精确清理本次资源。不要把此文件、凭据、截图或trace提交到仓库。浏览器操作按AGENTS.md委派到专用代理；桌面/窄屏、双语切换及刷新、主题各选项和CRM V1基准、核心管理流程及console/network都需实际检查，不能以自动化成功代替视觉验收。


## 可信入口与生命周期回归

全栈 E2E 直接启动 Next/IAM/Console。可以显式提供 `SPX_TEST_POSTGRES_URL`（允许创建临时库的管理连接）和 `SPX_TEST_REDIS_URL`，或显式 `--docker` 创建测试组件。远程 PG 只创建/删除本次随机临时库；Redis 只删除该库唯一账号/会话的精确键，不执行 FLUSH 或全平台前缀清理，也不删除共享来源桶。共用 DB 时来源桶仍共享计数并按原 TTL 到期，测试账号必须唯一。使用共享组件前需明确确认环境。备份恢复集成另需兼容版本的 pg_dump/pg_restore。

共享 HTTP 生命周期的真实监听测试覆盖后一个绑定失败释放前者、多个监听并行停止接收、默认及自定义 BaseContext 的活跃事件流观察退出取消，并保留自定义上下文值及无强杀退出；执行 `go test -race ./src/packages/go/httpx`。服务自身的资源仍由各 bootstrap 拥有，测试不停止已有进程。定向 Axios 边界测试使用真实本机 HTTP 响应覆盖迟到 401、错误密码及无效写入确认，不以手写 mock 实现代替网络/Axios 行为。


## 配置托管超级管理员

管理员 fixture 的用户名与密码由测试 IAM 配置注入，各副本一致。不得通过自助改密预热管理员会话：配置账号的改密、资料编辑、重置与角色分配必须被拒绝。普通账号继续验证改密成功及原会话失效。数据库集成覆盖管理员配置重复应用的幂等、凭据轮换/身份版本、账号切换和唯一索引；迁移到配置托管模式后必须应用管理员配置再验证服务可运行，不能把“版本已更新”当作初始化成功。

本轮人工设计验收只覆盖桌面浅色/深色与中英文，不开展移动端适配或新一轮窄屏验收。

设置页 E2E 验证 `/settings/profile`、`/settings/account` 为独立页面，头像 Settings/System 单行标签，设备管理入口移除，配置管理员凭据只读。数据表用真实接口验证默认 10 条、翻页偏移和修改每页数量后返回第一页。

用户与角色流程回归覆盖真实服务端搜索、10 条候选及滚动续页、禁用候选、Unicode 名称约束，以及同一弹框内错误密码/返回/确认后的草稿保留。测试辅助命令仅允许对 `spx_e2e_<随机值>` 所有者库中的指定测试账号缩短认证时间，不在生产加入过期开关，也不等待真实 30 分钟。确认成功后业务仍未写入，必须由用户再次提交；不读取浏览器原始 Network 事件、请求体、Cookie 或登录凭据。


## IAM 统一入口

IAM 真实 HTTP 用例在同一监听验证用户路由和 `POST /api/v1/iam/authorize`：仅用户 Cookie、错误服务凭证、超出白名单的 action 均不能获得服务授权；仅服务凭证不能访问用户身份接口。授权响应不能设置浏览器 Cookie，原用户的写请求仍校验 Origin/CSRF；旧 `/internal` 路由返回 404。

`internal/bootstrap/serve_integration_test.go` 使用远程组件的自有临时库验证HTTP 的真实启动/就绪/退出；HTTP 后端与 HTTPS 浏览器公开地址独立。标准客户端测试保留不可信证书拒绝，不增加应用 CA 配置。完整浏览器双副本栈默认 HTTP，不生成证书、不申请额外内部端口。


统一配置回归覆盖：同一根 IAM HOST/PORT 到达 Web 和 Console；连接位置不影响 BIND_HOST；公开 HTTPS 不改变 Web 本地监听端口；Web 缺少连接配置明确失败，不回退硬编码地址。根脚本测试验证环境隔离、示例后备构建且不创建私有文件；真实 Preview 使用官方 @next/env 和原生 CLI 启动 Web。

## 启动预检回归

`node --test scripts/preflight.test.mjs` 在临时工作副本运行真实 Go CLI：后续模块配置无效时，断言迁移尚未执行且没有业务日志。原生命令测试把依赖指向不可达本机端口，验证 `check-config` 不发起运行连接；字段诊断断言不包含输入秘密。`make config [MODULES="模块"]` 本身是离线校验，不等同于集成测试或网络就绪验收。
