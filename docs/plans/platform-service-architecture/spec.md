# 平台服务架构与工程规范重构

> 密码行为以 [简化密码策略](../simplify-password-policy/spec.md) 为准；本文的其他功能和验证范围保持不变。
> Web 基准更新：本文的 Arco/Vite/Redux 前端实现记录已由 [Web 整体替换计划](../replace-web-template/plan.md) 承接。现采用 Next.js/shadcn 基准并重建 IAM 页面；服务端业务与授权契约继续有效。


状态：方案已实施；具体本地验收结果与未验证项以 Plan 为准。审查依据见 [审查记录](review.md)。范围为当前 Web、IAM、Console、共享 Go 包、接口契约、配置与部署；Python 等后续服务遵守跨语言契约，本次不创建尚不存在的服务。

## 目标与约束

- HTTP 请求、响应、错误码、日志关联和接口文档在各服务之间一致；普通 JSON API 使用统一 Result，前端只维护一套解析逻辑。HTTP 状态码仍表达成功和失败，不以总是 200 代替协议语义。
- 路由按服务和业务集中注册；处理器只处理 HTTP，应用层处理用例和事务，持久层负责数据库访问。依赖在启动时组装，context 用于请求、截止时间及取消，不当作应用依赖容器。
- 普通数据访问采用成熟 ORM；复杂事务、锁和 PostgreSQL 特性允许有理由地使用参数化 SQL。数据库迁移独立、可审查、可重复升级，不依赖运行时自动改表。
- 按职责整理文件、中文注释、空行与长表达式；保留少量真实共享能力，不因为可能复用就做通用框架。当前无须兼容旧代码和旧 HTTP 格式；已有真实数据和已执行迁移不得因重构丢失。
- 默认同一个 PostgreSQL 实例、数据库及账号，模块可分别配置；迁移、维护和运行默认使用同一账号。普通开发启动只连接既有基础设施，Compose 为显式部署入口。

## 决策与范围

| 状态 | 内容 | 实施规则 |
|---|---|---|
| 本轮目标 | 统一 Result/错误码、业务分组路由、ORM 数据访问、可发现的迁移目录、cache 命名与清晰代码 | 属于本次重构，不新增身份产品或业务页面 |
| 保留现状 | 单 Go module、pnpm workspace、Authboss/SCS、Goose、go-redis、日志与原有安全规则 | 不借重构更换框架、升级无关依赖或重新实现已有组件 |
| 先试点 | chi 分组、GORM 与现有事务/会话适配、Result 类型生成 | 用一条现有业务链验证，结果记入 Plan；失败先定位具体缺口，不立即引入另一套框架 |
| 按真实需要提取 | 共享 helper、接口、事务适配、连接封装 | 当前存在复用或可独立说明的边界才提取，不按目录图预建层次 |
| 本轮不建设 | 新缓存/数据库驱动、滚动升级机制、通用重试/熔断框架、未来资源权限模型、全站视觉改造 | 发现实际需求后另行评估，不先加配置、接口或测试矩阵 |

已有授权、状态检查、事务与清理属于必要行为，不因“少写防御代码”而移除。对新增的推测性竞态或极小概率边界，先说明触发条件、实际发生可能及复杂度成本；需要扩大方案时再取得确认。不得把“未来可能需要”列为本轮必做。

## 调研及差距

主要参考：已将 [Gitea](https://github.com/go-gitea/gitea/tree/643bd3805160a587a1bcf10e59dcde8016c64672) 浅克隆并检出路由、服务、模型和数据库迁移源码；比对 [Grafana 数据库指南](https://github.com/grafana/grafana/blob/main/contribute/backend/database.md)、[Memos 的迁移器](https://github.com/usememos/memos/blob/main/store/migrator.go)、[Go 模块布局](https://go.dev/doc/modules/layout)及 [GORM](https://gorm.io/docs/transactions.html)。参考的是组织方式和已验证能力，不把项目特有抽象照搬。

| 方面 | 实际观察 | 重构前的差距与本轮处理 |
|---|---|---|
| 路由 | Gitea 集中注册公开/内部路由并按功能分组；认证中间件属于相应入口 | IAM 已分组，但 router 的大型 Handlers 结构与 transport 绑定映射重复，评估是否按业务拆成少量清晰的注册单元；保留独立内部入口及认证 |
| 数据访问 | Gitea 的 XORM 有事务、模型及 SQL 访问；Grafana 使用 SQLStore 和 XORM；均不是完全消除 SQL | transport/application 仍散落大量直接 SQL；改为业务仓储，禁止 handler 自行组 SQL 或直接访问连接池 |
| 迁移 | Gitea `modelmigration` 按版本维护并通过迁移入口加锁；Grafana 用独立迁移器及版本记录；Memos 嵌入并执行增量文件 | 已有 Goose、PG 会话锁及版本表；保留成熟组件，移动目录时同步 embed、容器构建与版本记录 |
| HTTP | Gitea 的 API 存在直接返回资源的实现，Grafana 有集中响应类型 | REST 不强制 Result；统一信封是我们的平台约定，必须落实到 OpenAPI、所有服务和 Web，而不是只加 Go struct |
| 配置/缓存 | 服务配置按实际依赖组织；Go 标准布局允许 internal 和 cmd 分层 | `db` 与 `redis` 命名不对称，cache 能表达能力，但 Redis 限流需要原子计数/过期，不应伪装成任意内存缓存都等价 |
| 可维护性 | 按功能拆分与清晰边界比复制目录层数重要 | `serve`、长 SQL、处理器与 DTO 耦合影响阅读；需要按实际能力拆分，中文注释说明约束，函数间空一行 |

## 跨语言 HTTP 契约

权威来源放 `src/contracts/`：`common/error-codes.json` 维护 code、默认公开 message、HTTP status 与归属；`common/` 下的 OpenAPI schema 定义 Result。模块 OpenAPI 引用这些定义；工具生成的 Go/TypeScript 常量是产物，不另立权威。公共协议说明记录字段和恢复语义，避免在多个 README 中复制注册表。Go 使用 `src/packages/go/result` 提供成功/失败封装，`httpx` 负责解码与协议边界，`httpx/middleware` 负责公共中间件；其他语言各自实现同一 wire contract，不共享 Go 类型，也不把业务错误处理全塞到共享包。用契约测试约束各语言实现。

普通 JSON 响应：

```json
{"code":"ok","message":"成功","data":{"id":"..."},"request_id":"..."}
```

```json
{"code":"permission_denied","message":"没有权限执行此操作","data":null,"request_id":"..."}
```

- `code` 是稳定字符串：成功 `ok`，通用错误 `invalid_request`、`unauthenticated`、`permission_denied`、`not_found`、`conflict`、`rate_limited`、`internal_error`；确需业务区分时以 `iam.`、`console.` 加具体含义。统一注册表列 HTTP 状态、默认安全消息及归属，不依赖 message 解析；外部接入的协议规范可另规定映射。
- `code` 与 HTTP 状态必须一致，契约测试拒绝“200 + 错误码”等冲突组合；成功的空列表用 `[]`，失败的 `data` 为 null，不能因 omitempty 丢掉协议要求字段。验证未知 code、非 JSON 响应及失败响应格式错误时，客户端给出安全通用错误并保留 request_id，不能把失败解析成成功。
- `message` 面向用户，可本地化，不含密码、token、SQL、堆栈或内部地址。`request_id` 用于日志关联，服务端生成/校验；不因上游传值而直接信任。`data` 为业务对象、数组或 null。列表 `data` 内统一为 `items`、`total`、`limit`、`offset`，计数与过滤口径明确。
- 常规 CRUD：GET 200；创建 201 并返回资源或 ID，适当提供 Location；更新 200 返回资源，删除可选 204 无响应体。错误按照 400/401/403/404/409/429/500/503 等真实状态映射；429 保留 Retry-After，401/403 按认证协议处理。参见 [HTTP 语义 RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html)。
- 204、HEAD、文件下载、SSE、WebSocket、健康探针及与第三方约定的协议不强塞 JSON 包装；OpenAPI 明确例外。内部服务 JSON 在适用时同样使用 Result，内部认证与权限决策不能因为信封改变而削弱。
- 无效 JSON/未知字段/请求过大走统一安全错误，限制长度、Content-Type、分页和排序白名单。请求边界统一拥有解码及结构校验：现有 contractValidation 与 Decode 在统一请求绑定入口内明确分工，不各自维护同一字段的校验规则；向内传已校验的类型化输入。沿用现有库的必要解码，不为追求一次内部解析另造解析器。应用层只校验业务状态、组织范围、权限和事务不变量，不重复验证 schema 已保证的形状。敏感请求保留 CSRF、来源校验、Cookie 安全属性、限流和账户枚举防护。跨域默认关闭；需要启用时再设计显式来源白名单、凭证、预检与 CSRF 的联动。
- 边界统一将领域错误映射为状态码与 code，内部日志保留带上下文的错误链；禁止业务代码自己反复拼 code/message，也不要把所有 error 都归为 500。Go helper 泛型可用时使用 `Result[T]`，OpenAPI 用可复用 envelope 组合具体 data schema，验证生成器实际支持后再决定代码生成方式；Web 基于同一契约生成类型，fetch/axios 封装处理 Result、401、取消和网络异常。
- RFC 9457 是错误格式的另一正式选项，但不是成功结果信封。因需求明确需要 Result，选择统一 Result；对外必须兼容 Problem Details 的接口单独适配，不虚称 Result 是 REST 标准。

### 错误码及请求约定

| HTTP | 通用 code | 典型情况 |
|---|---|---|
| 400 | `invalid_request` | JSON、字段、分页或参数不符合契约 |
| 401 | `unauthenticated` | 缺少、过期或已撤销的会话 |
| 403 | `permission_denied` | 身份有效但不允许操作；CSRF 失败使用明确安全错误码 |
| 404 | `not_found` | 不存在或不应暴露存在性的资源 |
| 409 | `conflict` | 版本变化、唯一约束或资源仍被引用 |
| 429 | `rate_limited` | 命中频率限制，返回 `Retry-After` |
| 500 | `internal_error` | 无法安全公开的内部异常 |
| 503 | `service_unavailable` | 依赖暂时不可用 |

登录凭据错误、主动改密和密码再次确认等现有 IAM 场景有独立码，实施时把现有已用码逐个列入契约，不把不同恢复方式压成同一种提示。错误码注册表须声明稳定名称、模块、HTTP 状态和默认消息；前端凭 code 作交互决策，不能匹配 message。错误信息的可公开性在服务端决定。输入日期时间使用 RFC 3339 和明确时区，ID 与大整数遵照 OpenAPI 的字符串定义，新增字段/可选字段按契约检查；服务间 request_id 需要可信入口生成或校验后传递。

## 模块目录与依赖

推荐单程序模块直接 `cmd/main.go`，不是强制所有项目都这样。Go 编译的是目录，`cmd/console/main.go` 对多命令有价值；当前每个模块各有一个 main，`src/apps/console/cmd/main.go` 更短，IAM 当前命令也由一个程序的子命令实现，可同样简化。若未来有两个独立二进制再采用 `cmd/<command>/main.go`，具体以真实入口决定。`go build ./src/apps/console/cmd` 等脚本、Docker、测试一起更新；不保留并行旧入口。

```text
src/
  apps/
    iam/
      cmd/main.go                 # 命令解析与选择启动/维护入口
      config.yaml
      migrations/                 # 按版本编号的 schema 及数据迁移
        embed.go                  # 与 SQL 同目录嵌入，Go embed 不支持 ..
        00001_identity.sql
      internal/
        config/                   # 只解析、整理与校验配置，不创建连接
        bootstrap/                # 依赖创建、组装与关闭
        middleware/               # IAM 的 Session、CSRF、契约与内部身份
        router/                   # 按身份、组织、角色等注册路由
        handler/                  # 认证和账号请求绑定、统一错误映射
          management/             # 用户、组织、成员、角色和审计接口
        service/                  # 认证、账号、组织、角色等用例和事务
        model/                    # 多处使用的持久化模型；局部类型可留仓储内
        repository/               # GORM 数据访问及少量必要 SQL
        authentication/           # 认证和 Session 适配
        authorization/            # 权限规则与范围判断
      README.md
    console/
      cmd/main.go
      config.yaml
      internal/config/
      internal/router/
      internal/handler/
      internal/service/           # 确有用例时添加
      README.md
    web/                          # 现有 React 应用
  contracts/                      # OpenAPI、错误码、跨语言测试夹具
  packages/go/
    result/                       # Success/Created/NoContent/Fail 与公开消息覆盖
    httpx/                        # 请求绑定与路由协议边界
      middleware/                 # 请求标识、异常恢复和访问日志
    cache/
      key/                        # 平台 key 模板；按模块文件而非一个巨型文件
        iam.go
      # 连接配置/实现仅在实际复用时添加，不预建 redis 适配层
    logging/
    configfile/
    iamclient/
```

目录为职责示意，不要求每个模块配齐所有包。Console 的简单 IAM 调用可直接由 handler 使用客户端，不添加只透传的 service；局部模型可与仓储放在一起，不为每张表配一组 interface/struct。`handler`、`service`、`repository` 按用户、组织、角色等实际功能拆文件或独立业务子包；接口按当前调用及依赖隔离需要定义，不以实现数量或未来复用作机械判断。IAM 安全逻辑现有的 authentication/authorization 可保留为子包，不强制合并为万能 security。模块的 migrations 可见且归模块所有；它们放 internal 在 Go 中合法，只是当前层次较深，问题是可发现性而非不规范。生成物仍以可再生源码为准。

## 依赖方向与组件边界

根目录保留一个 `go.mod/go.sum` 管理当前 Go 服务与共享包；前端沿用 pnpm workspace 和锁文件。当前不为每个服务制造独立 Go module、私有版本发布或 go.work。根脚本用于组合运行，模块仍有直接 build/run/test 的入口；配置中的地址属于运行部署，不影响 import 方向。

| 位置 | 可以依赖 | 不应承担 |
|---|---|---|
| cmd | 配置加载、模块启动入口 | 业务规则、连接创建 |
| bootstrap | 本模块实现、共享基础组件 | 业务规则、运行时全局依赖查找 |
| config | 配置文件/环境解析与类型校验 | 连接创建、迁移、网络 I/O |
| router | handler、中间件和必要契约 | 查询数据库、复制一份权限策略 |
| handler | service、请求/响应 DTO、Result 与 HTTP 基础组件 | 开事务、拼 SQL、返回数据库模型 |
| service | 本模块业务类型、仓储与事务能力、必要认证/授权组件 | http.ResponseWriter、读取环境变量、跨服务私表访问 |
| repository | 本模块持久化模型、GORM/驱动、事务上下文 | HTTP 状态码、Cookie、客户端 message |
| packages/go | 标准库、成熟第三方组件、公开契约 | import apps 的 internal、跨服务业务流程或万能容器 |
| contracts | 协议 schema、错误码及生成类型 | ORM、服务实现、部署凭据 |

上表是职责约束，不要求机械地创建接口。服务只有一个实现且无独立边界时直接传具体类型；真实外部依赖、事务能力确需隔离时定义调用方使用的窄接口。不以“方便 Mock”或单纯可测试性为由给每个 struct 添加 interface。仓储按实际用例设计，禁止建设 `BaseRepository[T]` 式全功能 CRUD 框架。ORM 类型留在仓储/事务适配处，业务请求 DTO 不携带数据库连接或 ORM model。

`internal` 仍用于 Go 可见性隔离。跨模块使用公开 HTTP 契约和客户端，不以共用数据库账号为理由读取其他模块的表。表归属明确；共享库中的 db/cache helper 只管理连接、生命周期和实际共用机制，不集中维护所有模块的模型及业务查询。

### 组件选择与接入规则

| 能力 | 本轮建议 | 使用边界 |
|---|---|---|
| HTTP 与路由 | `net/http` + `chi/v5`，先验证 Route/Group、404/405 和现有安全链 | 保持标准 Handler 签名，成熟 router 负责路径分组，不再手写一套前缀路由框架；当前标准库实现作为行为基线 |
| 请求/响应契约 | 沿用 OpenAPI、oapi-codegen、openapi-typescript 和实际验证器 | 验证组合 schema 和跨文件引用；一个权威 schema，不在 Go helper 和生成 DTO 中各自决定字段 |
| 普通 ORM | GORM PostgreSQL，试点通过后推广 | 显式事务/仓储；复杂 SQL 例外集中，ORM 不承担鉴权 |
| 升级迁移 | 继续 Goose | 版本、锁、失败恢复与数据验证，不另写迁移框架 |
| 认证、会话与 CSRF | 保留现有 Authboss、SCS、CSRF 适配 | 先验证 ORM 改造后的事务/连接与响应提交，不顺便更换身份体系 |
| 缓存/限流 | 继续 go-redis 与现有原子操作 | `cache` 对外表达能力，Redis 实现负责其特有语义 |
| 日志 | 继续 slog 与已引入的轮转组件 | 服务标识、请求标识及脱敏统一，不增加并行日志系统 |
| 跨域 | 默认不开启；需要时接成熟 net/http CORS 中间件 | 仅允许显式来源/方法/头，凭证和 CSRF 联动；不靠通配符“修复”前端报错 |

参考 [chi](https://github.com/go-chi/chi) 和 [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)。chi/GORM 及其接入方式仍需试点；其余明确保留的组件已存在于当前实现。不能将候选方案写成已安装或已实现的结论。选型必须减少当前自制代码或重复维护，检查许可、维护状态、锁定版本和实际 API；不能因为“大型项目使用”就批量引入其所有依赖。

路径分组后，现有基于 `mux.HandleFunc` 正则的契约检查不能继续作为唯一依据。采用 router 的实际路由遍历（如 [chi.Walk](https://pkg.go.dev/github.com/go-chi/chi/v5#Walk)）导出方法/完整路径，与 OpenAPI 逐项对比，并对 404/405/路径参数做请求测试；重复、漏注或未挂载路由都必须被发现。

## 启动装配与生命周期

- 一个组件有明确的创建者和关闭者。按配置校验 → 日志 → 本模块实际依赖 → 用例/仓储 → 路由/中间件 → 监听的顺序组装；中间失败则逆序释放已创建资源。没有实际使用数据库/Redis 的 Console 不增加占位连接。
- 进程收到终止信号后先停止接收请求，允许在途请求在期限内结束，最后关闭客户端、连接池及日志；关闭过程的 context 不直接复用已取消的进程 context。后台 goroutine 必须可退出，错误由拥有者接收，不启动无主任务。
- 配置启动时加载、类型化校验；运行期修改默认需要重启，Web 构建变量与后端运行变量分开说明。不预建热配置/插件系统。当前只有 PostgreSQL/Redis，不增加 driver 选择器、驱动注册表或插件结构；出现第二个真实后端需求时再确定配置与适配方式。
- PostgreSQL 的 pool、GORM 的 database/sql、SCS store 和 Goose 的关系在试点中明确：哪个创建池、哪个使用、哪个负责关闭。业务审计和业务修改必须共享事务；SCS 的独立持久化若无法加入同一事务，须分别说明失败后数据库状态、Cookie、会话可用性及恢复方式。响应失败不等于业务已回滚；不为此预建跨组件事务框架，也不能声称“一个池”就代表“一个事务”。
- 普通出站客户端设置超时、连接复用、响应大小限制、重定向和 TLS 策略。服务间认证与用户会话各自校验；默认不自动重试写操作。可重试只针对明确可重放的操作、有限次数并受总超时限制，不在本轮预建通用熔断框架。
- 健康状态区分进程存活与业务就绪；只有依赖及 schema 符合要求才能就绪。健康/指标不返回配置或密钥，内部接口不通过公网代理开放。监控实现先复用结构化日志和实际运行需求，不预部署额外平台。

## 中间件与响应提交

设计清楚执行顺序，不能只把组件名列出来。公开普通 JSON 请求建议为：

```text
请求标识与访问日志
  → 异常恢复
  → 请求预算/方法与 body 限制、必要响应头
  → 已启用且明确配置的跨域预检
  → Session 加载
  → Origin / CSRF 校验
  → 契约与字段验证
  → handler
  → service 内鉴权及数据范围校验
  → 仓储事务与审计
  → Session 持久化成功
  → 提交响应
```

具体顺序以现有认证适配器试点和失败测试校验。异常恢复在 Session/事务外层时，内部错误必须沿错误路径退出而不能被当成功提交；不把 recover 吞掉后继续写入。登录/注销、CSRF context 和健康接口的例外须在路由定义中可见，不用路径 contains/字符串猜测跳过认证。

内部入口先检查 TLS 与服务凭证和动作白名单，再按原请求会话/来源验证用户权限；不能把内部 token 当用户授权。业务 service 保持最终鉴权，前端显隐和路由中间件都不能替代它。

SSE 使用独立超时与写入策略，保持 Flush/取消能力及权限定时复核；不能经过缓存整个响应体的 JSON wrapper，不能统一设置短 WriteTimeout 导致正常流式请求断开。响应开始后发生错误以协议允许的终止事件或关闭连接处理，不能再补写 Result。共享中间件若包装 ResponseWriter，必须保留相应能力，且不未经需要实现一套通用流协议。

Result 边界覆盖普通 API 的 404/405（405 保留 Allow）、413、415、绑定失败和未处理异常；反向代理生成的错误也在 Web 客户端按非 JSON/网络错误处理。先编码再提交普通 JSON 的状态和响应头，编码失败不能留下 200 成功；已提交后断连只记录一次，不重写响应。日志记录最终状态、耗时和 request_id，默认不记录请求体、Cookie 和 Authorization。

## ORM 与迁移决策

推荐先以 IAM 一条有代表性的用户/角色用例试点 **GORM + PostgreSQL 驱动**，保留 Goose：GORM 对常规 CRUD、显式事务、行锁和参数绑定有成熟支持；与当前 pgx/SCS/Goose 如何共享连接池、传递同一事务、关闭资源必须在试点实测，不预先断言实现细节。对比 Ent（类型化与代码生成较强，但 schema/codegen 引入新构建链）和保留现有 sqlc（编译期 SQL 类型明确但仍要维护 SQL）；如果 GORM 不能表达现有锁和事务，保留受控原生 SQL 或重新评估。参考 [GORM 事务](https://gorm.io/docs/transactions.html)、[行锁](https://gorm.io/docs/advanced_query.html)、[原生 SQL](https://gorm.io/docs/sql_builder.html)、[sqlc](https://docs.sqlc.dev/en/latest/)。

- ORM 模型明确 schema、表、主外键、类型和 nullability；接口 DTO 与模型分离。禁止把密码哈希直接序列化；禁止动态拼用户输入；分页及排序白名单，避免 N+1。对更新计数、冲突、死锁、取消与事务回滚做真实 PostgreSQL 测试。
- 使用版本化迁移，不以 `AutoMigrate` 作为升级事实来源。已执行过的 00001 及 schema、触发器、唯一/复合外键、锁语义作为基线保留；对已部署数据库追加新迁移而不是改旧文件或重建库。若确认从未对外使用，可在实施前重新约定初始基线，但必须先审计本机/部署数据。
- 升级方案本轮建议采用维护窗口：停止本项目服务 → 备份 → 显式执行一次迁移 → 检查版本 → 启动并验证；这是方案建议，不表示已执行或已获部署授权。每个新增迁移说明前置条件、是否可事务化及失败恢复；不修改已执行脚本。需要不可逆变更时明确恢复步骤，不能以回退二进制代替数据恢复。当前没有滚动升级要求，不建设新旧版本双写或 expand/contract 流程。
- 验收复用现有空库初始化、带数据升级、幂等/迁移锁及版本不匹配检查，并保护认证/会话/管理员/审计数据；新增脚本有非事务 DDL 或不可逆数据变更时，才增加相应中断/恢复验证，不为了覆盖所有理论分支建立通用故障平台。数据库实例创建是部署职责；自动创建库仅在明确支持和账号权限满足时设计，不默认要求超级用户。

## 业务规则与事务验收

重构沿用当前产品规则，不以 ORM、目录或 Result 改造为由重做身份产品。以下矩阵是最低验收，不是新增功能列表：

| 当前能力 | 必须保住的行为 | 典型验证 |
|---|---|---|
| 初始管理员与组织 | 空库幂等初始化；已有用户不被提权或重置密码 | 重复/并发初始化、已有普通账号库异常 |
| 登录与改密 | 密码哈希和策略、密码管理、限流与锁定；无明文凭证泄漏 | 错误凭据、并发失败、改密撤销会话 |
| Session | 绝对/空闲到期、轮换与撤销、跨实例一致性；持久化失败不发成功 Cookie | 过期边界、双实例、失败提交/补偿 |
| 组织成员 | 组织与成员状态、跨组织隔离、移除/禁用后及时失效 | ID 替换越权、状态变更与并发请求 |
| 角色权限 | 系统角色保护、自定义角色、可委派范围、版本冲突、预览不授予额外能力 | 低权编辑高权角色、并发分配/撤销 |
| 管理员保护 | 不能删除/禁用最后有效管理员；事务内重查 | 两个并发变更不能同时通过 |
| 审计 | 业务变化与审计一致提交，审计失败导致修改回滚 | 事务失败、敏感字段不进入 metadata |
| 服务接入 | Console 依据当前身份和真实组织范围访问；SSE 撤销时终止 | 两个 IAM/Console 实例、失效会话、撤销流 |

事务能力优先封装在模块局部的仓储 Store/事务执行器中，向用例提供绑定到同一事务的仓储能力；不把 `*gorm.DB` 塞入请求 DTO/context，也不让业务函数逐层创建新事务。接口形状在试点中以实际涉及的仓储为准，不建立跨服务通用 UnitOfWork 框架。

每个写用例明确授权、锁定次序、读取、条件更新、审计、提交的顺序；仓储收到同一个事务，不能在方法里悄悄回到根连接。GORM 更新零值、NULL、RowsAffected、record-not-found、唯一冲突等语义逐项映射；死锁/序列化重试只能在可安全重放的完整事务边界进行，不能只重试其中一条写入。技术校验不代替组织约束，数据库约束继续作为最后保护。

当前只做已有能力授权和组织范围判断。为未来资源授权保留清楚的输入/输出边界；不提前建设组织记忆、skill/agent 资源模型、ABAC 或新的策略语言。

## 前端接入与共享状态

- Web 一个 HTTP 客户端解析 Result/非 JSON/网络错误，页面消费具体 DTO；message 用于展示，code 用于恢复动作，验证错误按结构映射到字段，不在组件里再解析错误字符串。
- 不把 Cookie Session 转存 localStorage 或复制到另一个 token 体系。401 统一使身份状态失效并处理跨标签退出；403 不误判为退出，密码再次确认和主动改密按各自 code 跳转。并发请求失败不能重复弹大量通知。
- 组织范围数据的缓存键包含组织 ID 和必要用户上下文；组织切换、退出、权限变化时取消/废弃旧请求并清理相关缓存，防止过期响应覆盖当前状态。后台结果和本地 UI 状态区分，沿用现有可用工具，不为重构盲目换状态库。
- 业务权限显示沿用后端能力投影和统一检查入口；前端路由、按钮和后端最终鉴权分层配合。管理中心、账号设置及普通业务导航保留当前约定，不借工程重构新增业务页面。

## Cache 命名及 key

`cache` 表示平台缓存配置，本轮将现有 `redis` 分组改为 `cache`，直接保留 host、port、password、db、prefix、url；不增加 driver 字段或多一层 redis 分组，当前仍明确使用 Redis。环境变量继续明确技术 `SPX_REDIS_*`，若改名须一次性同步配置、部署和文档。用户指定的 `SPX_POSTGRES_MODELESS` 保留为当前环境变量，驱动 DSN 参数仍是 `sslmode`。

`src/packages/go/cache/key/iam.go` 集中 IAM 实际 key 模板，其他模块真实出现时建立对应文件；模板直接写 `spx:iam:ratelimit:...`，前缀固定为 spx，不提供覆盖或双重拼接。保持现有 identifier 处理与 TTL 语义；用针对性单元测试检查模板、固定前缀及实际输入隔离。只有发现具体碰撞或敏感信息泄漏路径时才调整编码，不新增通用 key 编码协议。不把仅供 IAM 使用的模板抽象成跨模块万能枚举；也不把 Redis 原子脚本/分布式会话能力称为通用 cache 接口。未来更换后端时再验证其原子性、TTL 和失效语义，本轮不提前制作替代后端接口或测试。

## 非目标和验收

不创建未来业务模块、额外认证服务或自制 ORM/迁移框架；不为美观改协议、掩盖编译错误或扩大公开访问。最终验收以实际挂载路由与契约一致、核心身份与授权链路保持、数据升级和项目启停可用为准；各行为按 Plan 指定的最低必要测试层验证，不将每个路由重复写成 E2E。通过测试不等于授权提交、推送或部署。

## 工程质量与验收产物

- 工具链、依赖、生成器与锁文件固定；变更依赖要检查维护、许可证、迁移说明及测试，不批量升级无关依赖。CI 包含前端格式检查、typecheck、Go format/vet/test、契约生成无漂移、真实集成、核心 E2E 和构建；不通过关闭检查消除错误。GoLand/编辑器 SDK 与真实 go.mod 工具链另核对，IDE 红线与编译失败不能混为同一结论。
- 方法之间空一行；函数内部按初始化、依赖装配、执行、关闭等完整步骤留白。多行 struct 字面量每个字段独立一行；长调用、复杂条件和 SQL 按语义换行，禁止多个无关语句用分号塞在一行。Go 短生命周期变量保留惯用名；跨阶段变量使用表达业务的名字。注释说明约束/原因，不能用逐行解释弥补差的命名。
- 规范中附 [逻辑分段示例](code-style.md)。格式使用 gofmt/goimports，生成物使用现有生成一致性检查；方法空行、职责与依赖方向由针对性审查覆盖，不为此强制新增正则扫描器或自制 linter。
- 每阶段在 Plan 记录决定、变更范围、实际验证和剩余项；仅更新发生变化的模块说明，不要求每阶段再产一套目录图或重复报告。阶段内允许同步更新调用者，但不保留永久双栈 Result 或两套 ORM 来规避迁移。
- 当前“40 条路由”只是审计基线；验收以规范化后的 OpenAPI 和实际路由清单为准，不能靠固定数量证明正确。文档/技能分清当前实现、目标方案和仍待试点的决定；最终模块 README 维护已落地设计。
