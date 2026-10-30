# 配置与启动

## 配置文件

| 文件 | 内容 |
|---|---|
| 根`.env.example` → `.env` | 一键启动配置，按Web、IAM、Console、PostgreSQL、Redis分区 |
| `src/apps/iam/config.yaml` | IAM监听、数据库、Redis、认证策略、日志 |
| `src/apps/console/config.yaml` | Console监听、IAM连接、日志 |
| `src/apps/web/.env.example` → `.env.local` | Web独立运行的Next.js配置 |

根 `.env.example` 是整套应用的一键配置清单，复制为 `.env` 后统一填写。运行入口先合并进程环境与根 `.env`，再按模块注入；模块无需分别修改配置。Go 读取自己的 `config.yaml`，变量声明与默认值直接写在 YAML 中，优先使用环境值。实际 `.env` 和日志不提交。

## PostgreSQL：默认一个实例、一个库、一组账号

全局只维护`SPX_POSTGRES_HOST/PORT/DATABASE/USER/PASSWORD/SSLMODE`。默认数据库账号/库名为 `secplexus`，密码为 `secplexus`。YAML 的参数声明将这组环境值应用到模块 db 字段；应用运行、迁移、维护都使用同一连接，不创建模块用户或另一套运维账号。

账号需要具备本项目schema和版本表的建表、迁移权限。应用层的鉴权与审计事务保持不变；不再依靠单独数据库账号限制HTTP进程的DDL权限。

同一数据库中的表仍由对应模块管理。Console当前通过IAM接口取得身份信息，没有数据库查询，因此不添加无用的db配置。

需要让IAM使用独立连接时，使用`SPX_IAM_POSTGRES_HOST/PORT/DATABASE/USER/PASSWORD/SSLMODE`覆盖共享变量，或在没有相应环境变量时修改模块YAML字面值。Redis同理使用`SPX_IAM_REDIS_HOST/PORT/USER/PASSWORD/DB`。Console没有DB/cache配置。

完整DSN只作为高级覆盖`SPX_POSTGRES_URL`保留，用于特殊驱动参数；根示例不要求填写它。

## Redis：统一连接与key模板

连接使用`SPX_REDIS_HOST/PORT/USER/PASSWORD/DB`，默认密码为 `secplexus`。用户名默认 `default`（没有 root 账号），可通过 `cache.user` 或 `SPX_REDIS_USER` 修改。外部 Redis 应预先创建该 ACL 用户并授予应用所需权限；普通应用启动不会改外部用户。完整地址仍可通过 `SPX_REDIS_URL=redis://用户名:密码@主机:端口/库号` 指定，URL 中的特殊字符须编码；key 前缀固定为 spx，不提供配置覆盖。

[cache/key](../src/packages/go/cache/key/iam.go)集中定义已实现的类型化模板，调用Format构建key，不在业务代码中散落字符串拼接：

```text
spx:iam:ratelimit:ip:<sha256>
spx:iam:ratelimit:account:<sha256>
```

前缀、模块和业务名由模板固定。以后新增实际业务时增加对应模板，不预建尚不存在的key。不同平台部署可使用独立Redis库；同平台副本共用连接及前缀。

## 服务位置、监听与浏览器入口

根文件统一维护服务连接位置，Web 与 Console 使用同一组 IAM 设置，不各自填写另一份 URL：

```dotenv
SPX_WEB_PORT=5173
SPX_IAM_HOST=127.0.0.1
SPX_IAM_PORT=8081
SPX_CONSOLE_HOST=127.0.0.1
SPX_CONSOLE_PORT=8080
# 本机可留空；经服务器 IP、域名或反向代理访问时填实际浏览器入口。
SPX_WEB_ORIGIN=
```

- `HOST` 是调用方连接的 IP/域名；部署在其他机器时修改相应 HOST/PORT，所有调用方共同生效
- `BIND_HOST` 是当前进程监听的本机地址，默认 `0.0.0.0`；不是调用目标，也不代替认证/防火墙。Go 默认只定义在所属 YAML；Web 默认由本模块加载器拥有
- 端口通常共用 `SPX_IAM_PORT` / `SPX_CONSOLE_PORT`；确实需要本机监听不同端口时，用模块 `BIND_PORT` 显式覆盖，不改变调用方目标
- Web 监听端口只由 `SPX_WEB_PORT` 决定。`SPX_WEB_ORIGIN` 只供 IAM 的可信来源及 Cookie/CSRF 使用；Web 元信息不需要绝对地址，已移除相应依赖，不能决定本地监听端口；本机默认由 Web 端口生成 localhost origin
- 应用服务全部 HTTP，HTTPS 和证书由 Nginx 等反向代理处理。模块没有 scheme、证书或 CA 文件配置；外部入口可以 HTTP 或 HTTPS

同机启动读取实际 Go 监听结果，对未填写的连接位置推导可连接地址；不能将 `0.0.0.0` 用作客户端目标。进程环境、根 `.env` 的明确服务位置保持优先。

`pnpm dev` / `pnpm preview` 默认启动本地全部模块。`pnpm dev web` / `pnpm preview web` 仅启动 Web，连接配置指定的已有 IAM/Console，不执行 Go 迁移或启动其他模块；`pnpm dev iam`、`pnpm dev console` 单独启动指定后端。不会依据 IP 自动猜测拓扑或远程启动进程。

Compose 使用相同字段，通过容器服务名连接；公开映射端口由根配置推导。已有静态部署文件随契约同步，本轮不构建/部署容器。

## 环境变量与模块覆盖

```yaml
db:
  host: "${SPX_IAM_POSTGRES_HOST-${SPX_POSTGRES_HOST-127.0.0.1}}"
  port: "${SPX_IAM_POSTGRES_PORT-${SPX_POSTGRES_PORT-5432}}"
  name: "${SPX_IAM_POSTGRES_DATABASE-${SPX_POSTGRES_DATABASE-secplexus}}"
  user: "${SPX_IAM_POSTGRES_USER-${SPX_POSTGRES_USER-secplexus}}"
  password: "${SPX_IAM_POSTGRES_PASSWORD-${SPX_POSTGRES_PASSWORD-secplexus}}"
```

根入口先按“进程环境 > 根.env”合并并过滤到所属模块。Go字段优先级为：**模块专用环境 > 共享环境 > YAML字面值**；模块 YAML 是默认值的唯一来源，不维护另一份 Viper 默认表。显式空值保留，随后进行类型及业务配置校验。仅 YAML 字段声明的变量参与映射，不自动扫描任意环境键。模块独立运行不读取根.env。

完整DSN字段`SPX_IAM_POSTGRES_URL`优先于`SPX_POSTGRES_URL`，`SPX_IAM_REDIS_URL`优先于`SPX_REDIS_URL`；配置了非空URL时不再组合分项字段。模块环境显式设空URL可恢复分项组合。`SSLMODE` 与 PostgreSQL 驱动的 `sslmode` 语义一致。

YAML 使用标准参数表达式 `${NAME-default}`（未设置才后备）或 `${NAME:-default}`（未设置或空值后备）。变量名称、优先级与默认值都在字段旁声明；环境标量由 mapstructure 官方 Hook 转换为整数、布尔、列表和时长。未知字段、重复键、额外文档、非法类型与超过64KiB配置在加载边界拒绝，错误不输出秘密值。修改配置后重启。服务调用方是以服务ID为键的`service_clients`映射；凭证和action白名单属于同一配置项，不能以数组顺序猜测调用方。

## 运行

需要Go 1.27.3、Node.js 22.12+（建议24）、pnpm 10.32.1。

```sh
cp .env.example .env
chmod 600 .env
# 在根 .env 一次填写服务位置、数据库/Redis、管理员密码、CSRF密钥与Console凭证
# 使用已有PG/Redis，或显式部署基础组件：
pnpm compose:up postgres redis
pnpm config:check
make dev
```

`make dev`不部署基础组件，按配置连接；先离线校验全部选中模块，校验通过后使用同一数据库连接执行迁移/幂等初始化，再启动IAM、Console和Web。配置托管的唯一超级管理员默认 `admin/secplexus`，由 `SPX_IAM_BOOTSTRAP_USERNAME` / `SPX_IAM_BOOTSTRAP_PASSWORD`（对应 IAM YAML 的 bootstrap）指定。显式迁移和 IAM 启动会同步该配置；密码改变时更新哈希并撤销旧会话，配置不变时保持哈希和会话。该账号不能在平台内编辑、停用、移除或修改/重置密码，也不能通过运维重置命令绕过。修改配置后重启所有 IAM 副本，各副本须一致。普通账号仍支持自助改密与管理员重置。密码要求 8–128 个字符，管理员配置同样校验。

本地开发用 PostgreSQL、Redis 是持久资源：`pnpm compose:up postgres redis` 创建或启动同一套容器，数据使用命名卷保存；以后可在 Docker Desktop 中直接启动已有容器，也可重复执行该命令。`make stop` 只停止它们，不删除容器、卷或账号数据。普通启动、停止和测试清理都不能对这套开发资源执行 `down -v`、`rm -v` 或全局 prune；自动化测试另用隔离测试资源（临时数据库或显式测试容器），并仅清理本次测试资源。

| 命令 | 行为 |
|---|---|
| pnpm config:check [iam console web] | 离线检查选中模块，不连接依赖、不迁移、不创建业务日志；省略模块时检查全部 |
| make dev | 迁移、启动三个应用，Go/React代码热更新 |
| make build | 构建Go二进制到bin/、Web到模块.next/；可无.env，存在时加载Web所属配置 |
| pnpm preview | 启动后端并预览已构建Web，公共地址需与实际访问地址一致 |
| pnpm compose:up | 显式构建并部署整套服务 |
| make stop | 停当前工作区应用/容器，保留数据，无需.env |

所有统一启动入口都先检查.env：缺失则生成并提示检查，不覆盖已有文件。开发编译使用系统临时目录，正常退出后清理，不创建.local。

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
| `SPX_IAM_BOOTSTRAP_USERNAME/PASSWORD` | `admin` / `secplexus`；密码 8–128 字符 | 设置唯一系统管理员，正式使用前替换默认密码 |
| `SPX_WEB_PORT` | 根模板 `5173`；独立 Web 必填 | 修改 Web 本机监听端口 |
| `SPX_IAM_HOST/PORT`、`SPX_CONSOLE_HOST/PORT` | 模板使用回环地址及 `8081` / `8080` | 服务位于其他机器时修改；不是防火墙规则 |
| `SPX_WEB_ORIGIN` | 可空，本机推导 `http://localhost:<Web端口>` | 用服务器 IP、域名或 Nginx HTTPS 访问时配置真实入口，供 IAM 校验来源及设置 Cookie |
| `SPX_POSTGRES_*`、`SPX_REDIS_*` | 见根模板；对应实例、库及账号必须已存在 | 使用已有远程基础设施时统一修改；不自动创建外部账号 |
| 模块 `BIND_HOST/BIND_PORT` | YAML 默认监听 `0.0.0.0`，端口沿用公共服务端口；Web 可选 `SPX_WEB_BIND_HOST` | 只绑定某张网卡、回环地址或本机监听端口不同于连接端口时 |
| 模块专用 DB/Redis 变量、完整 DSN、日志及认证策略 | 字段旁 YAML 声明；高级项无需复制到根模板 | 确实需要模块独立连接、驱动额外参数或调整策略时 |

修改 `.env`、进程变量或 YAML 后重启受影响服务；Next 运行时配置也需重启。修改 IAM 可信来源需重启 IAM；修改服务连接位置需重启对应服务及其调用方。配置管理员身份/密码变更需同步重启全部 IAM 副本。Go 开发热更新只监听代码与 SQL，不把 YAML 变更当作已自动生效。

`config:check` 检查字段、类型、范围和必填凭证，不验证网络连通性、数据库权限或 schema 版本；这些由迁移和服务就绪检查验证。错误会指出 YAML 字段或环境变量名，不显示输入值。检查失败先按字段旁声明修正，再重新执行；没有应用迁移或启动任何选中服务。

独立 Web：在模块目录复制 `.env.example` 到 `.env.local`，填写已有 IAM/Console 地址，然后执行 `pnpm config:check --development`、`pnpm dev`；不带 `--development` 时检查 `start` 使用的生产环境文件。该模板直接定义值，不自引用，也不加载根 `.env`；根统一启动则直接注入同名进程变量，不需要子目录 `.env.local`。Next.js 原生环境加载优先保留已有进程变量。

## 日志与文件路径

本机从仓库根运行时统一写logs/iam.log、logs/console.log；slog输出JSON，Lumberjack按模块配置轮转，默认info、50MB、5份、14天压缩。多副本不能共写一个轮转文件。

相对文件路径以进程工作目录为准；二进制部署设置WorkingDirectory或显式日志路径。容器默认stdout，由Docker local驱动按10MB、3份轮转，需要文件日志时显式挂载可写目录。

应用不读取 TLS 证书/私钥或 CA 文件。数据库连接、密码及服务凭证由进程环境注入，不另建凭证文件。根 `.env` 中确需的日志文件相对路径按仓库根目录解析；Go 独立运行时相对路径按工作目录解析。

## 部署命名

Compose默认使用工作区项目名，也支持原生COMPOSE_PROJECT_NAME覆盖。不同项目名使用不同数据卷；已有部署应保留原名或明确迁移数据。停止不删卷、不停止外部数据库，不增加应用instance_id。

公开参考：[Kratos结构化配置](https://github.com/go-kratos/kratos-layout/blob/main/configs/config.yaml)、[Next.js环境配置](https://nextjs.org/docs/app/guides/environment-variables)、[Lumberjack](https://github.com/natefinch/lumberjack/blob/v2.2.1/README.md)。具体字段以本项目模块配置为准。

## 浏览器访问和跨域

Web 与 API 默认通过同一个入口访问。IAM 根据 `auth.web_origin` 校验请求来源，并使用 Cookie 与 CSRF 令牌保护写操作。公开入口配置为 localhost、127.0.0.1 或 ::1 时，接受相同协议和端口的明确回环别名；可分别使用 localhost 和 127.0.0.1 登录，Cookie/会话仍由浏览器按主机隔离。非回环入口只接受配置中的来源，其他端口、协议或主机不在兼容范围。当前没有开放通用跨域白名单或跨域凭证访问；如需跨域部署，必须同时设计可信来源、Cookie、CSRF 和代理策略，不能只加允许所有来源的响应头。


## 配置与启动代码的职责

模块的 `internal/config` 只做加载、显式环境绑定、类型转换和校验，以及依据字段构造连接地址；不会访问数据库、创建 Redis/HTTP 客户端或执行迁移。`internal/bootstrap` 显式创建依赖、挂载路由并负责失败清理和退出关闭；数据库操作由 repository 承担。

IAM 缓存分组为 `cache`，字段为 host、port、password、db、url；当前实现仍是 Redis，环境变量继续使用 `SPX_REDIS_*`，key 前缀在代码中固定为 spx。不增加占位 driver 或嵌套 redis 分组。PostgreSQL 环境变量保留 `SPX_POSTGRES_SSLMODE`，驱动连接参数仍是 `sslmode`。升级需修改配置分组名时保留原连接值，不覆盖用户 `.env`。

## Next.js Web

根启动注入 Web 所需的 `SPX_WEB_PORT`、`SPX_WEB_ORIGIN`、IAM/Console HOST/PORT，不注入 PG/Redis、管理员密码或服务凭证。Web 服务端用 Zod 校验连接配置，不在组件或请求中读取环境；只代理 `/api/v1/iam/...` 与 `/api/v1/console/...`，保留 Cookie、Origin、CSRF、响应状态和流式语义，外部 Authorization/伪造转发头不透传。

Web 独立运行时，将本模块 `.env.example` 复制为 `.env.local`，使用同名字段；通过 Next 官方 `@next/env` 加载后调用原生 CLI，配置的端口在 `pnpm dev` / `pnpm start` 均生效。不是自定义 Next 服务。进程环境优先于模块 env 文件；根启动已经注入变量时无需另配 `.env.local`。

`SPX_WEB_ORIGIN` 是 IAM 的安全输入，不传给 Web，也不再用于元信息；上游位置不会打进浏览器包。根构建允许没有私有 `.env`，以根示例的非秘密 Web 字段作为后备，不创建私有文件。修改运行配置后重启对应进程；不要从任意请求 Host/转发头推断可信入口。

`/dashboard` 保留 CRM V1 基准；身份及管理页面由真实 IAM 支撑。前端依赖、测试配置及 `@next/env` 均在 Web 模块，根 pnpm-lock.yaml 是唯一锁文件。`agentRules: false` 保持根 AGENTS 为统一指令来源。

Next 窄代理不透传 X-Forwarded-For、X-Real-IP 或 Forwarded，Gin 只信任显式配置的代理。来源限流当前计数代理连接来源；真实公网来源链仍属于部署前置条件，本轮不开展 Nginx 部署或可信多跳验收。运行日志排查见 [运行日志与错误定位](observability.md)。

## 声明式环境覆盖与默认值

```yaml
db:
  host: "${SPX_IAM_POSTGRES_HOST-${SPX_POSTGRES_HOST-127.0.0.1}}"
  port: "${SPX_IAM_POSTGRES_PORT-${SPX_POSTGRES_PORT-5432}}"
  sslmode: "${SPX_IAM_POSTGRES_SSLMODE-${SPX_POSTGRES_SSLMODE-disable}}"
```

打开文件就能看到模块环境、共享环境和默认值的顺序。未设置某一环境变量时继续后备；`${NAME-default}` 保留显式空值，`${NAME:-default}` 在空值时也后备。密码、服务凭证等仍按业务规则校验，不能因为用了变量展开就绕过必填或格式要求。

共享加载器先解析唯一 YAML 文档，再使用成熟的 `mvdan.cc/sh/v3` 参数展开组件逐个处理标量；没有启动 Shell，也没有配置命令执行器。环境值中的引号、换行、冒号及 `${...}` 按字面传入，不回写 YAML、不进行第二次表达式解释。Viper/mapstructure 继续负责严格解码和类型转换；配置校验保持模块边界。删除模块 Go 环境绑定文件，不再维护第二份映射或默认表。

进程环境优先于根 `.env`。根 `.env.example` 是统一公开配置模板，实际凭据只写 `.env`；构建只取模板中的非秘密 Web 字段作为后备。模块二进制独立运行读取进程环境与 YAML；用根入口选择单个模块即可直接复用整份根配置，无需复制模块 YAML。

`SPX_WEB_ORIGIN` 表示浏览器的可信来源：例如浏览器访问 `https://sec.example.com`，Nginx 转发至 HTTP Web，而 IAM 本身监听 HTTP 8081。IAM 无法从自身监听端口知道浏览器来源和是否应使用 Secure Cookie，因此此时填写实际来源。本机整套启动按 Web 端口自动生成，不必手填。这个值不用于监听、路由发现或元信息，也不会从任意请求头猜测。

应用名固定为所属模块的 iam/console，不再作为可配置项。内部 HTTP 协议固定，不配置 scheme、服务端证书或 CA 文件；配置能力仅保留真实使用的连接、会话/登录策略、保留期限、代理信任与日志轮转。

参考：[参数展开组件](https://pkg.go.dev/mvdan.cc/sh/v3/expand)、[Viper](https://github.com/spf13/viper)、[Next.js 官方环境加载](https://nextjs.org/docs/app/guides/environment-variables)。

## 首次构建与网络排障

Web 保留模板的 `next/font/google` 字体注册方式，首次构建需要访问 `fonts.googleapis.com` 和 `fonts.gstatic.com`。出现 `Error while requesting resource`，并伴随字体内部模块解析失败时，先检查构建机器到字体服务的 DNS、代理与网络连通性；这不代表 IAM/Console 的地址或应用证书配置错误。依赖安装成功不等于字体下载成功，后续命中缓存的构建也不等于首次构建验收通过。服务运行仍使用 HTTP，不能通过给 IAM 增加证书或关闭 TLS 校验来处理构建时的外部网络问题。

需要完全离线或受限网络构建时，必须另行确定字体资源随项目交付的方式及许可证，保持现有字体、字重与主题基准；本次没有替换字体或新增下载代理配置。
