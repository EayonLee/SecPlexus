# Gin 与全栈框架能力重构

状态：用户于 2026-10-29 批准实施；实现与本地分层验收完成，待 PR 审查。实际证据、保留边界见 [验收记录](acceptance.md)。

日期：2026-10-28。范围：IAM、Console、共享 Go HTTP/配置/命令能力、数据与迁移、限流、接口生成及 Web 请求/表单/列表工程基础。能力与官方源码依据见[框架审查](framework-capabilities.md)。

## 1. 需求与目标

以成熟框架承担通用机制，使项目代码聚焦业务规则。完整利用 Gin 的恢复、错误链、响应、Writer、路由、中间件和流能力，并深化 GORM/Goose、配置/命令、Session、Redis/限流及前端组件的使用。每项采用同时定义替代对象和验收，不以“引入一个库”作为完成。

本次允许重写实现、调整配置格式及内部类型，不建立历史兼容层。完整重构不等于改变已确认的产品规则、删除真实数据或绕过认证。涉及可观察协议变化时，同一次交付修改生产者、调用方、契约、测试和文档，不保留两套 HTTP 栈。

用户已确定：统一 Gin；全面研究框架原生/生态能力，允许为减少自制机制、优化架构与维护进行完整重构；参考 gin-vue-admin 的可取实现；保留 React/Next.js/shadcn 视觉基准；当前不建设部署和远程 CI。本 Spec 的具体组件组合与规则属于待审核方案，不代表已实施或用户已经逐项批准。

## 2. 当前事实与差距

| 方面 | 当前实现 | 本次处理 |
| --- | --- | --- |
| HTTP | IAM、Console 使用 Chi；`httpx` 提供绑定，IAM 以 kin-openapi 验证请求 | 改为 Gin 原生路由/Handler；通用输入处理由框架承担，消除重复解析和重复校验 |
| 认证 | Authboss、Argon2id、SCS、gorilla/csrf；独立的内部 TLS 入口 | 所有 HTTP 入口统一 Gin，移除 Authboss 编排与桥接；通用机制由 Argon2id/SCS/CSRF 组件承担，业务规则归 IAM |
| 授权 | 稳定权限目录、组织范围、多角色并集、委派及最近认证规则 | 继续由 IAM 裁决；不照搬基于 URL/方法的单角色权限模型 |
| 数据 | 已使用 GORM、pgx 共池和 Goose；部分 CRUD 使用 map，部分列表用 SQL 构造 JSON | 改进类型化模型、投影和批量操作，不把“已有 ORM”误写成“引入 ORM” |
| 配置 | `configfile` 自行展开 YAML 标量中的环境占位符；根脚本编排 Go/Web 环境 | 用成熟配置组件取代自制占位符机制，完整重定义优先级并同步根启动链路 |
| 契约 | OpenAPI 为权威源，目前主要生成 Go/TS 类型；已有路由、权限和错误码检查 | 深化为 Gin 接入、适用的 strict server、校验、SDK/Zod/Query options 生成，减少手写结构与调用 |
| Web | Next.js/shadcn/next-intl，已有 Zod/Zustand/Table；IAM 列表请求和表单状态主要手写 | 采用 Query、RHF 和生成能力替代机制，深化现有组件；保留页面、样式、主题、字体基准 |
| 治理 | 已有 AGENTS、项目技能、模块 README、Spec/Plan 和测试指南 | 原位更新并以真实代码作范例，不建立平行规则库 |

事实入口：[IAM](../../../src/apps/iam/README.md)、[Console](../../../src/apps/console/README.md)、[契约](../../../src/contracts/README.md)、[配置](../../configuration.md)、[测试](../../testing.md)。这些文档描述现状；本草案未批准、未实施前，不改变其权威约定。

## 3. 范围与不做事项

本次交付覆盖以下十四项要求；不拆为不同批次或留下长期过渡栈。

| 编号 | 必须交付的结果 |
| --- | --- |
| R1 | IAM/Console 所有 HTTP 入口统一 Gin，移除 Chi、Authboss 及其专用路由/认证适配 |
| R2 | 使用 Gin 原生 Recovery、错误链、响应/Writer、分组与流能力；成熟绑定/校验统一公开错误 |
| R3 | 深入使用 GORM 泛型/事务/关联/批量/错误/日志及 Goose 版本能力，保留有业务理由的 SQL |
| R4 | OpenAPI 单一真源生成 Go/Gin 接入及 Web SDK/Zod/Query options，工具链通过能力验证 |
| R5 | 实际路由、契约、权限标识和保护边界可检查 |
| R6 | 用 Viper 接管 Go 配置加载；根环境、模块覆盖、Web 环境边界完整闭环 |
| R7 | 用现有组织角色管理链路形成可运行的前后端业务开发范例 |
| R8 | Console/IAM 的职责和新增模块接入规则明确且与实现一致 |
| R9 | 本地生成、检查、测试和构建入口可执行，生成结果可复现 |
| R10 | 规则、真实范例、模块文档和计划状态同步维护 |
| R11 | 新增依赖及复制内容有版本、来源、许可和维护说明 |
| R12 | 成熟 Redis limiter 取代自写 Lua，连接池、超时、重试和 key 归属明确 |
| R13 | Cobra/Viper 与标准生命周期能力取代手工命令分流和重复启动机制 |
| R14 | Web 使用生成 SDK、Query、RHF/Zod 和已有 Table/next-intl，删除对应手写机制并保持视觉基准 |

不包含：新增案件/字典/导入导出/文件服务等产品；Vue 前端；可视化代码生成平台、运行时写源码/AST 注入；通用 BaseRepository、全局依赖容器、插件市场；新增调度服务；更换 SCS 会话组件、密码算法或引入 Casbin；无关依赖升级、全站视觉改造；容器镜像构建、生产部署、远程 CI 或 Git 提交推送。

现有 `maintain` 命令保留并验证配置接入；部署调度不作为本次缺口。未使用的 GVA 功能、数据库驱动、云 SDK 和管理页面不导入。

## 4. 服务边界与组织方式

```text
浏览器 → Next 同源公开入口 → /api/v1/iam/*     → IAM
                         → /api/v1/console/* → Console
Console → 认证且隔离的 /internal/v1/iam/* → IAM 授权裁决
```

- IAM 拥有账号、凭证、会话、组织、成员、角色、权限、授权决策和相关审计。
- Console 拥有安全运营业务及有实际需要的运营查询聚合，不是所有模块的网关。不得为 IAM 现有接口再写一套转发 Handler、DTO 和校验。
- Console 当前没有自有持久化业务，不增加 DB/Redis 配置或空仓储；不访问 IAM 私表或私有包。跨域调用不形成隐式分布式事务。
- Web 同源入口只路由公开模块 API，保留 Cookie、Origin、CSRF、状态、Set-Cookie 和流；不暴露内部入口、内部凭证或浏览器可指定的任意代理目标。
- `cmd` 使用 Cobra；`config` 使用 Viper/mapstructure 加载并校验；`bootstrap` 显式组装依赖及生命周期；`router` 注册生成/原生 Gin 路由和分组保护；`handler` 实现类型化接口边界；`service` 业务规则和事务；`repository` 数据访问。
- Gin Context 留在 HTTP 边界，向内显式传递标准 `context.Context` 与类型化值。共享包只放 IAM/Console 实际共用能力，不按表机械创建接口和层次。

## 5. 技术方案与约束

### 5.1 Gin 与成熟认证组件

所有 HTTP 入口采用 `gin.New()` 和原生 Handler，包括登录、二次认证、管理接口、健康检查、Console SSE 和内部授权；按无需会话、需要会话及内部服务入口组织路由。IAM 公开和内部入口使用独立 Engine/监听，内部 TLS、服务凭证与调用方 action 校验继续有效。统一处理 NoRoute/NoMethod、真实状态及必要的 Allow 头；明确 HEAD、OPTIONS、路径参数和斜杠重定向行为，不能因框架默认值改变写请求或绕过保护。

Gin 负责 HTTP，不承担密码算法、Session 存储或组织授权。选定组合如下，不把“统一 Gin”误解为只依赖一个库：

| 职责 | 采用方式 | 项目只维护的内容 |
| --- | --- | --- |
| 路由、绑定、协议校验 | Gin + oapi-codegen/Gin 官方校验组件；非协议结构使用 validator | 一份协议和真正的业务规则、公开错误映射 |
| 密码哈希、随机盐及密码比较 | 既有 `alexedwards/argon2id` | 密码政策、PHC 参数资源上限；不自行实现密码学 |
| 服务端 Session、随机令牌、Cookie、令牌轮换与持久化 | 既有 SCS + pgxstore | 账号版本、立即撤销、逻辑有效期、最近认证及提交失败处理 |
| CSRF Cookie 与令牌校验 | 既有 gorilla/csrf | 可信 Origin 和同源部署规则 |
| 账号状态、失败锁定、审计、组织授权 | IAM 用例 + GORM 事务 | 这些是平台业务规则，不能依赖路由库推断 |

移除 Authboss 的依据是当前实际收益和适配成本，而非历史兼容：目前仅 `Init("auth")` 启用密码登录；密码算法、锁定审计、Session 创建和授权均由外部组件或 IAM 实现，另需自定义 BodyReader、Responder、Renderer、用户存储接口、事件回调和 SCS 会话桥。通用账号 Save 还被显式拒绝，业务写入必须经过 IAM 事务。保留 Authboss 再适配 Gin 会延续这些平行抽象；本次选择删除该编排层，不为尚未使用的注册/找回密码等模块保留依赖。

HTTP 边界通过生成的 Gin 绑定或少数原生 Handler 绑定一次，将类型化凭证交给具名的登录/二次认证用例；用例返回身份或业务错误，不接收 Gin Context、Request/ResponseWriter，也不输出响应。采用 strict server 的接口仍注册在 Gin，而非另建入口。二次认证不重新编码成登录 JSON 后转发。删除 Authboss 读取器、响应器、回调和会话桥，保留 SCS 提交失败补偿真正需要的请求状态。

SCS/gorilla/csrf 的 net/http 组件允许通过窄的中间件组合接入 Gin；这不是第二个 HTTP 入口，也不允许保存旧 Handler 栈。不得把旧业务 Handler 全部包进 `WrapH`，或为了“全是 Gin”而自行实现 Session/CSRF。

共享 request_id、slog 访问日志、异常恢复和 Result 原位收敛。Recovery 使用 Gin 原生捕获及受控回调，并关闭其默认请求/异常转储输出；不继续手写通用 recover。业务错误经 `c.Error/c.Errors` 和统一映射处理，Abort 后必须 return；不得原样返回 `c.Errors.JSON/String`。访问日志使用 FullPath、Writer.Status/Size/Written，删除路由预匹配和通用 Writer 包装。客户端地址采用显式配置的 Gin 可信代理能力，默认不信任任意代理，不保留手写并行 IP 链。

IAM 公开链路必须满足：受控诊断边界 → SCS 加载 → Origin/CSRF → 输入绑定/校验 → 用例 → SCS 提交 → 最终响应。内部入口在执行授权前验证调用身份；就绪检查不被误绑为登录接口。具体中间件排列与异常清理由 Plan 的组件验证确认。

登录与二次认证另保留既有的 2 KiB 凭证体限制、IP/账号限流、密码计算并发上限、未知账号替代哈希校验、失败锁定和审计；一般 JSON 的 32 KiB 上限不能覆盖此更严格限制。绑定失败也不能绕过来源限流；Redis/数据库不可用返回依赖故障，不伪装成密码错误。密码计算后，创建会话事务仍检查最新账号/凭证版本及状态，避免把旧密码快照变成新会话。重新认证不延长原会话绝对期限。

### 5.2 绑定、校验和响应

- 优先使用官方生成的 Gin 绑定/strict server 与 Gin OpenAPI middleware；少数原生入口用 `ShouldBind*`，避免 MustBind 提前响应。validator 采用内建字段/集合/struct-level 能力处理其实际拥有的非协议规则，不与 HTTP schema 维护重复字段约束。
- 未知输入只在拥有它的边界解析、验证一次。验证后的值交给成熟生成绑定能力，不手写结构反射/JSON 回放。D1 必须验证固定版本组合对验证值复用和类型转换的支持。
- JSON 接口保持 32 KiB 上限、明确 Content-Type、未知字段和单 JSON 值规则；绑定器默认不等于已满足这些约束，必须实测。空体、尾随 JSON、错误类型、无效 UUID、整数溢出和超限分别映射已登记错误码。
- PATCH 明确区分未提供、null、空字符串、零和 false；请求对象、领域值、持久化对象和输出投影按边界区分，禁止批量绑定数据库模型。
- 列表继续采用 `limit`/`offset`，默认 50/0、limit 1–100、offset 0–100000；用绑定器和类型化规则替代手写解析，不因模板使用 Page/PageSize 改动现有产品。排序只有已有实际需求才实现，并使用列白名单和稳定次序。
- 普通响应维持 `code/message/data/request_id` 和真实 HTTP 状态；使用 Gin 渲染或生成 response visitor，项目仅维护薄的业务信封/错误码映射。禁止捕获已编码响应后再统一包装。204 无正文；SSE 用原生流/Flush，不经过普通响应缓冲。错误响应/日志不包含校验器原始值、SQL 或内部错误。
- 错误码与权限目录仍是独立业务注册表，不因接口真源调整而复制；Web 根据稳定 code 使用现有 next-intl 词典显示信息。

### 5.3 GORM 使用与数据安全

采用 GORM/pgx 共池、Goose 和模块 schema 所有权。普通 CRUD 优先 `gorm.G[T]` 泛型、显式字段更新、类型化投影、Scopes、条件关联和批量操作；适用事务使用 `Transaction` 回调处理提交/回滚，删除重复事务收尾。锁、原子 SQL 或复杂投影可在局部采用具体的 GORM/SQL API，不为统一语法增加一层泛型仓储。

关联读写须明确复合组织键、目标范围和查询数量。`Preload`、Association API、`Save` 或自动关联不能替代授权、委派、版本条件及最后管理员检查。修改只更新允许字段，正确处理零值/null，检查 RowsAffected；审计与业务修改使用同一个事务对象，不悄悄回到根连接。

锁、原子授权查询、数据库约束、审计写入及复杂投影允许集中在仓储的参数化 SQL。采用批量操作前核对现有触发器/约束和返回语义，不以删除安全检查或测试换取代码减少。不得启用 AutoMigrate、重写已执行迁移、清空数据库或默认引入软删除；确需 schema 变化时增加可审查的版本化迁移。

采用 GORM 原生错误分类/PG 驱动翻译及 logger 扩展，集中保留 PgError 错误链与约束语义；不复制内部错误到公开响应。慢查询、参数脱敏和 context 关联按官方能力配置，项目只补安全日志策略。Goose Provider 负责显式升级、锁与状态，替代硬编码 schema 版本；状态检查先确认版本表已存在，普通 serve 不因调用 Provider 查询而隐式建表。池、迁移和日志的能力边界见[审查表](framework-capabilities.md#3-其他后端组件利用能力与边界)。

### 5.4 接口定义与生成：选定协议优先的工具组合

选择 OpenAPI 3.x 为唯一协议真源，深化成熟工具而非建设代码优先生成器。这不是为了兼容当前实现：oapi-codegen 的 Gin 接入/strict server、官方 Gin 校验组件，以及 Hey API 的 SDK/Zod/Query 生成可以直接组合，减少手写 DTO、绑定、校验和调用代码。GVA 的 Swagger 2 注释链不作为目标，也不叠加另一套 API 框架。

协议源继续归 `src/contracts`，公共错误码/权限目录保持独立业务所有权。输出包括共享 Go 类型、模块 Gin 接入和适用的类型化请求/响应、真实需要的内部客户端，以及 Web TS/Fetch SDK/Zod/Query options。模块接入生成代码归模块 HTTP 边界；共享类型/客户端不导入应用 internal，使用官方 import-mapping 等能力解决包依赖，不能手写第二份 DTO。

生成的 strict server 不等于完整 schema 校验；字段结构由官方 Gin middleware/kin-openapi 负责，并向生成绑定器交付验证值。SSE/健康检查等原生操作明确生成排除和挂载边界，仍由 Gin 注册并参加清单对照。SDK 和表单使用派生 Zod，不另手写所有协议字段；业务用例仍检查授权/状态/事务。

公开/内部操作分开契约入口和生成配置，共享 schema 用引用且每个操作只有一个源。Web 仅生成公开 API 客户端和类型；内部授权/服务凭证方案不进入浏览器产物，内部 Go 客户端消费内部契约。允许调整现有协议源拆分，不能用重复手写 DTO 代替隔离。

**D1 是工具组合的验证门槛**：固定兼容版本，证明 nullable/optional/PATCH、组合类型、字符串版本号、Result、Cookie/CSRF/内部凭证、204、SSE、权限元数据、生成包边界和 Go/TS 消费均正确。新能力未实际验证前不能写成已交付。不得手改生成物、自制 schema 编译器或增加双向生成；能力不满足时带证据修订方案，不恢复重复手写结构。更详细的取舍与官方入口见[契约与 Web 审查](framework-capabilities.md#4-契约与-web一套输入驱动工具)。

### 5.5 路由、权限与安全不变量

现有路由对照测试改用 `Engine.Routes()`；规范化 `:id` 与 OpenAPI `{id}`，明确健康检查和框架路由的排除范围。检查方法/路径、operationId、权限元数据、目录引用及公开/内部入口划分。

路由存在且声明 action 不等于执行了授权。另用真实 HTTP/组件测试验证匿名拒绝、跨组织拒绝、内部端口隔离和未授权 action；不能把路由清单测试作为访问控制证明。保持多角色并集、无角色即无业务权限、即时撤权、最近认证和最后有效管理员保护。IAM 故障仍为 503，不得被 Web 当成退出成功。

### 5.6 Viper 与环境配置

Viper 使用模块局部实例，配置加载阶段一次解码到不可变的类型化配置；显式 `BindEnv`、允许有意义的空环境值、严格解码及最终校验。不使用全局配置对象、运行时 GetEnv、WatchConfig 或静默热更新依赖。

严格解码使用 UnmarshalExact 和明确的 mapstructure DecoderConfig/DecodeHook；默认弱类型转换不直接作为规则。duration、列表、CIDR 和调用方集合使用组件扩展能力，删除自制通用解析/校验引擎，不引入实验性隐式绑定。Cobra 的子命令、参数检查和 context 承担命令分流，配置选择在网络依赖创建前完成。

采用以下明确规则，替换 `${ENV:default}` 方言，不同时保留两种加载器：

| 场景 | 目标优先级/行为 |
| --- | --- |
| 根入口环境 | 进程环境 > 根 `.env`；不覆盖用户文件 |
| Go 配置字段 | 明确绑定的模块专用环境 > 适用的共享环境 > 模块 YAML 字面值 > 组件注册默认值 |
| 共享 PG/Redis | 继续使用 `SPX_POSTGRES_*`、`SPX_REDIS_*`；IAM 独立连接用 `SPX_IAM_POSTGRES_*`、`SPX_IAM_REDIS_*`，只映射真实字段 |
| 模块 YAML 覆盖 | 全局环境存在时不靠修改 YAML 抢优先级；使用模块专用环境覆盖，并在配置指南写明这一变化 |
| 完整 URL | 解析出非空 URL 时整体优先于分字段；共享 URL 存在时，仅改 host 不构成覆盖，须显式清除 URL 或设置模块 URL |
| 空值 | 显式空值不自动回退；可选 URL/日志文件可清空，必需凭证等由模块校验明确拒绝 |
| 类型 | duration、整数、布尔、可信 CIDR、内部调用方集合必须按显式解码规则处理；不依赖弱类型转换碰巧成功 |
| 加载时点 | 启动及 `--print-endpoints` 使用同一加载器；加载失败不得先打开网络连接或执行迁移 |

`--config` 只选择配置文件，不扩展为所有字段的动态 CLI 配置系统。绑定名不机械映射整个结构，不把未知 `SPX_*` 当作有效配置。内部调用方凭证如何绑定到类型化集合必须通过 D2 验证；不创建新的环境表达式语言。

根启动、构建、preview、独立模块、迁移和维护使用一致规则。按真实加载结果推导地址；Web 继续使用 Next 原生环境，根入口只注入已有允许的 Web/公开配置，保留构建期和运行期区别。模块级 PG/Redis 覆盖必须穿过根环境过滤器，不能只在 Go 单独运行时生效。

配置格式变化不自动改写用户 `.env` 或连接数据；提供字段迁移对照和公开示例。现有部署配置如引用被替代语法，仅同步必要的静态配置以免留下第二套规则，不开展镜像/编排重构、构建或部署。

### 5.7 范例与长期维护

选择现有组织角色管理为唯一端到端参考：角色列表、创建/编辑、权限配置和影响查看；引用其真实迁移、类型、路由、用例、仓储、生成类型、Web 页面和测试。它不是演示模块，也不新增无关业务；可重复验证并说明权限/事务边界。

在现有项目技能和文档中说明新增能力改哪些位置、哪些由工具生成、哪些规则必须业务实现。没有真实重复需求前不建设模板引擎；示例通过引用实际代码保持同步，不复制一套会失真的示例源码。

依赖和生成工具使用显式版本，记录直接用途和升级验证命令；不混入无关升级。复制第三方代码时记录上游仓库、提交、文件、本地修改及许可证，保留必要署名/许可；仅参考思想与实际复制分开记录。成对贡献指南同步更新；缺陷/安全报告沿用已有渠道，不凭空编造邮箱或开启外部功能。

### 5.8 Redis、限流、命令和生命周期

使用 `ulule/limiter/v3` 的 Redis 原子 Store 和 Gin 扩展，删除自写 Lua/计数算法；IP/账号的固定窗口政策保留，库负责计数/TTL，项目只提供 key 和业务拒绝策略。统一处理 429、Retry-After 及依赖故障 503；禁用内存回退和可能重复计数的盲目重试。最终 key 仍由集中定义组合为 `spx:iam:...`，正确处理库自带前缀，不要求兼容旧内部 key API。

Redis/pgx 的池、超时、取消、健康检查及关闭由组件承担，限制和所有权显式；不新建重复池或通用重试框架。Cobra 组织 serve/migrate/status/maintain/revoke-after-restore/reset-password 及端点输出，删除手工首参数分流和 FlagSet 分支。端点输出和帮助无网络副作用，错误退出不打印秘密。标准 `http.Server`、signal context、errors.Join 和实际需要的 errgroup 处理多监听生命周期；不引入 DI 容器或任务平台。

### 5.9 Web 工程能力

从同一契约生成 SDK、Zod v4 和 Query options，采用 TanStack Query 管理服务端查询、分页、取消、Mutation 和失效刷新；采用 RHF/Zod resolver 管理表单值、校验、提交和字段错误；已有 Table 承担列表行模型/受控分页，next-intl 承担文本与格式。删除自制列表请求状态、字符串式通用表单 DSL 和重复协议解码，不覆盖上游 UI/样式文件。

Query 的默认重试、staleTime、focus/reconnect 和缓存生命周期必须显式设置。身份代次、组织及授权版本参与缓存作用域；退出/撤权/切换立即取消并清除旧结果，保留必要的 epoch 和跨标签业务事件，不能由“框架会处理竞态”替代。密码等凭证不进入生成 mutation key、持久化缓存、Devtools 或日志；敏感操作直接用 SDK + 表单状态，不进入 Query MutationCache，完成后清理临时状态。SDK 保持同源凭证、CSRF、AbortSignal 和 401/403/503 语义，不自动重试写操作。

Zustand 保留内存身份及真正的 UI 状态，不重复缓存 Query 的服务端资源；使用 selector 限制订阅。Next.js 使用已安装版本的错误/加载/未找到边界和服务端配置隔离，保留 SSE Route Handler 及身份数据不共享缓存。此次是项目逻辑机制重构，不新增模板页面、不调整 CRM V1/字体/主题/共享控件基准。

## 6. 验收标准

| 编号 | 可验证结果 |
| --- | --- |
| A1 | IAM/Console 所有生产路由由 Gin 注册；生产代码和直接依赖没有 Chi/Authboss；旧 HTTP 栈、Authboss 读取/响应/事件/会话桥被移除 |
| A2 | 路径、查询、JSON 边界及错误响应符合 5.2；不存在重复 schema/struct 规则真源 |
| A3 | 登录/二次认证由 Gin 单次绑定，凭证不经内部 HTTP/JSON 回放；限流/哈希并发限制/失败锁定有效；改密、退出、撤销和跨副本会话正常，SCS 保存失败不能响应认证成功 |
| A4 | 跨组织/委派/多角色/最后管理员保护有效；撤权后 Console SSE 停止，IAM 故障不放行、不伪装退出 |
| A5 | 普通 CRUD/列表使用类型化 GORM；零值/空值、版本冲突、批量关联、事务回滚和审计原子性有真实 PG 验证 |
| A6 | D1 版本组合证据齐全；单一契约生成 Gin/Go/SDK/Zod/Query 产物可重复，消费方编译通过，路由/权限/结果码检查有效 |
| A7 | D2 验证通过；根环境、模块覆盖、内部凭证集合和 Web 过滤与表中规则一致；旧占位解析器移除 |
| A8 | Web 登录/管理/组织切换/中英文及流式链路通过必要验收；CRM V1、主题和共享 UI 基准未被改造 |
| A9 | 角色管理范例来自实际实现；文档、技能、配置示例和生成命令一致；删除旧机制及其无用依赖/测试 |
| A10 | 本地检查证据、失败原因和未运行项逐项可追踪；无凭证/个人路径、无未授权 Git 交付、无部署或远程 CI 执行 |
| A11 | 成熟 limiter 替代自写 Lua，固定窗口/原子性/TTL/key/429/503 通过真实 Redis 验证；Goose 无版本常量，serve 无隐式 DDL；命令与资源生命周期正确 |
| A12 | Query/RHF/SDK/Zod/Table 真正承接原有机制，缓存隔离、取消/失效、敏感 mutation、类型化表单与双语字段错误有验证；没有重复请求/表单引擎 |

完整交付需 A1–A12 全部满足，且框架审查表中的“采用”项有实际落点和替代删除证据；缺少真实组件或浏览器验证时如实记录未完成项，不以类型检查、Mock 或历史报告替代。

## 7. 参考依据与适用边界

- [gin-vue-admin 固定参考版本](https://github.com/flipped-aurora/gin-vue-admin/tree/cee04b08d1262036ca33e3026e1e13d29471c7d5)：参考模块接入、Gin 组织、GORM 用法和可运行开发范例；不移植 Vue、全局对象、安全模型或生成平台。
- [Gin 绑定文档](https://gin-gonic.com/en/docs/binding/)与[中间件文档](https://gin-gonic.com/en/docs/middleware/)：验证原生能力，不将文档中的简化错误返回作为项目实现。
- [validator](https://github.com/go-playground/validator)、[Viper](https://github.com/spf13/viper)、[GORM 关联](https://gorm.io/docs/associations.html)：固定实际采用版本后验证项目语义，不把“库支持”写成“组合已经通过”。
- [Authboss 当前依赖版本的登录实现](https://github.com/aarondl/authboss/blob/v3.5.3/auth/auth.go)、[SCS](https://github.com/alexedwards/scs)、[Argon2id](https://github.com/alexedwards/argon2id)：结合本项目真实调用比较认证职责；本次移除 Authboss 编排，保留成熟会话与密码组件。
- 本计划承接[平台服务重构](../platform-service-architecture/spec.md)、[结构化配置](../structured-module-configuration/spec.md)、[工作台与服务边界](../web-workbench/spec.md)；历史计划保留事实，实施完成后补充新方案入口，不重写历史验收记录。
