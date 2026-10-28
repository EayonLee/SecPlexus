# 框架能力审查与采用决策

设计核对：2026-10-28；实施：2026-10-29。状态：用户已批准，框架接入已实现，最终验收记录见[实施证据](acceptance.md)。下表保留选型依据，最终采用范围及必要窄适配以下方实现记录为准。实施范围与验收以 [Spec](spec.md)、任务以 [Plan](plan.md) 为准。

目标不是增加依赖数量或启用全部选项，而是让成熟组件承担真实通用机制，删除对应自制实现。每项采用必须同时指出能力入口、替代对象、项目仍需维护的业务策略和验证方法。现有实现不是必须保留的兼容目标；数据、权限和产品约束仍须明确。

## 1. 组合决策

```text
OpenAPI 模块契约 + 公共错误码/权限目录
  ├─ oapi-codegen → Go 类型、Gin 接入、适用的 strict server、内部 Go 客户端
  └─ Hey API → TypeScript、Fetch SDK、Zod、TanStack Query options

Gin → Recovery / 错误收集与响应 / 路由分组 / 协议边界
  ├─ IAM 凭证用例 → Argon2id + SCS + gorilla/csrf
  ├─ ulule/limiter → go-redis 原子计数与过期
  └─ IAM 事务用例 → GORM / pgx；Goose 负责显式迁移

Cobra/pflag → Viper/mapstructure → 类型化配置 → 显式依赖组装
Next + shadcn → 生成 SDK / TanStack Query / React Hook Form + Zod
Zustand 保留内存身份状态；next-intl 保留双语和格式能力
```

后端使用 Gin 一个 HTTP 栈；OpenAPI 是协议及生成输入，不是第二个 HTTP 框架。前端依赖和生成配置归 Web，根入口只编排；继续使用根唯一 pnpm 锁文件。所有新版本在能力验证时明确固定，不把官方 main 分支中的新特性假称为当前安装版本已具备。

## 2. Gin：明确使用哪些能力

| 能力入口 | 本项目采用方式 | 删除或避免的自制机制 | 验证要点 |
| --- | --- | --- | --- |
| `gin.New`、`Group`、`Use`、路由级 middleware | 全局诊断、IAM 会话/CSRF、内部服务身份、组织授权、限流按实际边界挂载 | Chi、手工 Handler 转接表、散落鉴权和重复外围路由匹配 | 公开/内部监听隔离；拒绝时确实停止 Handler；健康检查正确分组 |
| `CustomRecoveryWithWriter` | 使用原生 panic 捕获；禁用内置请求转储输出，回调仅记录受控类型/堆栈并交给统一故障输出 | 自写通用 recover 框架、GVA 自定义 Recovery 和异常入库平台 | panic、连接断开、Session 补偿、SSE 已提交场景；不泄露 Cookie/密码/原始 panic |
| `c.Error`、`c.Errors`、`c.Abort`、`c.Next` | 普通业务/依赖错误进入请求错误链，一处分类映射；Handler 报错后 Abort 并 return | Handler 到处自行决定 code/status/message；逐层同样的错误响应 | `Abort` 不会退出当前函数；不得遗漏 return；私有 error 不直接序列化 |
| `c.JSON`、`c.Status`、原生 render、生成的 response visitor | JSON/空响应由框架或生成代码编码；保留很薄的 Result 语义构造和故障映射，不在响应完成后再次包装 | 自制通用编码/状态跟踪层、捕获任意响应再套 Result 的中间件 | 201/204/4xx/5xx、编码失败、首次提交时机；同一响应只有一个输出责任方 |
| `c.Writer.Status/Size/Written`、`c.FullPath` | 请求完成日志直接读取 Gin 路由和 Writer，接入现有 slog；未知路由用固定标记 | Chi 路由预匹配、CaptureRoute、通用 ResponseWriter 计数器 | CSRF/会话提前拒绝仍有 request_id；日志不记录实际资源路径、query 和正文 |
| `ShouldBindUri/Query/Header/JSON/With`；生成绑定器 | 优先用生成的 Gin 接入代码绑定协议；手写入口使用原生 ShouldBind；严格体积/媒体类型边界用最小组合补齐 | 反复 `URL.Query`/`Atoi`/Decode、自制反射校验和 BodyValidated 标记 | 空体、未知字段、尾随 JSON、UUID/整数、PATCH 三态、协议错误映射 |
| validator 的字段名提取、内建规则、`dive`、struct-level 校验 | 配置或真正非协议结构使用成熟校验；HTTP schema 已拥有的规则不再手写一套 binding tags | 自制通用校验 DSL、重复维护 schema/struct 正则 | 字段名使用公开 JSON 名；零/false/缺省/null；错误只公开稳定类别 |
| `SetTrustedProxies`、`RemoteIPHeaders`、`ClientIP` | 显式 CIDR，仅采用已定义的转发头；没有可信代理时不信任转发链 | 手写 IP 链解析和第二套代理信任策略 | IPv4/IPv6、无效头、不可信来源、可信多级代理 |
| `NoRoute`、`NoMethod`、`HandleMethodNotAllowed` | 统一 404/405/Allow；明确 HEAD/OPTIONS、斜杠及路径修正配置 | 手工遍历方法并二次匹配的 RouterErrors | 未匹配请求不变成 200；错误入口不误作认证绕过或重定向 |
| `SSEvent`/Writer Flush、标准请求取消 | Console 使用原生流能力和 `Request.Context()`；保留业务要求的定期授权 | 自制通用流框架、对流套 JSON 或普通 Session 响应缓冲 | 首次 flush、客户端取消、撤权/故障关闭；不因 flush 伪造活跃会话 |
| `Engine.Routes` | 对照契约、operationId 和权限元数据；生成路由也必须在真实 Engine 中检查 | 数据库可编辑 API 目录、手写路由影子注册表 | 完整清单与真实访问控制测试分别证明，二者不能互相替代 |
| 标准 `http.Server` 挂载 Gin Engine | 明确 Header/Read/Write/Idle 策略，普通请求和长流不同；业务 I/O 传递 context deadline | 每个超时都起 goroutine 执行 `c.Next`、多套通用超时包装 | 截止后不并发写响应、Shutdown 等待/取消行为及监听失败清理 |

Gin 提供响应渲染和错误机制，**不内置 SecPlexus 的 Result 字段或业务错误码**。薄的业务映射是协议策略，不应被夸大成 Gin 自动完成，也不应据此保留整套自制 HTTP 框架。

源码核对：[Recovery](https://github.com/gin-gonic/gin/blob/master/recovery.go)、[Context](https://github.com/gin-gonic/gin/blob/master/context.go)、[Writer](https://github.com/gin-gonic/gin/blob/master/response_writer.go)、[JSON renderer](https://github.com/gin-gonic/gin/blob/master/render/json.go)、[Engine](https://github.com/gin-gonic/gin/blob/master/gin.go)。Recovery 的默认日志仍可能转储非 Authorization 请求头；自定义回调本身不足以关闭它。实施须核对固定版本的 Writer 参数和断连分支。

## 3. 其他后端组件：利用能力与边界

| 组件 | 采用的深层能力 | 对当前代码的具体改造 | 保留/不启用的边界 |
| --- | --- | --- | --- |
| GORM | `gorm.G[T]` 类型化 CRUD、context、返回 error；`Transaction` 回调；Scopes、条件 Preload/Join、批量操作、Clauses、RowsAffected、错误翻译、logger 扩展 | 普通路径优先泛型 API；以回调事务取代重复 Begin/defer Rollback/Commit；用类型化投影替代 SQL JSON 和 map 写入；明确关联的组织复合键 | 参数化 SQL/锁可保留；不做万能仓储、AutoMigrate、无理由 Hook；审计显式在同一事务，不塞进隐式全局 Hook |
| PostgreSQL 驱动与错误 | GORM `TranslateError` + `errors.Is`；pgx `PgError` 和连接池 | 当前 postgres v1.6.3 的翻译器保留 PgError 错误链，采用前以真实唯一键/FK/检查约束验证；集中把已知约束映射到业务冲突 | 不暴露 Message/Detail/SQL；不因翻译而丢失约束名或把所有数据库错误当 409 |
| GORM logger | context Trace、级别、慢查询策略、参数化日志能力 | 通过官方 logger.Interface 接 slog，只补项目脱敏策略；设置可解释的慢操作阈值 | `ParameterizedQueries` 不能抹掉数据库错误中的 Detail；禁止生产 `Debug()` 输出 SQL/凭证；不新建日志平台 |
| pgxpool / go-redis | 原生池、超时、取消、健康检查、关闭及池统计；Redis 原生脚本/连接管理由组件承担 | 明确 pool/dial/read/write 等实际限制和拥有者，删除重复网络连接包装；不为 GORM/SCS 建第二个独立池 | 现有两服务无 Cluster/Sentinel/分库需求，不预建多驱动；不盲目启用自动重试、Pub/Sub、观测后端 |
| `ulule/limiter/v3` + Redis Store | 原子固定窗口计数、TTL、剩余额度/Reset、Gin KeyGetter/错误/超限扩展 | 替换 `authentication/ratelimit.go` 自写 Lua；IP 与账号两个桶，库负责原子计数，项目提供摘要 key 和 429/503 映射 | 业务阈值仍为 60次/分钟、20次/15分钟；库前缀+业务键在 cache/key 统一组合为 `spx:iam:...`；不得 fail-open 或退回单机内存 |
| Goose | Provider、embed FS、自定义 Store、SessionLocker、Status/HasPending/GetVersions、上下文与迁移事务 | 迁移/状态走成熟 Provider，版本来自迁移集合而不是硬编码 `version == 2`；CLI 提供只读状态和显式升级 | Provider 查询可能初始化版本表；普通 serve 先用 Store 的只读能力确认已初始化，再查询状态，缺表直接失败，不触发建表。禁止 AutoMigrate/默认 down |
| SCS + pgxstore | 原生 Load/Commit/Status/RenewToken/Destroy、Cookie 属性、token 摘要、ErrorFunc/LoadAndSave | 删除 Authboss 会话桥；优先使用原生生命周期方法，只保留项目真实需要的提交屏障和补偿 | 已审查 LoadAndSave：不能未经验证就替代“提交失败不得附带成功正文/Cookie”的屏障。保留窄适配不等于自制 Session；逻辑 idle/absolute 仍由 IAM 管理 |
| Argon2id | CreateHash、DecodeHash、ComparePasswordAndHash；盐和比较由库承担 | 删除 Authboss Hasher 接口适配，保留清楚的密码政策/PHC 参数上限；已保存的合法哈希继续可验证 | 不自写密码学，不降为 bcrypt/JWT 模板默认；并发哈希资源上限仍是应用策略 |
| gorilla/csrf | 签名 token/Cookie、Secure/HttpOnly/SameSite、RequestHeader、ErrorHandler | 使用组件产生/校验 token，通过 ErrorHandler 接统一错误链；项目只描述可信 Origin 和同源规则 | 不自己比较 token，不开放任意凭证跨域；不复制 GVA CORS 逻辑 |
| Viper + mapstructure | 显式 BindEnv、SetDefault、AllowEmptyEnv、UnmarshalExact、组合 DecodeHook、DecoderConfigOption | 删除自制占位符解析；用官方 duration/slice hook 和明确的类型转换解码嵌套配置；启动后仅传类型化结果 | 默认 WeaklyTypedInput 并不严格，须显式配置和验证；不复制 WatchConfig 热改全局对象；不使用实验性隐式绑定来掩盖遗漏 |
| Cobra/pflag | 子命令、PersistentFlags、Args/必需参数校验、RunE、命令 context、帮助/退出错误 | 替换 IAM 手工首参数分流与 flag 分支；serve/migrate/status/maintain/revoke-after-restore/reset-password 明确；Console 同样接统一命令规范 | 不建立全局命令/配置单例；敏感输入继续用终端能力，不放命令行或生成补全输出；端点输出命令不建立连接 |
| 标准库 + x/sync | signal.NotifyContext、http.Server.Shutdown、errors.Join、必要的 errgroup | IAM 多监听和 Console 生命周期采用成熟取消/汇总能力，资源按创建顺序反向清理 | 无须引入反射 DI 容器或通用任务调度；errgroup 取消不自动关闭 Listener，关闭动作仍须明确 |
| slog + Lumberjack | 结构化字段、context 关联、Handler/LogValuer、现有轮转能力 | 统一 Gin/GORM/Redis/迁移的诊断字段和错误清洗，不因 GVA 使用 Zap 就同时维护第二套日志 | 不做异常数据库或收集全量请求响应；轮转文件单进程拥有，外部日志采集不在本次部署范围 |

GORM 依据：[泛型 API](https://gorm.io/docs/the_generics_way.html)、[事务](https://gorm.io/docs/transactions.html)、[错误](https://gorm.io/docs/error_handling.html)、[日志](https://gorm.io/docs/logger.html)、[当前 PG 驱动翻译器](https://github.com/go-gorm/postgres/blob/v1.6.3/error_translator.go)。不把版本文档的性能宣传当成项目实测，不为追求速度关闭业务事务。

其他依据：[限流 Store](https://github.com/ulule/limiter/blob/master/drivers/store/redis/store.go)及[Gin middleware](https://github.com/ulule/limiter/blob/master/drivers/middleware/gin/middleware.go)、[Goose Provider 文档](https://pressly.github.io/goose/documentation/provider/)及[v3.24.3 初始化源码](https://github.com/pressly/goose/blob/v3.24.3/provider_run.go)、[SCS 数据操作](https://github.com/alexedwards/scs/blob/master/data.go)及[会话中间件](https://github.com/alexedwards/scs/blob/master/session.go)、[Viper 源码](https://github.com/spf13/viper/blob/master/viper.go)、[Cobra](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)、[go-redis](https://github.com/redis/go-redis)。

## 4. 契约与 Web：一套输入驱动工具

### 4.1 选定契约优先，深化生成能力

本轮选择 OpenAPI 3.x 为唯一协议真源，继续放 `src/contracts`，这是工具组合与减少手写代码的选择，不是兼容旧实现。采用 oapi-codegen 的 `gin-server` 及适用的 `strict-server`、官方 Gin OpenAPI 校验中间件，生成 Go 模型、接入/参数绑定、类型化请求/响应和实际需要的内部客户端。

strict server 并不自动完成所有输入校验。公共 schema 规则交给官方 Gin middleware/kin-openapi；生成代码消费其验证后的值或成熟绑定结果，禁止再手写重复的 DTO 规则和反射转换。原生 validator 只用于真正由其拥有的配置/非协议校验。有 `$ref` 跨包时配置官方 import-mapping，不让公共客户端导入应用 internal；必要时只调整协议源拆分，不增加自制生成模板。

公开和内部操作分别拥有契约入口/生成配置，共享 schema 使用引用；每个操作只有一个源定义。Web 仅消费公开契约生成 SDK/schema，不把内部 authorize 或服务凭证方案打进浏览器产物；内部 Go 客户端只消费对应内部契约。允许重排现有混合文档，不靠手写第二份 public DTO 解决隔离。

SSE、健康检查及必须原生控制响应的少数操作可采用原生 Gin Handler，但仍在契约/路由清单中明确记录；没有第二个路由器。为它们设清晰的生成排除/手工挂载边界，不让两个入口重复注册同一操作。错误 Hook 统一接项目错误映射，不能沿用生成器默认 text/plain/裸 500。

不采用“Go DTO → Swagger 2 → 转换为 OpenAPI 3 → 重新修补”的链路；不叠加 Huma/Fizz 等第二套 API 绑定框架。采用成熟代码优先框架本身并非错误，但当前 Gin、跨模块契约、前后端生成已经有直接组合，额外框架没有证明能删除更多机制。

依据：[oapi-codegen Gin](https://github.com/oapi-codegen/oapi-codegen/blob/main/docs/gin-server.md)、[strict server 与 import mapping](https://github.com/oapi-codegen/oapi-codegen)、[官方 Gin 校验中间件](https://github.com/oapi-codegen/gin-middleware)。最新中间件说明了保留验证值的能力；D1 必须固定兼容版本组合并验证，不能把 main 上的特性直接归给现有 v2.4.1 生成器。

### 4.2 Web 采用明确组件，替代现有手写机制

| 组件 | 采用方式 | 删除或收敛的内容 | 安全与验收 |
| --- | --- | --- | --- |
| Hey API `openapi-ts` | 从本地契约生成 TS、Fetch SDK、Zod v4 schema、TanStack Query options；输出及配置在 Web | 替换只生成静态 TS 的单用途链路、页面字符串 URL/手写 DTO、`decode<T>` 的任意类型承诺 | 本地离线输入，不上传私有协议；schema/错误状态/204/字符串版本号/nullable 一致，固定生成器版本 |
| 生成 Fetch client | 官方 request/response/error interceptor 承担通用请求流程；项目只注入同源、CSRF、身份代次及稳定错误策略 | 自写通用 fetch/JSON 编解码层 | 禁止盲目重试 mutation/登录；非 JSON 401/503 和取消正确区分；客户端不存凭证 |
| TanStack Query v5 | 使用生成 query/mutation options、AbortSignal、失效刷新、分页/依赖查询；QueryClient/MutationCache 管理服务端状态 | 自制 `useList` 的 effect/计数 revision/loading/error/reload，页面重复拉取逻辑 | key/客户端作用域包含身份代次、组织和授权版本；退出/撤权/切换取消并清缓存，旧结果不得写回 |
| Query 默认策略 | 显式定义 staleTime/gcTime/retry/focus/reconnect；认证/权限查询立即失效，所有 mutation 不自动重试 | 依赖框架默认三次重试、缓存或 focus refetch 的隐式行为 | 401/403 不重试；503 进入不可用状态；失败不把旧权限当有效授权；不持久化敏感缓存 |
| React Hook Form + Zod resolver | 生成请求 schema 派生表单校验；使用 handleSubmit、Controller/register、formState、setError、reset | 自制通用 fields DSL、FormData 到 string map、手动 busy/error/dirty 状态 | 保留真实类型和 PATCH 缺省/null/false；密码确认等纯 UI 规则可独立 refine；校验消息接 next-intl，不记录值 |
| TanStack Table | 现有组件深入使用列/行模型、稳定 getRowId、受控服务端分页；权限决定可执行动作 | IAM 自制另一套列表状态/列渲染机制 | 不在客户端二次分页服务端分页结果；保持 shadcn 的相同 DOM/主题样式；不顺便新增业务排序/导出 |
| Zustand | 内存身份/界面状态使用 selector 和明确 action；服务端列表归 Query | 整份 store 订阅引起无关重渲染、重复缓存同一服务端资源 | 角色/会话/业务数据不进 localStorage；身份 epoch/跨标签事件是策略，不能机械删除 |
| Next.js | 按已安装文档采用路由错误边界、not-found/loading、Server/Client 分界、服务端配置隔离与流式 Route Handler | 自制整站异常路由/重复错误壳、把服务配置注入浏览器 | 客户端异常与后端故障分层；身份数据不进共享 RSC/Fetch 缓存；不把 IAM CRUD 再放 Server Action/Console 实现 |
| next-intl + shadcn | 保留类型化消息、ICU、formatter、错误/权限标签映射；复用官方 Field/FieldError 和既有 Dialog/Table | locale 条件分支、手拼翻译、手造控件 | en/zh-CN 同步；不更改 CRM V1、字体、主题、共享控件和产品导航基准 |

Query 的生成 key 默认不自动代表登录身份，mutation key 还可能包含输入正文。必须关闭含密码/凭证的 mutation key 生成、敏感 Devtools/持久化及任何请求正文日志；登录/改密等敏感凭证操作直接使用 SDK + 表单状态，不进入 Query MutationCache，完成即清理临时状态。身份切换时的取消和缓存清理是业务边界，不能以“Query 自动处理竞态”为由删掉。

依据：[Hey API SDK/运行时验证](https://heyapi.dev/docs/openapi/typescript/plugins/sdk)、[Fetch client](https://heyapi.dev/docs/openapi/typescript/clients/fetch)、[Zod](https://heyapi.dev/docs/openapi/typescript/plugins/zod)、[Query plugin](https://heyapi.dev/docs/openapi/typescript/plugins/tanstack-query)、[MIT 许可说明](https://heyapi.dev/docs/openapi/typescript/license)、[Query 默认行为](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)、[取消](https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation)、[shadcn RHF 指南](https://ui.shadcn.com/docs/forms/react-hook-form)、[next-intl 类型能力](https://next-intl.dev/docs/workflows/typescript)。Next.js 同时阅读了本地安装版本的错误处理及 Route Handler 文档，实施时继续以安装版本为准。

Table 另核对[官方 React 分页指南](https://github.com/TanStack/table/blob/main/docs/framework/react/guide/pagination.md)的 manualPagination/rowCount；实现使用已安装 v9 的 API，不照搬 v8 示例或无理由升级。前端组合遵循根目录官方 shadcn Skill，仅改变项目逻辑机制。

## 5. gin-vue-admin 逐项取舍

固定参考提交：`cee04b08d1262036ca33e3026e1e13d29471c7d5`。以下评价限定于读到的源码，不代表对整个项目做过完整安全测试。

| 上游实现 | 采用决策 | SecPlexus 落点 |
| --- | --- | --- |
| initialize/router.go 的公开/受保护组、模块注册 | 学习分组挂载及接入说明 | Gin 全局/分组/路由中间件，生成与手写操作边界；显式依赖替代全局 GroupApp |
| model/common/response 的集中响应 | 学习统一调用点，重写其策略 | Gin/生成 response visitor 输出 Result，真实 HTTP 状态；不复制所有失败 200 和整数 0/7 |
| middleware/logger.go 的前后置处理及扩展 | 学习扩展点，使用 Gin Writer/FullPath | slog 安全摘要，不读取 body/query，不输出 `c.Errors.String()` 的原始消息 |
| middleware/error.go | 使用 Gin 原生 Recovery 加受控回调替代 | 不复制请求头 Dump、任意 panic 入库、全局 DB 依赖 |
| middleware/operation.go | 区分访问日志与审计；不移植正文捕获 | 审计仍由用例在业务事务中写入；无密码/响应缓冲或事后独立写审计 |
| middleware/limit_ip.go | 学习按路由配置限流的意图，机制由成熟 limiter 承担 | 不复制 Exists/Get/Incr 的分步逻辑、Redis 为空放行和 200 错误 |
| middleware/timeout.go | 不复制 | 禁止 goroutine 内 `c.Next` 与超时分支并发写同一 Context；标准 deadline + 数据库/HTTP 取消 |
| middleware/cors.go | 不复制；当前同源无需新 CORS 中间件 | 不反射任意 Origin 加 Credentials；未来真有跨域需求才选官方维护的 cors 组件和明确名单 |
| JWT/Casbin middleware | 不照搬安全模型 | 保持 SCS 可撤销会话、稳定 action、组织及委派裁决；不新增平行 JWT/路由权限体系 |
| GORM 模型、查询和生成模板 | 学习模型/关联/批量/排序白名单的适用部分 | 泛型 GORM + 显式事务/投影；不复制全局 GVA_DB、直接绑定数据库模型和运行时 AutoMigrate |
| Viper 初始化 | 学习配置集中加载，使用其更多正式能力 | 严格解码/环境绑定/模块配置；不 WatchConfig 修改已经启动的依赖 |
| 业务范例、aiDoc 模块接入说明、生成器测试 | 学习真实范例与规则同步 | 现有角色管理端到端样板、官方生成产物回归；不建第二套 aiDoc/memory 或可视化 AST 生成平台 |

源码入口：[中间件目录](https://github.com/flipped-aurora/gin-vue-admin/tree/cee04b08d1262036ca33e3026e1e13d29471c7d5/server/middleware)、[路由组装](https://github.com/flipped-aurora/gin-vue-admin/blob/cee04b08d1262036ca33e3026e1e13d29471c7d5/server/initialize/router.go)、[统一响应](https://github.com/flipped-aurora/gin-vue-admin/blob/cee04b08d1262036ca33e3026e1e13d29471c7d5/server/model/common/response/response.go)、[服务模板](https://github.com/flipped-aurora/gin-vue-admin/blob/cee04b08d1262036ca33e3026e1e13d29471c7d5/server/resource/package/server/service/service.go.tpl)。复制文件时另记来源/改动/许可；当前仅参考和分析。

## 6. 工程治理与选型证明

- 新增明确依赖：Gin、Cobra/pflag、Viper/mapstructure、成熟 limiter、官方 Gin 协议中间件、Hey API、TanStack Query、RHF/resolvers；复用已有 GORM/Goose/Redis/SCS/Argon2id/CSRF/Zod/Table/next-intl，不同时引入同职责替代品。
- 每项工具固定版本，并记录支持的 Go/Node/Next/React/Zod 组合、许可证、生成配置和本地校验命令；评估依赖维护与漏洞，不把采用官方 main 特性等同于可安装稳定版本。
- Go 工具通过固定版本命令/工具声明管理；前端生成工具放 Web，根脚本委托。生成只读本地协议，先完成受控输出再替换产物，检查新增/删除文件和连续生成一致性。
- 本地静态检查采用现有 Biome/TypeScript、Go vet、必要的固定版本 golangci-lint 及 govulncheck；只启用本次有明确收益的错误/资源/静态分析规则，不为了通过而全局屏蔽或清理无关历史风格。不建立远程 CI、部署或自动发布。
- 三份真相保持明确：业务语义由 Spec/模块文档维护；协议结构由 OpenAPI 维护；安装/生成版本由依赖清单与锁文件维护。框架能力说明链接真实代码/配置，不把参考项目范例升级为未经批准的产品要求。
- 在 IAM/Console 真实链路和角色管理范例中逐项验证，而非新建演示应用。采用后必须删除替代的 Lua、通用绑定/编码/路由包装、认证桥、手工 CLI、版本常量、列表请求状态与表单 DSL；保留的窄适配必须说明组件无法直接表达的具体业务约束。

## 7. 实际接入与固定版本

| 组件 | 固定版本 | 实际能力与替代证据 |
| --- | --- | --- |
| Gin | 1.12.0 | `httpx/middleware/gin.go`原生Recovery/FullPath/Writer/404/405；公开Group/内部独立监听；Console SSEvent/Flush；删除Chi、CaptureRoute和手工路由表 |
| oapi-codegen / runtime / Gin middleware / kin-openapi | 2.8.1 / 1.8.0 / 1.2.1 / 0.152.0 | Go类型、公开Gin、内部strict及Go客户端；验证值复用、官方BodyDecoder扩展；公共/内部bundle分离；删除BodyValidated和TS-only生成器 |
| Redocly / Hey API | 2.7.0 / 0.99.0 | 标准bundle/移除未使用schema；Web SDK/Zod/Query；临时生成、清单、只读重生成校验；日期offset与JSON计数回归保护 |
| Viper / mapstructure / validator | 1.21.0 / 2.4.0 / 10.30.3 | 实例隔离、显式多环境优先级、严格解码Hook；配置结构使用必需/范围/IP及dive-CIDR规则；来源/密码字节数/时间关系等业务策略仍由模块负责；删除configfile占位符方言 |
| Cobra | 1.10.2 | 根/子命令、继承配置标志、参数与帮助；status独立命令，端点检查无网络；删除手工flag分流 |
| Argon2id / SCS / gorilla/csrf | 1.0.0 / 2.9.0 / 1.7.3 | 类型化密码用例、PHC参数限制、原生会话与CSRF；保留未知账号替代比较、锁定/审计/并发版本；删除Authboss桥及依赖 |
| ulule/limiter / go-redis | 3.11.2 / 9.7.3 | 官方Redis原子Store、固定窗口/TTL、双摘要桶和真实Retry-After；超时/禁用自动重试/503 fail-closed；删除自写Lua |
| GORM / pgx / Goose | 1.31.2 / 5.11.0 / 3.24.3 | 泛型公开投影、管理/授权/失败审计Transaction回调、批量授权关联、原生错误翻译保留PgError链、原有安全Trace；Provider/Store状态与显式迁移、缺表查询无DDL；删除schema版本常量 |
| Query / RHF / resolvers / Zod | 5.105.0 / 7.91.0 / 6.0.2 / 4.6.7 | 生成options与身份/版本键、取消清缓存、明确缓存策略；生成schema派生表单、Controller/FieldError、重置及提交态；删除useList/revision与fields字符串map DSL；密码不进入MutationCache |
| TanStack Table / Next / next-intl | 9.3.1 / 16.4.3 / 4.14.9 | 复用既有表格的受控服务端分页；保留同源代理、加载/错误边界和双语；未修改CRM V1、UI组件和主题样式 |
| golangci-lint / govulncheck / goimports | 2.15.0 / 1.9.0 / x/tools 0.51.0 | 本地检查；工具使用项目Go 1.27.3，不增加远程CI/部署 |

必要窄适配：Result先编码以便编码失败时尚未提交状态，再交Gin renderer写出；IAM独有SCS提交屏障保护成功正文/Cookie及补偿，Console流不经过它；官方校验器丢失MaxBytesReader的结构化错误时仅记录溢出标记；gorilla/csrf只在HTTP适配边界组合；生成内部客户端之外只保留TLS/凭证/有界响应及授权结果策略。上述代码不另建HTTP框架或schema编译器。

SCS当前`IdleTimeout=0`，空闲/绝对期限由IAM逻辑会话事务验证，内部只读Load不触发缓存提交；授权事务仍更新受绝对期限限制的逻辑活跃时间。不能仅凭“读请求”推断其他SCS配置也无需Commit。

许可证与参考：新增组件通过包管理器使用，不复制其业务源码；依赖版本由go.mod/go.sum和唯一pnpm锁文件固定。gin-vue-admin仅参考分组/中间件/装配思路，研究固定提交`cee04b08`，未引入其业务生成后台或数据库可编辑路由。前端继续保留既有MIT模板鸣谢及许可；本地上游参考提交`89f28da`，本轮未改CRM V1图表、UI组件、字体和主题基准。生成器输出保留第三方生成标记，不手改产物。

许可证核对来自安装版本的 LICENSE/package.json：Gin、validator、mapstructure、Viper、limiter、Redocly CLI、Hey API、Query、RHF/resolvers 为 MIT；Cobra、oapi-codegen Gin middleware/runtime 为 Apache-2.0。保留各包原始许可与生成声明，不把包管理器依赖误写成项目自有实现。
