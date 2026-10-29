# 框架能力审查与采用决策

修订日期：2026-10-29。状态：用户确认原生 Gin、必要自动化及配置收敛方向，当前按最终无 OpenAPI 方案实施；旧工具链仅作为历史快照。现行目标见 [Spec](spec.md)，执行门槛见 [Plan](plan.md)；第 7 节是此前已实现版本快照，不能当成本次最终选型或验收。

## 1. 组合决策

```text
原生 Gin 路由/Handler + Go DTO + binding/validator
  ├─ 运行时结构边界 → 类型化用例 → GORM/pgx 事务
  ├─ Argon2id / SCS / gorilla/csrf / 成熟 Redis limiter
  └─ 模块 HTTP DTO/README + 真实协议测试
Cobra → Viper/mapstructure → 模块 YAML 默认值 + 明确环境差异
本地：Next 同源入口 → IAM / Console；无强制 Caddy、无 Web 第二端口
Web：Axios / Query / RHF / Zod / Table / next-intl；只导出业务权限/结果码常量
```

“有生成文件”不是缺陷；问题是是否新增第二个规则所有者、反过来限制原生开发、迫使持续修补工具，或者让用户维护可推导配置。按真实收益选能力，不按框架功能清单全部启用。

## 2. Gin：明确使用哪些能力

| 能力 | 采用与所有权 | 不保留/不新增 |
| --- | --- | --- |
| Group/Use/方法注册 | 模块 router 显式注册公开/内部与保护分组 | 生成 Gin wrapper/strict 路由、影子路由平台 |
| ShouldBindJSON/Query/Uri + validator | 结构约束一次绑定；保留必需、枚举、集合/路径、PATCH 语义 | 同时运行 OpenAPI schema 和相同 struct/service 字段检查 |
| Recovery/c.Error/Writer/FullPath | 薄错误码映射、受控日志、真实状态与必要头 | 通用 recover 引擎、原始异常转储、已编码响应再包装 |
| 原生流/取消 | SSE 直接响应，标准 context/Shutdown 关闭 | 为 SSE 强制 JSON/完整响应缓冲 |
| TrustedProxies | 只在真实信任链成立时显式配置；本地承认代理来源 | 任意 XFF/全网可信、为当前本机强制加入口进程 |
| 成熟认证适配 | IAM SCS 提交保护、CSRF 与密码组件，业务规则归用例 | Authboss 桥、HTTP 凭证重放、第二套会话系统 |

Gin 的默认绑定不自动满足单 JSON 值、未知字段、三态和体积等全部要求。T2 必须验证后仅补必要标准库/正式扩展，不能换成另一套 API 框架。[Gin 绑定说明](https://gin-gonic.com/en/docs/binding/)。

## 3. 其他后端组件：利用能力与边界

| 组件 | 保留的实际用途 | 边界 |
| --- | --- | --- |
| GORM/pgx | 类型化投影/批量关联、显式字段、锁、Transaction、PgError/脱敏日志、共池 | 不机械禁用必要 SQL，不新增 BaseRepository/自动关联授权/AutoMigrate |
| Goose | 显式版本、迁移、状态和锁 | 不硬编码版本，不让 serve/status 隐式 DDL |
| Viper/mapstructure/validator | 显式环境绑定、严格解码、duration/列表、配置校验 | YAML 只存一套稳定默认值；不用整套 SetDefault 复制、不热更新依赖、不自写配置语言 |
| Cobra | 参数、子命令、context、离线端点检查 | 不为每个字段提供一套 CLI 覆盖 |
| Argon2id/SCS/CSRF | 哈希/物理会话/CSRF 通用机制，IAM 持有逻辑版本/期限/审计 | 不恢复 Authboss 或替换 JWT；保留实际提交补偿 |
| limiter/go-redis | 原子窗口、TTL、摘要 key、429/Retry-After、503/取消 | 不重写 Lua、退回单机计数或默认重试写操作 |
| net/http/context/errgroup | 监听、TLS、启动失败释放、活跃请求取消、Shutdown | 不新增生命周期框架，正常完成不靠 SIGKILL |

业务授权、委派、状态、版本、最后管理员和事务审计不能因为组件已经校验字段而省略。

## 4. Web 与最小生成

| 能力 | 实际用途 | 删除对象 |
| --- | --- | --- |
| Axios 1.21.0 | 官方拦截器、超时、取消和 JSON；平台薄适配核对 Result/状态/身份 | Hey API SDK、客户端核心、响应元数据生成 |
| Zod | 实际消费的响应和表单模型，类型由 z.infer 推导 | 生成 schema、独立 TS DTO、修补转换 |
| TanStack Query/RHF | 原生 query/mutation、范围缓存、表单与错误状态 | 生成 Query options；密码不进入 MutationCache |
| 业务目录导出 | Go/Web 共用结果码与权限键，四份导出可只读复核 | 中央 contracts、Swag/OpenAPI/Redocly 工具链 |

HTTP 字段同步通过模块文档、真实协议/模型边界/E2E 测试保护。目录分别归所属模块和实际共享包，不以另一名称重建中央协议平台。

## 5. gin-vue-admin 逐项取舍

固定参考提交：[cee04b08d1262036ca33e3026e1e13d29471c7d5](https://github.com/flipped-aurora/gin-vue-admin/tree/cee04b08d1262036ca33e3026e1e13d29471c7d5)。

- 采用直接的 Gin 分组/中间件、具名 Handler、模块 Go DTO 和代码附近的接口说明，减少开发时需理解的层数。
- 参考 GORM/可运行业务范例，不导入可视化脚手架、Vue、动态数据库路由或尚未使用的产品功能。
- 不照搬全局 DB/依赖对象、JWT/Casbin、单角色 URL 授权或业务失败统一 HTTP 200；本项目已有更具体的会话、组织和事务约束。
- 不因它用了 Swag 就跳过完整能力验证；它的文档、校验、前端调用方式不等于本项目已经可用的端到端组合。

## 6. 工程治理与必要性审查

每个新增组件/抽象/配置先写：当前问题、原生能力不足、可删除对象、维护成本、可验证结果。可推导上游地址/元信息不让用户重复配置；本地没有部署需求，不强制 Caddy、第二 Web 端口或安装流程。稳定默认值归 YAML，根编排不静默覆盖用户有效值。

保持隔离测试、许可证和已确认业务边界；本次不部署/构建应用镜像/远程 CI/Git 交付。desloppify 是证据入口，生成/第三方误分类、检测缺失和未完成主观评审不能当成架构评分；不可用质量工具不得伪造通过。

## 6.1 当前消费层选型补充

Axios 固定 1.21.0（MIT），使用官方请求/响应拦截、JSON、超时与 AbortSignal。Go 模型归所属模块，共享内部鉴权类型归 iamclient，结果归 result，权限语义归 IAM authorization；Web 消费 Zod 与 API 归 Web。旧 Swag/Hey/Redocly 与中央 contracts 均已删除。下节仅为历史，不作为当前依赖清单。

## 7. 实际接入与固定版本

以下完整保留前一次实现的版本和适配快照，属于历史事实。oapi-codegen 后端接入、全量生成及 Caddy 方案已被当前整改目标替代；不要求继续维护这些旧组合，也不把此表写成新方案完成。


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



## 8. 本次明确撤回与待验证事项

- 撤回：本地 Caddy v2.11.7 强制入口及其安装/检查/编排、SPX_WEB_INTERNAL_PORT=5174 和派生配置重复填写；不修改用户系统 Caddy。
- 替换：OpenAPI 反向生成 Go 路由/DTO/strict 与生产请求校验。Web 改用 Axios/Zod，不保留 API 生成链。
- 待验证：原生 PATCH 三态、完整文档/消费者组合、直接 Next 的 dev HMR/preview、实际 Console 流退出及二次认证可见错误。
- 明确不包含：公网多用户真实 IP 独立限流和生产代理信任链；部署前必须另行验证，不以本轮完成抹掉该限制。
