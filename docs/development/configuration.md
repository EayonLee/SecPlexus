# 配置与启动

## 配置文件

| 文件 | 内容 |
|---|---|
| 根`.env.example` → `.env` | 四个应用与自带依赖的一键部署设置 |
| `src/apps/iam/config.yaml` | IAM监听、数据库、Redis、认证策略、日志 |
| `src/apps/console/config.yaml` | Console监听、IAM连接、日志 |
| `src/apps/ingest/config.yaml` | Ingest 监听、共享依赖、数据接入通道、事件主题与写入 |
| `src/apps/web/.env.example` → `.env.local` | Web独立运行的Next.js配置 |

根 `.env.example` 集中列出部署选项，复制为 `.env` 后填写。常用项直接设置，低频项解除注释后覆盖。启动入口合并进程环境和根 `.env`，再按模块和运行角色注入。Go 模块的 `config.yaml` 声明运行默认值。私有 `.env` 和日志保存在本地。

## PostgreSQL 与 Redis

IAM 和 Ingest 共用 PostgreSQL 与 Redis。Compose 按服务名定位自带实例，根模板集中提供自带实例的端口、账号设置及外接实例的标准连接入口。PostgreSQL 的库和 schema 按模块管理，Redis 用键前缀区分模块。Console 调用 IAM，当前无需连接 PostgreSQL 或 Redis。

独立运行连接外部实例时，分别设置 `SPX_POSTGRESQL_URL` 和 `SPX_REDIS_URL`。URL 包含完整的地址、账号、库及 TLS 设置。Compose 自带实例的连接交给编排，外接实例的账号及权限由部署者预设。用户名、密码中的保留字符按 URI 编码。 [PostgreSQL 连接串](https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING) · [Compose 服务发现](https://docs.docker.com/compose/how-tos/networking/#default-network-and-service-discovery)

### 数据库迁移：按模块拆分，三条纪律

迁移 SQL 放在每个模块自己的 `migrations/` 目录（如 `src/apps/iam/migrations/`），随模块二进制经 `go:embed` 发布，由各自模块的 `migrate` 命令执行；不建中央迁移目录或中央迁移器。理由：模块是独立部署单元，Go 的 embed 规则也无法引用包目录之外的文件；GitHub 大型项目（Sourcegraph、Supabase、Temporal）的规律一致——迁移目录的拆分单位是可部署单元/数据库归属，不是「库在同一个实例」。

所有模块共享一个 PostgreSQL，因此迁移必须遵守三条纪律：

1. **版本账本放自己 schema 里**，路径 `<module>.goose_db_version`（IAM 现行 `iam.goose_db_version`）。账本与表结构同生共死：`DROP SCHEMA iam CASCADE` 就是一次完整重置，不会出现「账本记着已应用、schema 却是空的」这种错位；一个模块的迁移历史也不影响其他模块部署。
2. **统一使用 goose Provider + `WithSessionLocker`**。goose 的 advisory lock ID 是固定常量（`crc64("goose")`），所有模块的迁移天然互斥排队，避免并发 DDL；不要绕开锁自造并行迁移。
3. **禁止跨 schema 外键**。模块 A 的迁移不得 `REFERENCES` 模块 B 的表——那会把两个模块的迁移顺序、回滚顺序和表生命周期绑死。跨模块引用存标识符，关系通过对方模块的 API 解析。此规则由 `pnpm catalogs:check` 中的 `scripts/migrations-check.mjs` 机器强制：跨 schema `REFERENCES` 或一个模块触碰多个 schema 都会使检查失败。

根启动按固定顺序逐模块执行 `migrate`（见 `scripts/start.mjs`）；新增模块时在该处登记，且模块间不得有共享库对象的迁移依赖。

## Redis 键模板

共享客户端和键模板位于 [redis](../../src/packages/go/redis)。模板固定为 `spx:<模块>:<业务>:<标识>`，例如 `spx:iam:ratelimit:account:<sha256>`。新增键由所属模块维护。同一部署的副本共用连接与前缀。

## 服务位置、监听与浏览器入口

同机源码启动时使用本机地址，Compose 使用服务名。默认只需配置应用端口。开发入口为 `http://127.0.0.1:<SPX_WEB_PORT>`，端口改变后，IAM 的浏览器可信来源一同派生。通过域名、服务器 IP 或反向代理访问时填写实际的 `SPX_WEB_ORIGIN`，例如 `https://sec.example.com`。

独立部署时，使用 `SPX_<APP>_HOST` 和 `SPX_<APP>_PORT` 配置调用目标。只有明确需要监听其他网卡时才设置 `SPX_<APP>_BIND_HOST`。Go 服务跟随模块 YAML 的监听默认值，Compose 在容器内部指定监听地址。Web 的容器内端口与宿主机发布端口由 Compose 管理。

外部 HTTPS 通常由反向代理终止。独立运行 Ingest 时可以通过 `SPX_INGEST_TLS_CERT_FILE`、`SPX_INGEST_TLS_KEY_FILE` 启用原生 TLS。证书须覆盖就绪检查和 Web 上游使用的地址。私有 CA 可用 `NODE_EXTRA_CA_CERTS`，Web 可用 `SPX_INGEST_ORIGIN=https://<host>:<port>` 指定上游。

同机启动读取实际 Go 监听结果，对未填写的连接位置推导可连接地址；不能将 `0.0.0.0` 用作客户端目标。进程环境、根 `.env` 的明确服务位置保持优先。

`make dev` / `make preview` 默认启动本地全部模块。选择 `MODULES="web"` 时，Web 在运行时连接已有的 IAM、Console 和可选的 Ingest。Ingest 地址可由 `SPX_INGEST_HOST/PORT` 提供，显式 `SPX_INGEST_ORIGIN` 优先。修改 Web 的上游后重启 Web 即可生效。`MODULES="iam"` 或 `MODULES="console"` 单独启动指定后端。

开发 Compose 的四个应用通过服务名连接。全栈使用内置 PostgreSQL、Redis、Kafka 和 ClickHouse。管理员初始化凭据只交给 IAM 初始化任务。仅选择 PostgreSQL 和 Redis 时无需应用凭据。IAM 与 Ingest 分别构建一个镜像，同模块任务共用。直接在空镜像缓存下运行独立的 Ingest 迁移任务时，先使用同一 Compose 项目构建 `event-topics` 服务的镜像。 [开发 Compose 清单](../../deploy/compose/compose.yaml)

## 环境变量

根入口先按“进程环境 > 根.env”合并并过滤到所属模块。根`.env`遵循 dotenv 语义（由 Node `node:util` 的`parseEnv`解析）：**未加引号时，值中的`#`及其后内容被当作注释丢弃**。含`#`、空格或其它特殊字符的值必须用单引号或双引号包裹，例如`SPX_IAM_BOOTSTRAP_PASSWORD="SecPlexus#2026"`；反斜杠转义`\#`不生效。校验错误会报出实际接收到的字符数以暴露此类截断，例如`bootstrap.password must contain a valid 12–128 character administrator password (got 9)`。Go字段优先级为：**进程环境 > 根.env > YAML字面值**；模块 YAML 是默认值的唯一来源，不维护另一份 Viper 默认表。显式空值保留，随后进行类型及业务配置校验。仅 YAML 字段声明的变量参与映射，不自动扫描任意环境键。模块独立运行不读取根.env。


YAML 使用标准参数表达式 `${NAME-default}`（未设置才后备）或 `${NAME:-default}`（未设置或空值后备）。变量名称、优先级与默认值都在字段旁声明；环境标量由 mapstructure 官方 Hook 转换为整数、布尔、列表和时长。未知字段、重复键、额外文档、非法类型与超过64KiB配置在加载边界拒绝，错误不输出秘密值。修改配置后重启。服务调用方是以服务ID为键的`service_clients`映射；凭证和action白名单属于同一配置项，不能以数组顺序猜测调用方。

## 运行

需要Go 1.27.4、Node.js 22.12+（建议24）、pnpm 10.32.1与GNU Make。Make是macOS、Linux和Windows统一的唯一外层入口，配方不依赖POSIX shell；平台差异由Node脚本承担。Windows可用w64devkit、Chocolatey或MSYS2安装GNU Make，并在PowerShell或`cmd.exe`中运行。根`package.json`中的`pnpm`脚本是Make的内部实现，不作为日常公开入口。

```sh
make config      # 首次运行生成.env，然后停止以便检查
# 在根 .env 填写管理员密码、CSRF 签名密钥与各服务凭据。
# 独立运行时填写实际依赖地址。
# 使用已有PG/Redis，或显式部署基础组件：
make compose-up SERVICES="postgresql redis"
make dev
```

`make dev` 先离线校验选中的应用，再连接依赖、迁移并执行首次管理员初始化。托管管理员默认名为 `admin`，口令须显式设置（12–128 个字符）。迁移同步权限目录，重复初始化保留登录信息。显式运行 `iam sync-administrator` 才会应用托管凭据变化，并撤销受影响的会话。

本地开发用 PostgreSQL、Redis 是持久资源：`make compose-up SERVICES="postgresql redis"` 创建或启动同一套容器，数据使用命名卷保存；以后可在 Docker Desktop 中直接启动已有容器，也可重复执行该命令。`make stop`只停止它们，不删除容器、卷或账号数据。普通启动、停止和测试清理都不能对这套开发资源执行 `down -v`、`rm -v` 或全局 prune；自动化测试另用隔离测试资源（临时数据库或显式测试容器），并仅清理本次测试资源。

| 命令 | 行为 |
|---|---|
| make config [MODULES="iam console web"] | 离线检查选中模块，不连接依赖、不迁移、不创建业务日志；省略模块时检查全部 |
| make dev | 迁移、启动四个应用，Go/React代码热更新 |
| make build | 构建Go二进制到bin/、Web到模块.next/；可无.env，存在时加载Web所属配置 |
| make preview | 启动后端并预览已构建Web，公共地址需与实际访问地址一致 |
| make compose-up [SERVICES="postgresql redis"] | 显式构建并部署整套服务 |
| make event-store [ACTION=…] | 使用根配置检查、迁移、核对或运行 Ingest 事件存储，默认执行离线检查 |
| make stop | 停当前工作区应用/容器，保留数据，无需.env |
| make test | 运行脚本、Go与Web单元测试 |
| make test-e2e [ARGS="--docker"] / make integration [ARGS="--docker --race"] | 浏览器 E2E / 集成测试 |

所有统一启动入口都先检查.env：缺失则生成并提示检查，不覆盖已有文件。开发编译使用系统临时目录，正常退出后清理，不创建.local。启动器把本次服务根进程写入仓库内`.spx/run.json`（已被Git忽略），`stop`据此按进程树清理；Windows、macOS与Linux使用同一记录。首次生成的`.env`请按平台方式限制权限，例如macOS/Linux的`chmod 600 .env`或Windows的ACL设置。

模块独立运行不自动读取根.env，由进程环境提供密码等值：

```sh
go run ./src/apps/iam/cmd check-config --config src/apps/iam/config.yaml
go run ./src/apps/console/cmd check-config --config src/apps/console/config.yaml
go run ./src/apps/iam/cmd migrate --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd serve --config src/apps/iam/config.yaml
go run ./src/apps/console/cmd --config src/apps/console/config.yaml
```

维护、恢复与迁移使用相同db连接。恢复备份后先运行`revoke-after-restore`，再开放流量；不自动down迁移或清卷。

Web独立运行使用Next.js原生.env.example→.env.local；NEXT_PUBLIC_*不能放密码、密钥或调用凭证。省略Go的--config时读取工作目录config.yaml；二进制部署可以显式指定配置。

## 必填、默认与高级设置

| 配置 | 本机默认 / 要求 | 何时修改 |
|---|---|---|
| `SPX_IAM_CSRF_KEY` | 必填，恰好 32 字节；`openssl rand -hex 16` 可生成 | 首次启动前填写；所有 IAM 副本保持一致 |
| `SPX_CONSOLE_IAM_TOKEN` | 必填，32–512 字节；`openssl rand -hex 32` 可生成 | 首次启动前填写；IAM 和 Console 使用同一个值 |
| `SPX_INGEST_IAM_TOKEN` | 必填，32–512 字节，使用独立值 | 首次启动前填写；IAM 和 Ingest 使用同一个值 |
| `SPX_IAM_BOOTSTRAP_USERNAME/PASSWORD` | 用户名默认 `admin`；密码必填，12–128 字符 | 设置唯一系统管理员 |
| `SPX_WEB_PORT` | 根模板 `5173`；独立 Web 必填 | 修改 Web 本机监听端口 |
| `SPX_<APP>_HOST`、`SPX_<APP>_PORT` | 根入口默认推导同机地址。IAM、Console、Ingest 分别使用 8081、8080、8082 | 独立运行且调用方与服务分开部署时设置连接目标 |
| `SPX_KAFKA_BROKERS` | 独立运行 Ingest 必填，Compose 使用 `kafka:9092` | 独立运行时填写实际 broker 地址，多个地址用逗号分隔 |
| `SPX_WEB_ORIGIN` | 可空，本机推导 `http://127.0.0.1:<Web 端口>` | 用服务器 IP、域名或 Nginx HTTPS 访问时配置真实入口，供 IAM 校验来源及设置 Cookie |
| `SPX_POSTGRESQL_*`、`SPX_REDIS_*` | 见根模板；对应实例、库及账号必须已存在 | 使用已有远程基础设施时统一修改；不自动创建外部账号 |
| 低频调优与诊断 | 根模板在对应模块内以注释展示。模块 YAML 保持默认值 | 按项取消注释并调整，查看该项适用的启动方式 |

独立运行修改配置后重启受影响的进程。整栈 Compose 修改 `.env` 后再次执行 `make compose-up`，让 Compose 以新环境创建受影响的容器。Go 开发热更新只监听代码和 SQL，改 YAML 后重新启动。已有托管管理员改名或改密需要执行 IAM 的 `sync-administrator`。已有 Kafka 主题在重新启动应用前先完成 [主题保留期维护](../../src/apps/ingest/README.md#修改已有主题的保留期)。

`config:check` 检查字段、类型、范围和必填凭证，不验证网络连通性、数据库权限或 schema 版本；这些由迁移和服务就绪检查验证。错误会指出 YAML 字段或环境变量名，不显示输入值。检查失败先按字段旁声明修正，再重新执行；没有应用迁移或启动任何选中服务。

独立 Web：在模块目录复制 `.env.example` 到 `.env.local`，填写已有 IAM/Console 地址，然后执行 `pnpm config:check --development`、`pnpm dev`；不带 `--development` 时检查 `start` 使用的生产环境文件。该模板直接定义值，不自引用，也不加载根 `.env`；根统一启动则直接注入同名进程变量，不需要子目录 `.env.local`。Next.js 原生环境加载优先保留已有进程变量。

## 日志与文件路径

独立运行时，Go 模块默认在 `logs/` 中按模块名写结构化 JSON。默认级别为 `info`，每个文件上限 50 MiB，保留至多 5 份且最长 14 天，轮转后压缩。每个副本使用自己的文件。

模块 YAML 中的相对路径以进程工作目录为准。根 `.env` 的 `*_FILE` 相对路径由根启动入口解析。整栈 Compose 输出到标准输出，由 Docker `local` 驱动按 `max-size=10m`、`max-file=3` 轮转。同一模块的日志级别、独立运行输出与轮转选项列在同一组。Ingest 的日志级别也用于事件写入进程。

数据库连接和凭据由对应进程读取。Ingest 独立运行时可指定原生 TLS 证书和私钥，并为外接 Kafka 指定 CA 文件。

## 部署命名

Compose默认使用工作区项目名，也支持原生COMPOSE_PROJECT_NAME覆盖。不同项目名使用不同数据卷；已有部署应保留原名或明确迁移数据。停止不删卷、不停止外部数据库，不增加应用instance_id。

公开参考：[Kratos结构化配置](https://github.com/go-kratos/kratos-layout/blob/main/configs/config.yaml)、[Next.js环境配置](https://nextjs.org/docs/app/guides/environment-variables)、[Lumberjack](https://github.com/natefinch/lumberjack/blob/v2.2.1/README.md)。具体字段以本项目模块配置为准。

## 浏览器访问和跨域

Web 与 API 默认通过同一个入口访问。IAM 根据 `auth.web_origin` 校验请求来源，并使用 Cookie 与 CSRF 令牌保护写操作。公开入口配置为 localhost、127.0.0.1 或 ::1 时，接受相同协议和端口的明确回环别名；可分别使用 localhost 和 127.0.0.1 登录，Cookie/会话仍由浏览器按主机隔离。非回环入口只接受配置中的来源，其他端口、协议或主机不在兼容范围。当前没有开放通用跨域白名单或跨域凭证访问；如需跨域部署，必须同时设计可信来源、Cookie、CSRF 和代理策略，不能只加允许所有来源的响应头。


## 反向代理与安全响应头

### 可信代理

登录限流按来源地址分桶，来源地址取自 `ClientIP()`，而它是否采信 `X-Forwarded-For` 完全由可信代理列表决定。

- 未配置 `SPX_IAM_TRUSTED_PROXY_CIDRS`：不解析转发头，来源地址就是直连对端。直连部署必须保持这个状态，否则任何客户端都能伪造来源。
- 部署在反向代理之后：填真实代理网段。留空会让所有请求看起来都来自代理地址，登录限流退化为全站共用一个桶（默认 20 次/分钟）。
- 写成 `0.0.0.0/0` 或 `::/0`：启动直接失败。这两个值等于信任任意客户端，与限流的目的相反。

正式入口将 `SPX_WEB_EDGE_TOKEN` 注入 Web，并以相同值配置反代。令牌使用 32 字节随机量的十六进制编码。下面两条指令放在转发到 Web 的 `location` 中，令牌一行由部署侧凭据文件提供。

```nginx
proxy_set_header X-SPX-Client-IP $remote_addr;
include /run/secrets/secplexus-edge-header.conf;
```

凭据文件内容为 `proxy_set_header X-SPX-Edge-Token "<token>";`。两条指令均覆盖访问方的同名头。IAM 的 `SPX_IAM_TRUSTED_PROXY_CIDRS` 填 Web 到 IAM 这一跳的地址段，IAM 端口仅在服务网络可达。 [GitLab 可信代理配置](https://docs.gitlab.com/omnibus/settings/nginx/#configure-gitlab-trusted-proxies-and-nginx-real_ip-module)

依据：Gin 官方文档 *Trusted proxies* 明确 "Gin trusts all proxies by default if you don't specify a trusted proxy … this is NOT safe"，并说明非可信代理直连时转发头会被完全忽略；同类真实缺陷见 [router-for-me/CLIProxyAPI#4733](https://github.com/router-for-me/CLIProxyAPI/issues/4733)（限流键取自未校验的 XFF）。

启动日志在未配置可信代理时输出一条 `service.proxy_trust_disabled`，提示部署者确认是否处于反代之后。

### 安全响应头

Go 服务（IAM、Console）与 Web 都下发同一组头，取值依据 [OWASP HTTP Headers Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html)：

| 头 | 值 | 作用 |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | 禁止 MIME 嗅探 |
| `X-Frame-Options` | `DENY` | 管理台不允许被内嵌 |
| `Content-Security-Policy` | API：`frame-ancestors 'none'; base-uri 'none'; form-action 'none'`；Web：`frame-ancestors 'none'; base-uri 'self'; object-src 'none'` | 压住点击劫持与基址劫持 |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | 显式声明，不依赖浏览器默认 |
| `Permissions-Policy` | 关闭 camera/microphone/geolocation/usb/payment | 本平台不使用这些特性 |
| `Strict-Transport-Security` | `max-age=31536000` | 明文 HTTP 下浏览器会忽略，反代终止 TLS 时同样生效 |
| `Cross-Origin-Opener-Policy` / `Cross-Origin-Resource-Policy` | `same-origin` | 隔离跨源窗口引用与资源加载 |
| `X-XSS-Protection` | `0` | 该头已废弃，显式关闭 |

CSP 目前不写 `script-src`：Web 的内联启动脚本需要 nonce 方案，属于独立改动；写死 `script-src` 会直接打断页面。
Web 关闭 `x-powered-by`，避免把框架与版本告诉扫描器。

Go 侧的安全头挂在 `httpx/middleware` 的 `NewEngine` 最前面，因此 404、405、panic 恢复与鉴权拒绝的响应带同一组头，不会出现"错误响应泄露路由是否存在"的差异。

## 配置与启动代码的职责

模块的 `internal/config` 只做加载、显式环境绑定、类型转换和校验，以及依据字段构造连接地址；不会访问数据库、创建 Redis/HTTP 客户端或执行迁移。`internal/bootstrap` 显式创建依赖、挂载路由并负责失败清理和退出关闭；数据库操作由 repository 承担。


## Next.js Web

根启动注入 Web 所需的 `SPX_WEB_PORT` 和 IAM、Console、Ingest 的上游地址，不注入 PG/Redis、管理员密码或服务凭证。Web 服务端用 Zod 校验连接配置，不在组件或请求中读取环境；代理命名空间下的 IAM、Console 和 Ingest API，保留 Cookie、Origin、CSRF、响应状态和流式语义，仅设备推送路由透传 Bearer 令牌。转发头按可信入口配置处理。

Web 独立运行时，将本模块 `.env.example` 复制为 `.env.local`，使用同名字段；通过 Next 官方 `@next/env` 加载后调用原生 CLI，配置的端口在 `pnpm dev` / `pnpm start` 均生效。不是自定义 Next 服务。进程环境优先于模块 env 文件；根启动已经注入变量时无需另配 `.env.local`。

`SPX_WEB_ORIGIN` 是 IAM 的安全输入，不传给 Web，也不再用于元信息；上游位置不会打进浏览器包。根构建允许没有私有 `.env`，以根示例的非秘密 Web 字段作为后备，不创建私有文件。修改运行配置后重启对应进程；不要从任意请求 Host/转发头推断可信入口。

`/dashboard` 保留 CRM V1 基准；身份及管理页面由真实 IAM 支撑。前端依赖、测试配置及 `@next/env` 均在 Web 模块，根 pnpm-lock.yaml 是唯一锁文件。`agentRules: false` 保持根 AGENTS 为统一指令来源。

Next 窄代理不透传 X-Forwarded-For、X-Real-IP 或 Forwarded，Gin 只信任显式配置的代理。来源限流当前计数代理连接来源；真实公网来源链仍属于部署前置条件，本轮不开展 Nginx 部署或可信多跳验收。运行日志排查见 [运行日志与错误定位](observability.md)。

## 声明式环境变量与默认值

```yaml
kafka:
  brokers: "${SPX_KAFKA_BROKERS-}"
  namespace: "${SPX_KAFKA_NAMESPACE-spx}"
  retention_days: "${SPX_KAFKA_RETENTION_DAYS-7}"
```

打开文件就能看到环境变量名与其默认值，一层展开即可读完。未设置某一环境变量时使用默认值；`${NAME-default}` 保留显式空值，`${NAME:-default}` 在空值时也后备。密码、服务凭证等仍按业务规则校验，不能因为用了变量展开就绕过必填或格式要求。

共享加载器先解析唯一 YAML 文档，再使用成熟的 `mvdan.cc/sh/v3` 参数展开组件逐个处理标量；没有启动 Shell，也没有配置命令执行器。环境值中的引号、换行、冒号及 `${...}` 按字面传入，不回写 YAML、不进行第二次表达式解释。Viper/mapstructure 继续负责严格解码和类型转换；配置校验保持模块边界。删除模块 Go 环境绑定文件，不再维护第二份映射或默认表。

进程环境优先于根 `.env`。根 `.env.example` 是统一公开配置模板，实际凭据只写 `.env`；构建只取模板中的非秘密 Web 字段作为后备。模块二进制独立运行读取进程环境与 YAML；用根入口选择单个模块即可直接复用整份根配置，无需复制模块 YAML。

`SPX_WEB_ORIGIN` 表示浏览器的可信来源：例如浏览器访问 `https://sec.example.com`，Nginx 转发至 HTTP Web，而 IAM 本身监听 HTTP 8081。IAM 无法从自身监听端口知道浏览器来源和是否应使用 Secure Cookie，因此此时填写实际来源。本机整套启动按 Web 端口自动生成，不必手填。这个值不用于监听、路由发现或元信息，也不会从任意请求头猜测。

应用名固定为所属模块的 iam/console，不再作为可配置项。内部 HTTP 协议固定，不配置 scheme、服务端证书或 CA 文件；配置能力仅保留真实使用的连接、会话/登录策略、保留期限、代理信任与日志轮转。

参考：[参数展开组件](https://pkg.go.dev/mvdan.cc/sh/v3/expand)、[Viper](https://github.com/spf13/viper)、[Next.js 官方环境加载](https://nextjs.org/docs/app/guides/environment-variables)。

## 首次构建与网络排障

Web 保留模板的 `next/font/google` 字体注册方式，首次构建需要访问 `fonts.googleapis.com` 和 `fonts.gstatic.com`。出现 `Error while requesting resource`，并伴随字体内部模块解析失败时，先检查构建机器到字体服务的 DNS、代理与网络连通性；这不代表 IAM/Console 的地址或应用证书配置错误。依赖安装成功不等于字体下载成功，后续命中缓存的构建也不等于首次构建验收通过。服务运行仍使用 HTTP，不能通过给 IAM 增加证书或关闭 TLS 校验来处理构建时的外部网络问题。

需要完全离线或受限网络构建时，必须另行确定字体资源随项目交付的方式及许可证，保持现有字体、字重与主题基准；本次没有替换字体或新增下载代理配置。

## 配置命名规则（强制）

键名必须回答两个问题：**这是谁的配置**、**它决定什么行为**。用既有模块和业务术语描述作用，例如 `SPX_KAFKA_RETENTION_DAYS`。

### 依赖连接与业务配置

模块 YAML 使用 `postgresql`、`redis`、`clickhouse`、`kafka` 记录实际依赖。根模板只维护一份平台地址和传输设置。常驻进程、查询和迁移账号分别只获取所需权限。模块拥有主题、数据库、表、键及业务策略。

源码或自定义编排中的 PostgreSQL 使用相同的连接字段。外接环境为迁移和常驻服务按各自的进程注入 `SPX_POSTGRESQL_URL`，共享集群地址及数据库，分别使用持有所需权限的账号。内置开发 Compose 共用单个开发账号。

Ingest 的 `input.kafka` 用于外部 Kafka 数据接入，`kafka` 用于平台集群连接，`events` 声明发布和写入身份。事件主题、写入失败主题和消费组由 `SPX_KAFKA_NAMESPACE` 统一派生，默认 `spx`。共享集群中的每套部署使用不同且稳定的命名空间。Kafka 平台主题默认保留 7 天，使用 `SPX_KAFKA_RETENTION_DAYS` 调整。ClickHouse 事件按 UTC 月分区永久保存。 [Ingest 配置](../../src/apps/ingest/config.yaml) · [Kafka 消费组配置](https://kafka.apache.org/42/configuration/consumer-configs/#group.id)

根模板按同样的顺序组织公开与私有环境文件。每个组件使用三行分隔标题。模块里的共用小组依次为网络、服务凭据和日志，模块专用选项插在服务凭据与日志之间。每种用途只使用一个连续小组，常用项和可选项一起展示。小组之间空一行，注释写在相关项上方，说明用途、单位、空值含义或适用方式。需要一起设置的项相邻排列。

Ingest 的 HTTP 请求体和并发上限、每个数据接入的限流、队列与批量发布、Kafka 和 Syslog 接入刷新均可通过根 `.env` 配置。整栈 Compose 和源码启动都读取这些值。嵌套的外部 Kafka 接入凭据由模块 YAML 声明。开发测试专用连接见 [测试指南](testing.md)。

### 共享库不绑定 Web 框架

`iamclient` 只包含协议客户端（`Authorize`、`RecordAudit`、类型与常量），
不 import 任何 Web 框架。gin 适配与审计编排在 `httpx/middleware`（`Guard`、`Session`）。

依据：`golang.org/x/oauth2` 的 `Transport` 实现 `http.RoundTripper`、Kubernetes client-go
只提供 net/http 客户端，框架适配都在使用方。这样 `iamclient` 可被 CLI、任务、其他框架复用。
