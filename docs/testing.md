# 测试分层与用例维护

## 原则

按可观察行为和最小必要边界选择测试层次，越高层越少、越聚焦，不规定机械比例。单元测试覆盖规则和边界；集成测试覆盖真实组件契约；E2E覆盖关键用户旅程。一个断言能在低层可靠验证，就不在每层重复穷举。

| 层次 | 覆盖内容 | 不应承担 |
|---|---|---|
| 单元/组件测试 | 纯规则、状态转换、输入边界、配置优先级、界面交互；外部I/O隔离 | 真实数据库约束或整个系统是否可用的证明 |
| 集成测试 | PG事务/FK/迁移、会话持久化、服务间认证及错误契约、真实Redis限流；含核心失败/并发不变量 | 每个分支、一次性排障步骤、工单复现记录、UI逐按钮验证 |
| E2E | 浏览器通过真实Web/API/数据库完成少量完整核心流程，验证最终用户结果与访问边界 | 重复覆盖所有字段校验、排列组合、模板演示或Mock登录 |

“核心流程”包含有价值的拒绝与恢复路径，不仅成功路径；例如跨组织访问必须拒绝属于长期安全契约。只有实际组件边界才能暴露的问题，不能用Mock替代后仍声称集成已验证。

## 缺陷修复验证

- 临时排障脚本、浏览器操作记录、某次工单复现不直接加入长期集成/E2E套件。
- 回归优先放能稳定定位原因的最低层，命名描述长期行为，不以bug编号、修复日期或实现细节组织测试。
- 如果缺陷暴露了核心组件契约或用户旅程的缺口，补充/更新该契约或现有流程测试；不要为同一问题在多个层次各复制一套复现步骤。
- 一次验证通过不代表需要永久保存脚本；长期保留的用例必须说明其保护的核心行为，范围以需求验收为依据。

## 结构与工具

- Go单元测试随被测包放置，使用标准testing、清楚的子测试名和有价值的表驱动；集成测试采用`*_integration_test.go`和`integration`构建标签，显式选跑。
- 项目前端单元测试放`src/apps/web/tests/unit/`，使用Vitest验证导航、身份边界和国际化消息/组件；与生产页面分开维护。
- 平台浏览器E2E统一放`src/apps/web/tests/e2e/`，采用Playwright；角色、租户和会话按测试隔离。当前已接入真实IAM、Console和数据库，不Mock认证。
- 工具选型和测试数据都属于测试，不在生产逻辑增加绕过认证、强制管理员或跳过迁移的开关。

## 可执行入口

```sh
pnpm test:unit
pnpm catalogs:check
pnpm typecheck
make build
pnpm test:integration --docker --race
pnpm --dir src/apps/web exec playwright install chromium
pnpm test:e2e --docker
```

集成测试也支持显式提供`SPX_TEST_POSTGRES_URL`与`SPX_TEST_REDIS_URL`，不加`--docker`。
外部Redis优先专用测试库；当前用户明确允许远程 DB0，测试仅清理随机测试账号/会话拥有的精确键，共享来源桶保留并按原 TTL 到期，禁止 FLUSH 或前缀清理。PG测试连接需要创建独立测试库的权限；测试只删除自己创建的`spx_test_<UUID>`数据库。
备份恢复用例使用与服务端兼容的 pg_dump/pg_restore；外部PG无需构建应用容器，测试仍创建独立随机数据库。

`--docker`明确授权测试脚本创建独立的临时PG/Redis容器、随机端口和临时凭据，结束后清理自有资源。
浏览器套件构建真实服务，启动两个IAM及两个Console实例，并通过根启动脚本启动第一个实例及Web。
浏览器环境先本地构建 Next.js，再通过 preview 入口启动并等待页面与 API 就绪；不连接正常开发库、不部署服务。它验证默认管理员初始化登录、新用户直接登录、主动改密、管理中心角色编辑、五种权限组合、跨标签退出、SSE撤权及组织上下文隔离。
不读取或改写开发数据库，也不使用开发登录凭据。

Playwright使用单worker、有界条件等待和零重试。禁用trace与视频，避免收集密码、Cookie；
必要截图写入被忽略的`test-results/`，不进入仓库。

依赖漏洞检查：

```sh
GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./src/...
```

当前执行本地分层检查和业务目录产物一致性检查；不开展部署或远程 CI，实际验收证据记录在 Plan。

## 环境和稳定性

真实集成与E2E使用显式测试连接、独立数据库/schema、账号、实例命名空间与端口；不能默认读开发/生产DSN后执行清理。测试启动可显式创建隔离容器，也可使用已提供测试环境，不要求所有开发者安装Docker。缺少集成依赖应明确失败或被明确标记为未执行，CI验收任务不能静默skip后变绿。

每个用例可独立运行、可重复，准备和回收只作用于本次拥有的数据。失败也执行受控清理，避免依赖顺序、固定账号或共享Cookie。依赖就绪和业务完成使用健康检查、条件断言与有界等待，不使用固定sleep或提高重试次数掩盖不稳定。

E2E优先语义定位（role/label等）和用户可见结果，避免内部CSS、DOM结构和截图尺寸耦合；业务旅程不Mock自己负责的认证或授权。外部第三方可在明确边界使用受控替身，报告说明真实验证到哪里。

失败保留必要诊断，密码/Cookie/API Key与个人数据脱敏；截图、trace和测试产物放gitignored目录。覆盖率是发现缺口的辅助指标，不通过镜像实现、只断言调用次数或重复测试抬高数字。

## 参考

[The Practical Test Pyramid](https://martinfowler.com/articles/practical-test-pyramid.html)讨论分层及组合；[微服务测试策略](https://martinfowler.com/articles/microservice-testing/)区分单元、组件边界与E2E；[Playwright最佳实践](https://playwright.dev/docs/best-practices)强调用户行为、隔离及稳定定位；[Go测试文档](https://go.dev/doc/tutorial/add-a-test)给出标准testing入口。本项目不机械套用某种比例或把所有回归归为E2E。

## Web 模块测试

前端测试和工具配置由 `src/apps/web/` 维护：`vitest.config.mts`、`playwright.config.ts`、`tests/unit/`、`tests/e2e/`。根命令只编排 Go/脚本检查并转发 Web 命令，不持有前端测试依赖。

构建与类型检查覆盖全部 Next.js 路由。E2E 使用真实 Cookie、CSRF、主动改密、角色权限、组织隔离及 SSE 撤权。通过 `pnpm test:e2e --docker --grep <名称>` 可定向运行。安装浏览器使用 `pnpm --dir src/apps/web exec playwright install chromium`。原作者站点统计、仓库星数和托管配置已移除；测试不依赖这些外部服务。

国际化回归还覆盖中英文词典键、ICU变量和复数、Dashboard真实组件的双语渲染。既有IAM浏览器用例在登录前显式选择中文，独立于默认语言；语言切换浏览器检查覆盖 Cookie 持久化、html lang、元信息和窄屏显示。

## 本地框架检查与人工浏览器验收

```sh
pnpm catalogs:generate
pnpm catalogs:check
pnpm test:unit
pnpm typecheck
pnpm lint
go vet ./src/...
make lint-go
make vuln-go
make build
pnpm test:integration --docker --race
pnpm test:e2e --docker
pnpm test:e2e --docker --manual
```

`lint-go`固定golangci-lint v2.15.0，`vuln-go`固定govulncheck v1.9.0，均使用项目Go 1.27.3工具链；纯本地命令，不触发部署或远程CI。漏洞扫描的“已调用符号”与仅模块/包引用的结果分别报告，不把工具运行失败当作零漏洞。

manual模式创建相同的独立双副本栈，在权限0600的临时文件中提供仅本次测试的URL/账号，保持运行等待标准输入结束，再精确清理本次资源。不要把此文件、凭据、截图或trace提交到仓库。浏览器操作按AGENTS.md委派到专用代理；桌面/窄屏、双语切换及刷新、主题各选项和CRM V1基准、核心管理流程及console/network都需实际检查，不能以自动化成功代替视觉验收。


## 可信入口与生命周期回归

全栈 E2E 直接启动 Next/IAM/Console。可以显式提供 `SPX_TEST_POSTGRES_URL`（允许创建临时库的管理连接）和 `SPX_TEST_REDIS_URL`，或显式 `--docker` 创建测试组件。远程 PG 只创建/删除本次随机临时库；Redis 只删除该库唯一账号/会话的精确键，不执行 FLUSH 或全平台前缀清理，也不删除共享来源桶。共用 DB 时来源桶仍共享计数并按原 TTL 到期，测试账号必须唯一。使用共享组件前需明确确认环境。备份恢复集成另需兼容版本的 pg_dump/pg_restore。

共享 HTTP 生命周期的真实监听测试覆盖后一个绑定失败释放前者、多个监听并行停止接收、默认及自定义 BaseContext 的活跃事件流观察退出取消，并保留自定义上下文值及无强杀退出；执行 `go test -race ./src/packages/go/httpx`。服务自身的资源仍由各 bootstrap 拥有，测试不停止已有进程。定向 Axios 边界测试使用真实本机 HTTP 响应覆盖迟到 401、错误密码及无效写入确认，不以手写 mock 实现代替网络/Axios 行为。
