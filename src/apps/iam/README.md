# SecPlexus IAM

平台的账号与授权服务。管账号、角色、权限、登录会话和操作审计。

所有其他模块都向它要授权结论，自己不做账号判断。

## 它是什么

IAM 是整个平台里唯一知道「你是谁、你能干什么」的地方。

用户登录拿到 HttpOnly Cookie。此后每个请求，业务模块把 Cookie 转给 IAM，IAM 回答两件事，这个人是谁，这个动作允不允许。业务模块不存用户表，不校验密码，不判断角色。

用 Go + Gin 实现，不部署 Keycloak、Zitadel 这类独立账号产品。认证、用户目录、授权在同一个进程里。大厂拆成独立用户中心是被几十个系统几万员工的规模逼出来的部署拆分，不是领域本该拆。我们就一套安全运营团队，拆开只剩同步和一致性成本。Grafana、GitLab、Django、TheHive 都是内置合并的。

## 它管什么，不管什么

| 管 | 不管 |
|---|---|
| 账号、密码、登录会话 | 业务资源的属主和可见性 |
| 用户与角色、角色与权限的绑定 | 审批状态机 |
| 角色与权限的增删改 | 菜单和按钮长什么样 |
| 登录、改密、审计 | 具体某条 Case 该不该给你看 |
| 回答「这个用户有没有这个 action 的权限」 | 业务模块内部的状态检查 |

IAM 回答的是「你有没有这个能力」，不是「这条数据归不归你」。后者必须由资源所属模块自己查真实归属，因为只有它知道那张表长什么样。

## 业务架构

### 三个概念

**用户**是自然人，全局唯一。一个人的账号在整个平台只有一个。

**角色**是权限的容器。角色只有名称、说明和权限三项，没有业务代码、没有类型；除内置的超级管理员外不预设任何角色，需要什么角色由使用者按需创建并勾选权限。

**权限**是扁平的能力键，共 7 项。没有分级作用域，也没有作用域参数。

授权只有一条路径：

```
用户 --user_roles--> 角色 --role_permissions--> 权限
```

用户直接持有角色，没有中间关系对象；角色全局存在，不属于任何范围。没有继承、没有作用域、没有按范围裁剪的判定分支。

产品是一台设备、一个安全运营团队、一套权限：没有外部租户，没有多客户隔离。因此模型里没有租户、范围或关系中间层，也就不存在「先解析范围再合并授权」的递归判定。

### 多角色，权限取并集

一个人可以同时是「管理员」和「审计员」。有效权限是所有角色的并集。

刻意这么设计。现实中同一个人经常身兼多职，强制单角色会逼出「为每种组合建一个角色」的组合爆炸，最后角色表里躺着「管理员+审计员」「管理员+只读」这类垃圾。

首版没有 deny 优先级，没有角色继承，没有用户直授。要收回权限就把用户从角色里移除，或把权限从角色里去掉。

### 权限目录在代码里

7 个扁平权限键：

```
iam.users.read       查看用户与角色归属        normal   无需近期认证
iam.users.manage     创建、修改、删除用户      high     需近期认证
iam.users.roles      分配用户角色              high     需近期认证
iam.users.reset      重置用户密码              high     需近期认证
iam.roles.read       查看角色与权限目录        normal   无需近期认证
iam.roles.manage     创建、修改、删除角色      high     需近期认证
iam.audit.read       查看审计事件              normal   无需近期认证
```

高风险键标记 `requires_recent_auth`：操作者必须在近期认证窗口（默认 30 分钟）内确认过密码，
否则返回 `iam.recent_auth_required`。个人资料页与每个管理表单共用同一个确认框，确认后自动重放该次提交。

清单在 `internal/authorization/permissions.json`，版本化管理。数据库里有一份运行时投影，启动时同步。

权限键固定为 `module.resource.action`，这三段不单独存列而是由键解析得到：少一份可以漂移的副本。
目录里删掉的键不会消失，而是在下次启动时标记为 `retired`——历史授权保留，但立即不再生效，
因此「删权限」不会留下一个仍然可用的幽灵键。

同步只按 `permission_key` upsert 元数据；键本身是唯一稳定标识，键变化等同新增权限，旧键在下次启动时标记为 `retired`。已执行的 SQL 迁移不得修改；表结构变化必须新增前向迁移，不能要求部署侧删除 schema 或 Goose 版本表。

模块开发者定义「能做什么」，应用使用者决定「谁能做」。使用者没法凭空造出一个后端不存在的权限。

委派有上限。操作者只能把「自己当前持有 且 目录标记 delegable」的权限授予别人。持有 `iam.roles.manage` 不等于拥有全部业务权限。

### 一次授权判定怎么走的

五个步骤，全在同一个数据库事务里。

1. 校验服务凭证与 action 白名单（服务调用场景）。
2. 加载用户会话，校验状态、token 摘要与 `auth_epoch`。
3. 取该用户经 `user_roles` 持有的全部角色，合并 `role_permissions` 得到有效权限。
4. 判断目标 action 是否在有效权限内，并检查 `requires_recent_auth` 的近期认证窗口。
5. 写审计并返回决定。

只做请求内合并，不跨请求缓存。这是刻意的。一旦缓存「允许」结果，撤权就不是即时的，用户被移出角色后还能继续访问一段时间。不存在 Redis 故障后继续用旧权限的降级路径。

依赖不可用时返回 503，绝不放行。

判定结果只有 `{allowed, decision_id, principal}`：扁平模型没有范围字段，调用方也不需要再比对作用域。

### 内置数据来自数据库

三类内置数据在启动时按配置幂等写入，前端只读接口，不在前端写死任何内置行：

1. **权限目录**：以代码中的 `permissions.json` 为准，补充缺失键，更新描述、风险级别与可委派等元数据。
2. **超级管理员角色**：`name='超级管理员'`、`builtin=true`，权限是权限目录全部有效键；只补不删。该角色不可改名称、不可改说明、不可改权限、不可删除。
3. **配置管理员**：`bootstrap.username/password` 写入 `users.configured=true`，并授予超级管理员角色；改配置后下次启动更新用户名/密码并撤销旧会话。

标记写在数据里：`roles.builtin` 标记内置角色，`users.configured` 标记配置托管账号。数据库对两列各有一个部分唯一索引（`roles_single_builtin`、`users_single_configured`），保证各只有一行；服务层据标记拒绝改名、改权限、删除、停用与页面改密。

超级管理员就是「持有内置超级管理员角色」这一条数据，不再有 `users.is_super_admin`。

它是最后的兜底。配置管理员不能在页面被删除、停用或改密码：如果能被页面修改，一个被盗的普通管理员账号就能把自己提权成超管，整个权限体系归零。

明文不进响应和日志。

## 认证与会话

### 覆盖范围

| 能力 | 状态 |
|---|---|
| 用户名密码登录 | ✅ |
| 登出 | ✅ |
| 二次认证（高风险操作前重新输密码） | ✅ |
| 本人改密 | ✅ |
| 会话校验（每个请求） | ✅ |
| 滚动续期 | ✅ |
| 令牌轮换 | ✅ |
| 即时撤销（改密/停用/恢复后） | ✅ |
| 并发登录限流 | ✅ |
| 账号锁定 | ✅ |
| 公开注册 | ❌ 不做 |
| SSO（OIDC/SAML） | ❌ 未实现 |
| MFA | ❌ 未实现 |
| API Token / SCIM | ❌ 未实现 |

**没有公开注册是刻意的。** 账号由持有 `iam.users.manage` 的管理员创建，超级管理员来自启动配置。谁能进系统必须有人负责。如果开放注册，任何知道域名的人都能建账号。

### 会话机制

不用 JWT，用服务端会话 + Cookie。

```
登录
  → 校验密码（Argon2id）
  → 建逻辑会话（user_sessions）
  → SCS 写 Cookie（存会话 ID）
  → 返回 HttpOnly Cookie

每个后续请求
  → 加载 Cookie
  → 查逻辑会话：状态 / token 摘要 / 账号版本 / 双超时
  → 续期 idle_expires_at（不超过 absolute）
```

三个机制配合

| 机制 | 作用 |
|---|---|
| **滚动续期** | 每次请求把空闲超时往后推，默认 30 分钟 |
| **令牌轮换** | 登录和二次认证时换新 token，旧逻辑会话标记撤销 |
| **账号版本** | 改密、停用、改配置时递增 `auth_epoch`，所有旧会话立即失效 |

`absolute_expires_at` 是硬上限，默认 8 小时，滚动续期不能突破它。

**这套比 JWT 更严格**。JWT 一旦签发无法撤销，而这里每次请求都查库验证，撤权即时生效。

Cookie 特性

```
HTTP 环境   spx_session          HttpOnly, SameSite=Lax, Path=/
HTTPS 环境  __Host-spx_session   HttpOnly, SameSite=Lax, Path=/, Secure
```

CSRF token 放在独立 Cookie `spx_csrf`，写请求必须带 `X-CSRF-Token` header。

### 登录防爆破

三层防护

| 层 | 限制 |
|---|---|
| 按来源 IP | 每分钟 60 次 |
| 按账号 | 每 15 分钟 20 次 |
| 连续失败锁定 | 默认 5 次失败锁 15 分钟 |

限流计数在 Redis，Redis 故障时登录写操作返回 503，不放行。

密码计算有并发上限，同时最多 4 个 Argon2id 运算。这是防 CPU 打满，不是限用户。

失败登录也写审计（`outcome=denied`，`reason=invalid_credentials`），但只在有明确用户时递增失败计数。不存在的用户名不会暴露差异，实现上走一个假哈希比较，耗时一致。

### 账号规则

**用户名** 3–39 位，字母开头、字母或数字结尾，中间允许 `.` `_` `-`。拒绝连续分隔符（`a--b`、`a..b`）。

保留名单阻止新账号占用系统概念标识，`root`、`system`、`master`、`support`、`security`、`api`、`null`、`undefined`。保留检查只在创建时做，不进入登录路径，否则已存在的账号会登不进去。

**密码** 12–128 个 Unicode 字符，不能含 NUL。用 Argon2id 哈希，存 PHC 格式（含参数）。

**邮箱** 可选，最长 254 字符，本地部分不超过 64。唯一性用小写归一后的生成列加部分唯一索引，空邮箱不参与唯一性。不剥离加号子地址（那是合法的），也不剥离点号（那只对 gmail 消费者账号成立）。

**显示名** 1–100 字符，不能含 NUL。

**时区** 可选，用户展示偏好。接受 `utc`、`browser` 语义值或合法 IANA 时区名。服务端始终以 UTC 存储和传输，这列只影响渲染。

## 技术架构

### 模块间关系

业务模块是**平级**的，没有上下游之分。

```
                    浏览器
                       │  Cookie + CSRF
                       ▼
                  Web（同源入口）
                       │  /api/v1/{module}/* 窄代理
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
    Console       llm-gateway    collector…
        │              │              │
        └──────────────┴──────────────┘
                       │  各自独立调用
                       ▼
                      IAM（:8081）
                   ├─▶ PostgreSQL
                   └─▶ Redis
```

每个模块从 Web 同源入口进入，各自向 IAM 要授权结论。

**Console 不是网关，不是权限中转站。** 它只是安全运营业务模块，按自身业务需要调用其他模块。其他模块要验权限，直接调 IAM。

这一点在架构文档里写死了（`docs/design/architecture/02-technical-architecture.md`），也是代码事实，Web 的代理白名单里 `iam` 和 `console` 是并列的两个模块前缀，没有谁包着谁。

### IAM 的请求链路

```
浏览器
   HttpOnly Cookie（spx_session）+ CSRF token
      ↓
Web（Next.js）
   同源窄代理 /api/v1/iam/*
      ↓
IAM（Go + Gin，:8081）
   ├─▶ PostgreSQL（iam schema）
   └─▶ Redis（限流计数）
```

Web 只做转发，不解析账号，不合并权限。Cookie 原样传。

单端口，用户接口和服务接口共用 `/api/v1/iam` 前缀，按路由分别认证。应用内明文 HTTP，HTTPS 由外层反向代理终止。

### 技术选型

| 用途 | 选择 | 为什么 |
|---|---|---|
| HTTP | Gin | 原生路由、中间件、错误链、SSE |
| 数据访问 | sqlc（生成类型化查询）+ pgx v5 | SQL 是唯一事实源，列名/类型/参数在编译期受检查；schema 取 goose 迁移，不维护第二份 DDL。见 [数据访问规范](../../../docs/development/data-access.md) |
| 驱动 | pgx v5 | 连接池只有一份：`database/sql` 适配、SCS 会话存储和迁移检查共用它 |
| 会话 | SCS | Cookie 编码、续期、store 接口现成 |
| 密码 | Argon2id | 内存硬，抗 GPU 爆破 |
| CSRF | gorilla/csrf | 标准实现 |
| 迁移 | Goose | 显式执行，版本记录在库，不做启动自动建表 |
| 命令 | Cobra | 七个子命令 |
| 配置 | Viper + mapstructure | YAML 声明环境占位，严格类型解码 |

### 九张表

```
iam.users                 用户主表（含 timezone、configured、auth_epoch、email_normalized）
iam.password_credentials  密码哈希、失败计数、锁定
iam.roles                 角色（全局；内置标记 builtin）
iam.permissions           权限目录（运行时投影；键即标识）
iam.role_permissions      角色 ↔ 权限
iam.user_roles            用户 ↔ 角色
iam.user_sessions         逻辑会话（可枚举、可撤销）
iam.scs_sessions          SCS 的 Cookie 存储
iam.audit_events          审计
```

三条值得知道的设计

**密码单独一张表**：登录失败计数要用 `FOR UPDATE` 锁行。合进 `users` 的话，每次登录都会锁用户行，挡住所有读操作。

**两张会话表不是重复**：`scs_sessions` 只存 Cookie 指向的会话 ID，`user_sessions` 存完整状态（token 摘要、账号版本、双超时）。Cookie 带指针，状态在库里。

**绑定直接指向全局对象**：`user_roles(user_id, role_id)` 的主键就是唯一绑定，`role_permissions(role_id, permission_key)` 指向角色与权限目录。没有作用域列，从存储层杜绝重复授予与悬空权限。

### 角色和用户的状态语义

角色没有 status（存在即可用，内置角色只读）。用户有 `active` / `disabled`。

这个简化是有意的。多一个状态就多一套状态机，每个查询都要带 `status='active'` 条件，而这些条件经常被忘记。删掉之后「存在即有效」是唯一需要记住的规则；用户状态只用于停用账号并撤销其会话。

## 别的服务怎么用

### 一个客户端，一个方法

```go
import "github.com/EayonLee/SecPlexus/src/packages/go/iamclient"

client, _ := iamclient.New(
    "http://127.0.0.1:8081",
    os.Getenv("SPX_CONSOLE_IAM_TOKEN"),
)
defer client.Close()

decision, err := client.Authorize(
    ctx, r,
    iamclient.PermissionIAMUsersRead,
)
```

`r` 是原始 `*http.Request`，client 从里面取 Cookie、CSRF header、method、path、Origin。调用方不用自己拼。

返回的 `decision`

```go
decision.Allowed       // 允不允许
decision.DecisionID    // 排障和审计关联
decision.Principal     // 用户是谁
decision.Scope         // {kind:"platform"}，扁平模型只有这一个范围
```

`iamclient` 包里除 `Authorize` 外只有 `Close`。类型定义在 `types.go`，权限常量是生成代码。

### 两条铁律

**服务凭证不能替代登录用户。** 每个调用方有自己的 token，在 IAM 的 `service_clients` 里配置，同时限定能问哪些 action。token 只证明「我是 Console」，不证明「我是张三」。IAM 验完服务凭证后，还要再验用户 Cookie。

**用户 Cookie 不能替代服务凭证。** 反过来也不行。两条通道独立，缺一不可。

配置长这样

```yaml
service_clients:
  console:
    token: "${SPX_CONSOLE_IAM_TOKEN-}"
    actions: [iam.users.read]
```

`actions` 是白名单，不在列表里的 action 一律拒绝。token 用 SHA-256 摘要比对，常数时间比较，防时序攻击。

### 当前接入情况

**已接入，Console。** 它调 IAM 验证会话与 SSE 授权，是唯一配了 `service_clients` 的模块。

**规划中，其余模块。** 按架构设计，`llm-gateway`、`collector`、`correlator`、`agent-runtime`、`soar`、`scheduler` 等后续模块都会各自接入 IAM，方式完全一样，在自己的 `config.yaml` 里配一个 `service_clients` 条目，拿一个独立 token，声明自己需要问哪些 action。

**它们不经过 Console。** Console 是安全运营业务模块，不是网关，不是权限中转站。每个模块直连 IAM。

模块定义还在演进，具体有哪些、叫什么名字，以后可能变。但接入方式不会变，因为它是 IAM 的契约，不是某个模块的实现细节。

新增一个模块的完整动作只有两步

```yaml
# 1. IAM 侧加一段配置
service_clients:
  your-module:
    token: "${SPX_YOUR_MODULE_IAM_TOKEN-}"
    actions: [iam.users.read]
```

```go
// 2. 调用方用 iamclient
client, _ := iamclient.New(iamURL, yourToken)
decision, err := client.Authorize(ctx, r, iamclient.PermissionIAMUsersRead)
```

不用改 IAM 代码，不用重启其他模块。

### 调用方必须做和不该做

必须做，需要哪个权限就在自己的路由声明里写死权限键，不从请求参数或浏览器提交的值取 action。IAM 只回答「这个已声明的动作允不允许」。

必须做，真实资源的归属自己查。IAM 说「这个用户有 `iam.users.read`」，不代表「这条业务记录他能看」。

不该做，读 IAM 的表。不该做，导入 IAM 的 `internal` 包。不该做，自己实现一套角色判断。

Console 就是这么接的，`src/apps/console/internal/handler/server.go` 里只有 `s.IAM.Authorize` 调用，其余全是业务逻辑。

## 怎么跑起来

### 本地开发

从仓库根目录，用统一入口

```sh
cp .env.example .env
chmod 600 .env
# 填数据库密码、管理员密码、CSRF key、Console 的 IAM token

pnpm compose:up postgres redis   # 或者连已有实例
make dev
```

`make dev` 先跑迁移和初始化，再启动 IAM、Console、Web。

### 七个命令

```sh
go run ./src/apps/iam/cmd check-config         --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd serve                --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd migrate              --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd status               --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd maintain             --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd revoke-after-restore --config src/apps/iam/config.yaml
go run ./src/apps/iam/cmd reset-password --user-id <uuid> --config src/apps/iam/config.yaml
```

`check-config` 离线校验，不连数据库不建日志。改完配置先跑这个。

`status` 只读，不因状态查询隐式建表。

`maintain` 清理过期数据，每次每表最多 1000 条，由部署侧定时调度。

`revoke-after-restore` 必须在数据库恢复后、开放流量前跑。否则备份里的旧会话会复活。

`reset-password` 是运维兜底，交互式无回显输入，走和普通改密一样的用例，撤销会话、写审计，不会顺便提权。配置托管的账号不走这条路径。

### 首次启动做了什么

```
迁移建表
      ↓
同步权限目录 + 内置超级管理员角色（builtin=true）
      ↓
写入/更新配置管理员（configured=true）并授予超级管理员角色
      ↓
校验 schema 版本与唯一超管存在
      ↓
serve 就绪
```

没有安装页面。管理员是配置给的，直接去登录页登。

迁移、种子、首账号初始化都可重试，并发取 advisory 锁。配置管理员与内置角色在初始化阶段落定；改配置时更新凭据、递增账号版本并撤销旧会话。

### 配置

配置在 `src/apps/iam/config.yaml`，每项都有中文注释说明是什么、默认值为什么这么定、改了会怎样。

必填四项，由环境变量或私有 `.env` 提供

```
SPX_IAM_CSRF_KEY            32 字节，签名 CSRF token
SPX_IAM_BOOTSTRAP_PASSWORD  超管密码，12–128 字符
SPX_CONSOLE_IAM_TOKEN       Console 调 IAM 凭证，32–512 字节
SPX_POSTGRES_PASSWORD       数据库密码
```

关键默认值

| 配置 | 默认 | 约束 |
|---|---|---|
| `session_idle_timeout` | 30m | > 0 |
| `session_absolute_timeout` | 8h | ≥ idle，≤ 168h |
| `reauth_window` | 30m | > 0 |
| `login_max_failures` | 5 | 1–100 |
| `login_lock_duration` | 15m | > 0 |
| `audit_days` | 365 | 1–36500 |
| `session_days` | 30 | 1–36500 |

改配置要重启，不热更新。

## API 一览

25 个业务路由加 1 个服务路由，全部在 `/api/v1/iam` 下。另有 `/health/ready` 就绪检查。

**认证**

```
GET  /auth/context           CSRF 上下文
POST /auth/login             登录
POST /auth/logout            登出
POST /auth/reauthenticate    二次认证
```

**本人**

```
GET   /me                    账号、角色与有效权限
PATCH /me                    更新时区偏好
POST  /me/password           本人改密
```

**用户**

```
GET    /users                      列表（含角色）
POST   /users                      创建
GET    /users/:id                  详情
PATCH  /users/:id                  修改
DELETE /users/:id                  删除
PUT    /users/:id/roles            设置角色 {role_ids}
POST   /users/:id/reset-password   重置密码
```

**角色**

```
GET    /roles                      列表（含内置行、用户数）
POST   /roles                      创建
GET    /roles/:id                  详情（含权限键）
PATCH  /roles/:id                  修改名称与说明
DELETE /roles/:id                  删除（有用户持有或内置角色则拒绝）
PUT    /roles/:id/permissions      全量替换角色权限
```

角色详情已经返回权限键，所以没有单独的权限读取端点；持有某角色的用户由用户列表的 `roles` 字段表达，
所以没有 `/roles/:id/users`。授权影响预览（`/permission-preview`）没有前端调用方，已删除——
少一个端点就少一处需要鉴权与维护的契约。

**目录与审计**

```
GET  /permissions            权限目录（含 can_delegate）
GET  /audit-events           审计日志（单一列表，无范围裁剪）
```

**服务接口**

```
POST /authorize              服务凭证 + action 白名单 + 原用户上下文
```

`/me` 返回 `principal`（含 `user_id`、`session_id`、`username`、`display_name`、`configured`、`auth_epoch`）、`roles`、`permissions` 与时区偏好。

统一响应 `{code, message, data, request_id}`。401 会话失效、403 授权拒绝、409 冲突、429 限流、503 依赖故障。

## 数据

### 审计

`iam.audit_events` 记录谁、什么时候、对什么、做了什么、结果如何。

```sql
actor_type      -- user / system / deployment_operator
actor_id        -- 操作者
actor_name      -- 快照，防止用户改名后追溯困难
action          -- 如 iam.roles.manage
target_type     -- user / role / session / platform
target_id       -- 目标 ID
outcome         -- success / denied / failure
reason_code     -- 受控原因码
request_id      -- 跨服务关联
decision_id     -- 授权判定关联
metadata        -- 白名单字段，8KiB 以内
```

审计不带范围维度，也不按范围裁剪：单一列表按 `action`、`actor_id`、`outcome` 与时间过滤。审计和业务变更在同一事务，要么都成要么都不成。审计写入失败会导致安全变更回滚。授权变更只记录差异（`permissions_added` / `permissions_removed`、`roles_added` / `roles_removed`），避免整份集合撑破 metadata 上限。

不记录密码、Cookie、原始请求体。

### 保留期

| 数据 | 默认 | 说明 |
|---|---|---|
| 审计 | 365 天 | 《网络安全法》第二十一条要求日志留存不少于六个月。180 天正好是法定下限没有余量，所以默认一年 |
| 已失效会话 | 30 天 | 只影响审计排查用的历史，会话有效性由 `absolute_expires_at` 判定 |
| SCS Cookie 行 | 过期即清 | 临时存储 |

都可配置。清理是物理删除，删掉之后追溯不了。

## 实现结构

```
src/apps/iam/
  cmd/main.go              命令解析、配置加载、选择启动或维护入口
  config.yaml              配置、环境占位、默认值
  migrations/              版本化 SQL，已执行的脚本不修改
  internal/
    bootstrap/             创建依赖、组装服务、失败清理
    config/                类型化加载和校验，不建连接
    router/                路由注册
    handler/               请求绑定、统一错误映射
      management/          用户、角色、审计管理
    middleware/            Session 提交屏障、CSRF、服务凭证、契约校验
    service/               用例、最终授权、事务、审计顺序
    repository/            sqlc 查询、锁、初始化、维护
    authentication/        凭证用例、SCS、可撤销逻辑会话、限流
    authorization/         权限求值和委派规则（不依赖 HTTP 和存储）
    account/               账号、密码、邮箱、显示名、时区规则
    enum/                  数据库中封闭取值的 Go 常量（status、state、actor_type…）
    model/                 HTTP DTO 和请求边界校验
    testutil/              集成测试的隔离数据库
```

分层原则，handler 只管 HTTP，service 管事务和业务不变量，repository 管 SQL，authorization 是纯函数不碰 IO。
目录、依赖方向与命名遵循[后端工程结构规范](../../../docs/development/backend-structure.md)。

表结构、封闭取值、注释与索引遵循[数据库设计规范](../../../docs/development/database-design.md)：
每个业务表与列都有注释（`migrations/00002_table_comments.sql`），`status`/`state`/`risk_level`/`actor_type`/`outcome`
的取值在 `internal/enum` 定义一次，Go 代码比较常量而不是字符串字面量。

想换授权求值引擎（比如将来上 Casbin 或 OpenFGA），改 `authorization` 一个包就行。

### 会话提交屏障

IAM 独有的一处机制。

SCS 的 `LoadAndSave` 不能保证「会话写库失败时不发送成功响应」，所以 IAM 的非流式响应先写进缓冲区，确认 SCS 持久化成功后才发给浏览器。

失败时补偿撤销已创建的逻辑会话，然后返回 503。不会出现「浏览器拿到 Cookie 但服务端没这个会话」的哑状态。

Console 的 SSE 是流式，不经过这个屏障。

### 管理写操作用统一事务边界

所有管理写走同一个 `Manage` 方法

```
取 advisory 事务锁（串行化低频管理写）
  ↓
按 UUID 顺序锁涉及的用户
  ↓
重新验证操作者会话版本和当前权限
  ↓
校验目标（内置角色/配置管理员不可变）
  ↓
执行业务变更 + 写审计
  ↓
提交
```

串行化低频管理写，比在多个入口分别处理竞态简单。登录和普通业务读不持这把锁。

### 登录防护

两道独立的闸门，避免把「暴力破解」和「账号锁定」混成一个机制：

| 维度 | 机制 | 配置 |
| --- | --- | --- |
| 账号 | 连续失败 5 次锁定 15 分钟，到期自动解锁 | `login_max_failures`、`login_lock_duration` |
| 来源 | 每 IP 每分钟 60 次认证请求 | `login_ip_rate` |
| 账号 | 每 15 分钟 20 次认证请求 | `login_account_rate` |

**密码确认的失败不计数。** 操作者已经持有有效会话，把输错的 sudo 密码计入锁定，
等于让一次手滑把管理员锁在门外——这是可用性缺陷，不是安全收益。

### 会话只存认证状态

`user_sessions` 里没有 IP 和 User-Agent：没有设备管理界面时这两列没有消费者，留着只是"以后可能有用"的
猜测。登录来源写进登录审计的 `metadata.client_ip`，溯源时按 `request_id` 与接入层日志对齐——
审计是取证事实的归属地，会话表只回答"这个会话还有效吗"。

审计表另有追加写约束：触发器拒绝 `UPDATE`，`DELETE` 只在保留期清理的事务里通过
`iam.audit_purge` 变量放行。代码里没有任何路径能改写历史。

### 内置数据保护

平台始终保留内置超级管理员角色与至少一名持有它的 active 账号。会破坏这条的写操作返回 409，并发场景也覆盖：

| 不变量 | 结果码 |
| --- | --- |
| 内置角色不可改名、改说明、改权限、删除 | `iam.builtin_role_immutable` |
| 配置管理员不可删除、停用、改密、摘除内置角色 | `iam.configured_user_immutable` |
| 必须留下至少一名启用中的超级管理员 | `iam.last_super_admin` |
| 不能删除或停用当前登录账号 | `iam.self_target_forbidden` |
| 仍有用户持有的角色不可删除 | `iam.role_in_use` |
| 角色名全局唯一 | `iam.role_name_taken` |

配置管理员的内置角色由部署配置保证：即便还有别的超级管理员，页面也不能把它摘掉——否则下次启动会补回，
使用者会看到「改了又变回去」。其他角色按需创建与删除。

删除账号不再被历史归属卡住：`roles.created_by` 与 `user_roles.created_by` 是 `ON DELETE SET NULL`，
删账号只把创建人置空，角色绑定与审计都保留；凭据、会话与角色绑定随主体级联删除。

## 几个容易被误解的点

**它不是 OAuth 服务器。** 没有 client credentials、没有 scope、没有 refresh token 这套 OAuth 词汇。有登录、有会话、有滚动续期和令牌轮换，但不是 OAuth 协议。

**它也做用户登录。** `POST /auth/login` 是真账号密码登录，返回 HttpOnly Cookie。不只是服务间认证。

**它不缓存授权结果。** 每次判定都读数据库。将来如果压测证明是瓶颈，也只会考虑缓存拒绝结果，不会缓存允许结果。

**Console 不是必经中转。** 其他模块要验权限直接调 IAM，不用经过 Console。

**角色是全局的。** 没有作用域列，也没有「同一角色在不同范围表现不同」的语义；一个角色被哪些用户持有由 `user_roles` 决定。

**用户的角色与用户的停用语义不同。** 角色没有 status（存在即可用，内置角色只读）；用户有 active/disabled，停用即撤销其全部会话，而改显示名称或邮箱不撤销会话。

## 相关文档

- [技术架构](../../../docs/design/architecture/02-technical-architecture.md) 全景服务划分和数据流
- [配置指南](../../../docs/development/configuration.md) 字段、优先级、重启要求
- [测试指南](../../../docs/development/testing.md) 怎么跑单元、集成、E2E
- [角色管理开发示例](../../../docs/development/role-management.md) 从协议到前端的一条完整链路
- [日志与错误定位](../../../docs/development/observability.md) 用 request_id 串起跨服务排障
