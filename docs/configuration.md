# 配置与启动

## 配置文件

| 文件 | 内容 |
|---|---|
| 根`.env.example` → `.env` | 一键启动配置，按Web、IAM、Console、PostgreSQL、Redis分区 |
| `src/apps/iam/config.yaml` | IAM监听、数据库、Redis、认证策略、日志 |
| `src/apps/console/config.yaml` | Console监听、IAM连接、日志 |
| `src/apps/web/.env.example` → `.env.local` | Web独立运行的Next.js配置 |

Go直接加载config.yaml，不需要再复制example。实际.env、certs和logs目录不提交。

## PostgreSQL：默认一个实例、一个库、一组账号

全局只维护`SPX_POSTGRES_HOST/PORT/DATABASE/USER/PASSWORD/MODELESS`。默认数据库账号/库名为 `secplexus`，密码为 `secplexus`。模块的db字段默认引用这组值；应用运行、迁移、维护都使用同一连接，不创建模块用户或另一套运维账号。

账号需要具备本项目schema和版本表的建表、迁移权限。应用层的鉴权与审计事务保持不变；不再依靠单独数据库账号限制HTTP进程的DDL权限。

同一数据库中的表仍由对应模块管理。Console当前通过IAM接口取得身份信息，没有数据库查询，因此不添加无用的db配置。

需要让模块连接独立实例或使用其他账号时，直接修改该模块config.yaml，例如把db.host、db.user改为该连接的值，把password改为引用该模块自己的密码变量。其他模块继续使用全局默认值，无需改代码或增加配置层。

完整DSN只作为高级覆盖`SPX_POSTGRES_URL`保留，用于特殊驱动参数；根示例不要求填写它。

## Redis：统一连接与key模板

连接使用`SPX_REDIS_HOST/PORT/USER/PASSWORD/DB`，默认密码为 `secplexus`。用户名默认 `default`（没有 root 账号），可通过 `cache.user` 或 `SPX_REDIS_USER` 修改。外部 Redis 应预先创建该 ACL 用户并授予应用所需权限；普通应用启动不会改外部用户。完整地址仍可通过 `SPX_REDIS_URL=redis://用户名:密码@主机:端口/库号` 指定，URL 中的特殊字符须编码；key 前缀固定为 spx，不提供配置覆盖。

[cache/key](../src/packages/go/cache/key/iam.go)集中定义已实现的类型化模板，调用Format构建key，不在业务代码中散落字符串拼接：

```text
spx:iam:ratelimit:ip:<sha256>
spx:iam:ratelimit:account:<sha256>
```

前缀、模块和业务名由模板固定。以后新增实际业务时增加对应模板，不预建尚不存在的key。不同平台部署可使用独立Redis库；同平台副本共用连接及前缀。

## 服务地址只配置一次

服务监听用host和整数port。`0.0.0.0`表示绑定所有网卡，不是客户端访问地址。

根启动脚本通过`--print-endpoints`读取Go实际解析的监听地址，再生成Web代理与Console→IAM的连接URL；通配绑定会转换为本机回环地址。修改模块监听端口后，不用再改几处代理URL。

`SPX_PUBLIC_URL`是浏览器实际访问地址，用于Cookie和CSRF。HTTP服务间使用完整URL包含协议是必要的；只在独立/远程部署需要显式指定：

| 可选覆盖 | 使用方 |
|---|---|
| SPX_WEB_IAM_PROXY_TARGET | Web独立连接IAM |
| SPX_WEB_CONSOLE_PROXY_TARGET | Web独立连接Console |
| SPX_CONSOLE_IAM_BASE_URL | Console连接远程IAM，必须HTTPS |

Compose使用服务名连接，Next.js公开API代理地址由同一组模块端口变量生成，不重复写固定后端地址。PG/Redis容器内部使用固定协议端口，根端口变量控制宿主机映射；不会根据连接是否为空自动部署组件。

## 环境变量与模块覆盖

```yaml
db:
  host: "${SPX_POSTGRES_HOST:127.0.0.1}"
  port: "${SPX_POSTGRES_PORT:5432}"
  name: "${SPX_POSTGRES_DATABASE:secplexus}"
  user: "${SPX_POSTGRES_USER:secplexus}"
  password: "${SPX_POSTGRES_PASSWORD:secplexus}"
```

`${NAME:default}`优先读取进程环境，未设置时使用默认值；显式空值保留。根入口先按“进程环境 > 根.env”合并。模块配置中的字面值不会被隐式的环境映射再次覆盖，因此可以直接修改某个模块的连接。

只替换YAML标量，不执行shell，也不把密码中的换行、冒号或美元符号当成配置语法。未知字段、重复键、额外文档和非法类型明确报错，错误不打印密码或密钥。修改配置后重启。

## 运行

需要Go 1.27.3、Node.js 22.12+（建议24）、pnpm 10.32.1。

```sh
cp .env.example .env
# 填写数据库密码、初始管理员密码、CSRF密钥和Console调用凭证
pnpm certs:generate
# 使用已有PG/Redis，或显式部署基础组件：
pnpm compose:up postgres redis
make dev
```

`make dev`不部署基础组件，按配置连接；先用同一数据库连接执行迁移/幂等初始化，再启动IAM、Console和Web。初始账号默认 `admin/secplexus`，可用 `SPX_IAM_BOOTSTRAP_PASSWORD` 修改初始化密码；只在空账号库创建，不覆盖已有账号。普通密码设置要求 8–128 个字符，初始化管理员使用同一规则。

本地开发用 PostgreSQL、Redis 是持久资源：`pnpm compose:up postgres redis` 创建或启动同一套容器，数据使用命名卷保存；以后可在 Docker Desktop 中直接启动已有容器，也可重复执行该命令。`make stop` 只停止它们，不删除容器、卷或账号数据。普通启动、停止和测试清理都不能对这套开发资源执行 `down -v`、`rm -v` 或全局 prune；自动化测试另用临时容器，并仅清理本次测试资源。

| 命令 | 行为 |
|---|---|
| make dev | 迁移、启动三个应用，Go/React代码热更新 |
| make build | 构建Go二进制到bin/、Web到模块.next/；可无.env，存在时加载公开Web配置 |
| pnpm preview | 启动后端并预览已构建Web，公共地址需与实际访问地址一致 |
| pnpm compose:up | 显式构建并部署整套服务 |
| make stop | 停当前工作区应用/容器，保留数据，无需.env |

所有统一启动入口都先检查.env：缺失则生成并提示检查，不覆盖已有文件。开发编译使用系统临时目录，正常退出后清理，不创建.local。

模块独立运行不自动读取根.env，由进程环境提供密码等值：

```sh
go run ./src/apps/iam/cmd migrate --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd serve --config src/apps/iam/config.yaml
go run ./src/apps/console/cmd --config src/apps/console/config.yaml
```

维护、恢复与迁移使用相同db连接。恢复备份后先运行`revoke-after-restore`，再开放流量；不自动down迁移或清卷。

Web独立运行使用Next.js原生.env.example→.env.local；NEXT_PUBLIC_*不能放密码、密钥或调用凭证。省略Go的--config时读取工作目录config.yaml；二进制部署可以显式指定配置。

## 日志和TLS文件

本机从仓库根运行时统一写logs/iam.log、logs/console.log；slog输出JSON，Lumberjack按模块配置轮转，默认info、50MB、5份、14天压缩。多副本不能共写一个轮转文件。

相对文件路径以进程工作目录为准；二进制部署设置WorkingDirectory或显式日志路径。容器默认stdout，由Docker local驱动按10MB、3份轮转，需要文件日志时显式挂载可写目录。

只有TLS证书和私钥使用certs文件；数据库连接、密码和调用凭证直接从环境读取，不另建连接/密码文件。.env中的文件路径由根启动脚本转换为绝对路径。证书生成保留已有文件，不覆盖部署证书。

## 部署命名

Compose默认使用工作区项目名，也支持原生COMPOSE_PROJECT_NAME覆盖。不同项目名使用不同数据卷；已有部署应保留原名或明确迁移数据。停止不删卷、不停止外部数据库，不增加应用instance_id。

公开参考：[Kratos结构化配置](https://github.com/go-kratos/kratos-layout/blob/main/configs/config.yaml)、[Next.js环境配置](https://nextjs.org/docs/app/guides/environment-variables)、[Lumberjack](https://github.com/natefinch/lumberjack/blob/v2.2.1/README.md)。具体字段以本项目模块配置为准。

## 浏览器访问和跨域

Web 与 API 默认通过同一个入口访问。IAM 根据 `app.public_url` 校验请求来源，并使用 Cookie 与 CSRF 令牌保护写操作。公开入口配置为 localhost、127.0.0.1 或 ::1 时，接受相同协议和端口的明确回环别名；可分别使用 localhost 和 127.0.0.1 登录，Cookie/会话仍由浏览器按主机隔离。非回环入口只接受配置中的来源，其他端口、协议或主机不在兼容范围。当前没有开放通用跨域白名单或跨域凭证访问；如需跨域部署，必须同时设计可信来源、Cookie、CSRF 和代理策略，不能只加允许所有来源的响应头。


## 配置与启动代码的职责

模块的 `internal/config` 只做加载、环境展开、类型转换和校验，以及依据字段构造连接地址；不会访问数据库、创建 Redis/HTTP 客户端或执行迁移。`internal/bootstrap` 显式创建依赖、挂载路由并负责失败清理和退出关闭；数据库操作由 repository 承担。

IAM 缓存分组为 `cache`，字段为 host、port、password、db、url；当前实现仍是 Redis，环境变量继续使用 `SPX_REDIS_*`，key 前缀在代码中固定为 spx。不增加占位 driver 或嵌套 redis 分组。PostgreSQL 环境变量保留 `SPX_POSTGRES_MODELESS`，驱动连接参数仍是 `sslmode`。升级需修改配置分组名时保留原连接值，不覆盖用户 `.env`。

## Next.js Web

根入口使用 `SPX_WEB_HOST` / `SPX_WEB_PORT` 传递 Next CLI 的监听参数；模块独立运行保留上游默认端口 3000。`pnpm preview` 使用已构建的 `next start`，不是 Vite 静态预览，公共 origin 仍需与浏览器地址一致。`make build` 生成模块 `.next/`，容器保留 Node 运行时。

新增 Route Handler 只代理 `/api/v1/iam/...` 与 `/api/v1/console/...`，服务端配置模块在加载时读取 `SPX_WEB_IAM_PROXY_TARGET` / `SPX_WEB_CONSOLE_PROXY_TARGET`；默认本机8081/8080，统一启动时按实际Go端口注入。它保留 Cookie、CSRF、Origin、状态码、Set-Cookie 与 SSE 流，不向浏览器开放内部接口或内部凭证。Web 环境不接收数据库、Redis、初始化密码或 Console 内部凭证。

`/` 进入工作空间，未认证时跳转 `/login`。`/dashboard` 是CRM V1安全智驾工作台，受真实IAM认证保护，当前保留示例数据；其余模板页面和演示身份已移除。前端依赖及测试配置由 `src/apps/web/` 维护，版本固定在模块 package.json，根 pnpm-lock.yaml 是唯一锁文件。发布、Git 钩子、外部站点统计和仓库星数配置已移除。

本机开发默认监听 `127.0.0.1`。若保留旧的通配监听值，而 `SPX_PUBLIC_URL` 使用本机回环主机，根脚本会将开发监听对齐到该回环主机，满足 Next 原生 HMR 来源检查。预览/生产仍按配置监听；没有修改上游 allowedDevOrigins 或开放跨域凭证访问。

关键链路的日志覆盖、级别及 request_id 排查方式见 [运行日志与错误定位](observability.md)。

Web 的 `agentRules: false` 关闭框架重复生成模块级 Agent 指令，统一遵循根 AGENTS.md。生产构建保留日志调用，受控错误与生命周期事件按项目可观测性约定输出。元信息的公开 origin 使用 NEXT_PUBLIC_APP_URL；根启动按 SPX_PUBLIC_URL 提供默认值。

根目录 `.env` 对当前 Web 的控制范围如下，进程环境优先于文件值：

| 变量 | 生效范围 |
|---|---|
| `SPX_WEB_HOST` / `SPX_WEB_PORT` | 根入口启动的 Next 开发/预览监听地址 |
| `SPX_PUBLIC_URL` | IAM 的 Cookie/CSRF 来源，同时作为 Web 公开元信息地址的默认值 |
| `SPX_WEB_IAM_PROXY_TARGET` / `SPX_WEB_CONSOLE_PROXY_TARGET` | Next 服务端公开 API 的可选目标覆盖；省略时从 Go 实际监听配置推导 |
| `NEXT_PUBLIC_APP_URL` | 可选公开元信息地址覆盖；省略时复用 `SPX_PUBLIC_URL` |

`make dev`、`make build`、`pnpm preview` 都使用根配置入口。构建只向 Web 传递筛选后的环境，不注入数据库、Redis或内部调用凭证。监听地址与代理目标在重启后生效；`NEXT_PUBLIC_*` 会被 Next.js 在生产构建时固化，修改公开元信息地址后需要重新执行本地构建。单独在 Web 模块执行 `pnpm dev/build/start` 则使用其原生环境加载，不隐式读取根 `.env`。根入口注入的同名环境变量优先于模块环境文件，统一启动时通常无需再创建 Web `.env.local`。

已存在的 PostgreSQL 数据卷不会随 POSTGRES_PASSWORD 环境变量更换用户密码，修改连接密码时须同步修改数据库角色。Redis 的启动密码同样须与客户端配置一致；Compose 保留命名数据卷。

自带 Compose 使用同一组 SPX_REDIS_USER/PASSWORD 设置 Redis ACL，配置自定义用户时停用 default 用户，健康检查也使用该用户。密码为空时明确使用 nopass。ACL 规则通过密码哈希传递，不把明文密码放入 Redis 启动参数；数据仍保存在原命名卷。使用外部 Redis 时不启动该 Compose 服务。

## Web 语言与格式

Web 使用 next-intl，支持 `en`、`zh-CN`，默认英文。语言菜单通过 Server Action 写入独立的 `spx-locale` 偏好 Cookie，服务端请求配置校验后加载对应词典；无效值回退英文，不影响 IAM 会话或权限。语言不改变 URL，也不改变 API 路径。

语言集合、默认值与显示时区集中在 `src/apps/web/src/i18n/config.ts`。界面日期统一按 UTC 展示，语言只改变表示方式；金额币种不随语言变化。文本词典在 Web 的 `messages/`，业务名称和标识不翻译。实现与新增消息约定见 [Web 国际化说明](../src/apps/web/README.md#internationalization)。
