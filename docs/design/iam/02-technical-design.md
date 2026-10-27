# IAM 技术设计

| | |
|---|---|
| 文档编号 | SPX-DESIGN-IAM-002 |
| 状态 | 待评审 |
| 上游文档 | [01-requirements.md](01-requirements.md)（需求编号 FR-x / NFR-x 在本文追踪） |
| 技术栈约束 | Python 3.12 · FastAPI · PostgreSQL 16 · Redis 7（架构总纲已定） |

> 阅读对象：开发、架构、测试。本文回答"怎么做"——领域模型、认证与授权设计、数据隔离实现、API、选型理由、安全考量、扩展与维护 SOP。

---

## 1. 设计原则

| # | 原则 | 落点 |
|---|---|---|
| P1 | **PDP/PEP 分离**：所有权限的定义、判定、管理收敛于 iam（策略决策点）；执行分布在三个检查点（API 中间件 / 数据层 RLS / 前端渲染） | §3 架构、§7、§8 |
| P2 | **正确性由框架保证，不靠人的记忆**：开发者声明式接入（权限点装饰器、org_id 列），隔离与校验自动生效 | §7.2、§8.3、§14 |
| P3 | **双保险隔离**：应用层自动过滤为主 + 数据库 RLS 兜底，各自独立可测 | §8 |
| P4 | **验签不依赖 iam 进程存活**（NFR3）：策略快照本地缓存，iam 宕机不影响已登录用户 | §7.3 |
| P5 | **加法演进**：ABAC / SCIM / SAML / Zitadel 外包认证全部为叠加而非重构 | §14.4 |

## 2. 需求追踪矩阵

| 需求 | 设计章节 | 需求 | 设计章节 |
|---|---|---|---|
| A1–A3 认证/策略/锁定 | §5.1、§5.4 | D1–D6 授权 | §6 |
| A4 MFA | §5.5 | E1–E4 会话/Token/JWT | §5.2、§5.6 |
| A5/A6 OIDC/SAML | §5.7 | F1–F4 数据隔离 | §8 |
| A7/A8 会话管理 | §5.3、§5.8 | G1–G5 管理界面 | §9（API） |
| B1–B3/C1–C4 组织与用户 | §4 领域模型 | H1–H2 审计/自举 | §10、§12 |
| — | — | NFR1–NFR7 | §5、§7、§12、§13、§15 |

## 3. 总体架构

```
┌────────────────────────────────────────────────────────────────────┐
│  iam（src/apps/iam；当前为 Go 应用基础目录）                         │
│                                                                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ │
│  │ authn    │ │ directory│ │ authz    │ │ token    │ │ admin    │ │
│  │ 认证模块  │ │ 目录模块  │ │ 授权模块  │ │ 令牌模块  │ │ 管理API   │ │
│  │ 登录/锁定 │ │ org/user │ │ RBAC/权限 │ │ 会话/JWT/ │ │ org/用户/ │ │
│  │ MFA/OIDC │ │ /group   │ │ 点/菜单派生│ │ APIToken │ │ 角色管理  │ │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ │
│       └────────────┴─────┬──────┴────────────┴────────────┘       │
│                     PostgreSQL（权威存储）+ Redis（会话/锁定/缓存）     │
└──────────────────────────┬─────────────────────────────────────────┘
                           │ 签发与分发：会话 cookie / 服务间 JWT / 权限快照
     ┌─────────────────────┼──────────────────────┐
     ▼ 执行点①API          ▼ 执行点②数据            ▼ 执行点③前端
  各服务 FastAPI 中间件     SQLAlchemy 自动过滤      web /me/permissions
  （contracts 库提供        + PG RLS 策略           按权限声明渲染
   get_ctx / require）
```

三条总线（contracts 库承载，全部服务的唯一接入姿势）：

| 总线 | 机制 | 开发者姿势 |
|---|---|---|
| 上下文总线 | 中间件验签 → `RequestContext{user_id, org_id, roles, perms}` | `ctx = Depends(get_ctx)` |
| 数据访问总线 | 事务级自动 `SET LOCAL app.org_id` + ORM 自动 org 过滤 | 新表带 `org_id` 列即得 |
| 事件总线 | Redis Streams 消息头带 `org_id` | 生产者填头、消费者还原上下文 |

## 4. 领域模型与数据库设计

### 4.1 实体关系

```
orgs 1───n org_memberships n───1 users 1───n oidc_identities
  │              │(每org一角色)        │
  │              └─────→ roles        1───n api_tokens
  1───n groups                          1───n totp_credentials
        │
        n───1 roles
roles n───n permissions（role_permissions）
users n───n groups（group_members，组授权与用户授权取并集）
```

### 4.2 核心表结构（DDL 级设计，迁移由 Alembic 管理）

```sql
-- 组织（权威目录）。归档不删除：archived_at 置位后只读
CREATE TABLE iam_orgs (
    org_id       UUID PRIMARY KEY,                -- 应用层生成 UUIDv7
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,            -- URL 友好标识
    description  TEXT,
    settings     JSONB NOT NULL DEFAULT '{}',          -- 组织级设置（B3，P1 启用：默认角色/保留参数覆盖）
    status       TEXT NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','archived')),
    created_by   UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at  TIMESTAMPTZ
);

-- 用户。停用不物理删除（保留审计关联）
CREATE TABLE iam_users (
    user_id        UUID PRIMARY KEY,
    username       CITEXT NOT NULL UNIQUE,        -- 大小写不敏感
    email          CITEXT UNIQUE,
    display_name   TEXT NOT NULL,
    password_hash  TEXT,                          -- argon2id PHC 串；OIDC-only 用户为 NULL
    password_updated_at TIMESTAMPTZ,
    is_platform_admin BOOLEAN NOT NULL DEFAULT false,  -- 平台管理员（跨org）
    status         TEXT NOT NULL DEFAULT 'active'
                   CHECK (status IN ('active','disabled')),
    must_change_password BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at    TIMESTAMPTZ
);

-- 组织成员关系：一个用户多组织，每组织一个角色，一个默认
CREATE TABLE iam_org_memberships (
    org_id      UUID NOT NULL REFERENCES iam_orgs(org_id),
    user_id     UUID NOT NULL REFERENCES iam_users(user_id),
    role_id     UUID NOT NULL REFERENCES iam_roles(role_id),
    is_default  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);
-- 恰好一个默认组织（部分唯一索引保证）
CREATE UNIQUE INDEX one_default_org_per_user
    ON iam_org_memberships (user_id) WHERE is_default;

-- 角色。scope='platform' 为平台级（platform_admin），其余 org 级
CREATE TABLE iam_roles (
    role_id     UUID PRIMARY KEY,
    org_id      UUID NULL REFERENCES iam_orgs(org_id),  -- NULL = 平台预置角色
    name        TEXT NOT NULL,
    description TEXT,
    scope        TEXT NOT NULL DEFAULT 'org' CHECK (scope IN ('org','platform')),
    is_builtin   BOOLEAN NOT NULL DEFAULT false,       -- 预置角色不可删
    mfa_required BOOLEAN NOT NULL DEFAULT false,       -- 强制 MFA（A4/G4，按角色配置）
    UNIQUE NULLS NOT DISTINCT (org_id, name)
);

-- 权限点：resource:action，服务启动时注册，iam 校验清单（防拼写错误，D1）
CREATE TABLE iam_permissions (
    permission_id  UUID PRIMARY KEY,
    code           TEXT NOT NULL UNIQUE,          -- 如 'cases:write'
    description    TEXT NOT NULL,
    service        TEXT NOT NULL                  -- 声明方服务名
);

CREATE TABLE iam_role_permissions (
    role_id       UUID NOT NULL REFERENCES iam_roles(role_id),
    permission_id UUID NOT NULL REFERENCES iam_permissions(permission_id),
    conditions    JSONB NULL,                     -- ABAC 预留（N3：加法演进）
    PRIMARY KEY (role_id, permission_id)
);

-- 用户组（组织内批量授权；不做数据隔离）
CREATE TABLE iam_groups (
    group_id UUID PRIMARY KEY,
    org_id   UUID NOT NULL REFERENCES iam_orgs(org_id),
    name     TEXT NOT NULL,
    UNIQUE (org_id, name)
);
CREATE TABLE iam_group_members (
    group_id UUID NOT NULL REFERENCES iam_groups(group_id),
    user_id  UUID NOT NULL REFERENCES iam_users(user_id),
    PRIMARY KEY (group_id, user_id)
);
CREATE TABLE iam_group_roles (
    group_id UUID NOT NULL REFERENCES iam_groups(group_id),
    role_id  UUID NOT NULL REFERENCES iam_roles(role_id),
    PRIMARY KEY (group_id, role_id)
);

-- API Token（E2）：opaque，前缀可识别 + 哈希存储，明文只在签发时返回一次
CREATE TABLE iam_api_tokens (
    token_id     UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES iam_users(user_id),
    org_id       UUID NOT NULL REFERENCES iam_orgs(org_id),  -- token 绑定组织上下文
    name         TEXT NOT NULL,
    token_prefix TEXT NOT NULL,                  -- 展示/检索用前 8 字符
    token_hash   BYTEA NOT NULL,                 -- SHA-256(全 token)
    permission_scope JSONB NOT NULL,             -- 权限点子集（≤ 签发者权限）
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,                   -- 每次使用更新，去抖 5 分钟（避免高频调用写放大）
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- OIDC 身份绑定（A5）：一个外部身份 ↔ 一个本地用户
CREATE TABLE iam_oidc_identities (
    provider    TEXT NOT NULL,                   -- 配置的 provider 标识
    subject     TEXT NOT NULL,                   -- IdP 侧 sub
    user_id     UUID NOT NULL REFERENCES iam_users(user_id),
    linked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);

-- TOTP（A4）：密钥加密存储（见 §5.5），启用后登录强制第二因子
CREATE TABLE iam_totp_credentials (
    user_id    UUID PRIMARY KEY REFERENCES iam_users(user_id),
    secret_enc BYTEA NOT NULL,                   -- AES-GCM(KEK) 加密的 base32 secret
    confirmed  BOOLEAN NOT NULL DEFAULT false,   -- 绑定确认后才生效
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**不落 PG 的运行态**（Redis，命名规范 `iam:{域}:{键}`）：会话（`iam:session:{sid}`，内容见 §5.3）；刷新令牌族（`iam:refresh:{token_hash}`，family id + 状态，E4 重用检测）；登录失败计数（`iam:login_fail:{username}` / `iam:login_fail_ip:{ip}`，TTL=锁定窗口）；权限快照缓存（`iam:permissions:{user_id}:{org_id}`，见 §7.3）；TOTP 防重放（`iam:totp_used:{user_id}:{step}`）。

> **org 归属自己管自己**：iam_orgs 等目录表不带 org_id（它们就是 org 的定义处）；全平台**业务表**带 org_id 并启用 RLS（§8）。目录表的访问控制走 API 权限点（orgs:manage 等），不需要行级隔离。

**归档语义（B1）**：组织归档是业务动作不是权限动作——中间件在还原上下文时检查 org status，`archived` 组织的请求：读类（GET）正常放行（数据可回看）、写类（POST/PATCH/DELETE）返回 `409 ORG_ARCHIVED`。归档同时吊销该组织全部会话与 API Token（触发式，一次性）。

### 4.3 全局层与隔离层的表归属（F2 三层可见性）

| 层 | 表（示例） | org_id | RLS |
|---|---|---|---|
| 全局共享 | iocs、rule 库、att&ck 映射 | 无此列 | 不启用；写权限 `intel:write`（仅平台管理员，F4） |
| 组织隔离·业务 | alerts、cases、investigations、incident 元数据 | NOT NULL | 启用 |
| 组织隔离·经验 | institutional_memory、entities | NOT NULL | 启用；IOC 命中结果由服务层回流全局层 |

## 5. 认证设计（AuthN）

### 5.1 密码凭据（A1/A2）

- 哈希算法 **Argon2id**（OWASP 首选），参数按 OWASP 基准：`m=64 MiB, t=3, p=4`（服务启动自检耗时，>1s 降级 `m=32,t=3` 并告警）。
- PHC 格式串整列存储，含盐与参数——升级参数时旧哈希在下次登录时透明重哈希。
- 密码策略（A2）在设置/修改/重置三处统一校验：长度 ≥12、四类字符中三类、top-1000 弱口令黑名单（随包分发，可扩展）。
- 登录响应统一模糊错误 `invalid credentials`（不泄露用户是否存在）；时序安全：用户不存在也执行一次哈希运算。

### 5.2 凭据类型总览（三种身份，四种凭据）

| 身份 | 凭据 | 存储 | 吊销 |
|---|---|---|---|
| 浏览器用户 | 会话 cookie（opaque 256bit sid） | Redis（哈希不可逆存 key） | 即时（删 key） |
| 浏览器用户 | 刷新令牌（opaque，仅 httpOnly cookie） | Redis 族结构 | 即时 + 重用检测（§5.6） |
| 机器/脚本 | API Token（`spx_` 前缀 opaque） | PG 存 SHA-256 | 即时（revoked_at） |
| 内部服务 | 短效 JWT（RS256） | 不落地（15 min） | 短效自然过期 + 吊销名单（jti 黑名单，P1） |

选型依据：**浏览器会话用服务端 session 而非长效 JWT**——OWASP Session Management 的结论：需要即时吊销的场景（登出、强制下线、停用账号）服务端会话是正解；JWT 的无状态扩展性在几百用户量级毫无收益，却牺牲吊销能力。

### 5.3 会话管理（A7/E1，对齐 [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)）

- Cookie 属性：`HttpOnly; Secure; SameSite=Lax; Path=/`（跨前端同源部署不需要 None）。
- sid：`secrets.token_urlsafe(32)`（256 bit），仅作 Redis key，任何地方不回显。
- 生命周期：空闲 TTL 8h（每次请求续期）、绝对过期 24h；登录/切组织时**重签**（防会话固定）。
- 会话内容：`user_id, org_id(当前), permissions_version, issued_at`——**角色与权限点都不进会话**（§7.3 缓存层承载，角色变更即时生效），会话只承载身份与当前组织。
- 强制下线/停用账号/吊销 token：直接删 Redis key / 置 status，下一个请求即 401（验收场景 4/5）。

### 5.4 登录防爆破（A3）

```
失败计数（Redis，INCR + TTL）：
  iam:login_fail:{username}       连续 5 次 → 锁定 15 分钟（正确密码也拒绝，模糊提示"账号已锁定"）
  iam:login_fail_ip:{ip}          100 次/15min → IP 限速（429）
锁定/解锁、全部失败尝试 → 审计事件（H1）
```

### 5.5 TOTP 多因子（A4）

- 库：`pyotp`；绑定流程：生成 secret（OTPAUTH URI，二维码）→ 用户提交 6 位确认码 → `confirmed=true` 生效（未确认的 secret 不能用于登录）。
- secret 加密：AES-256-GCM，KEK 来自部署密钥（`SPX_SECRET_KEY` 派生子密钥，HKDF），密文落 `iam_totp_credentials`。
- 校验窗口 ±1 步（30s/步）；防重放：已用 (time-step, user) 记入 Redis 短 TTL。
- 强制 MFA 按角色配置（A4/G4）：`iam_roles.mfa_required`（DDL 见 §4.2），预置 `platform_admin`/`org_admin` 默认 `true`，管理员在登录安全页可改；登录时命中强制角色而未绑定 → 引导绑定流程，未完成不签发会话。

### 5.6 刷新令牌轮换与重用检测（E4，[Auth0 模式](https://auth0.com)）

```
签发：登录 → {会话, 刷新令牌 v1}（族 family_id=F）
刷新：v1 使用 → 作废 v1 → 签发 v2（同族）；30s 宽限窗（网络重试，Okta 实践）
重用检测：已作废的 v1 再次出现 → 判定泄漏 → 吊销整个族 F（含会话）→ 审计告警
```

### 5.7 OIDC / SAML 对接（A5/A6）

- OIDC：`authlib` 作客户端（authorization code flow + PKCE）；管理员配置 `{issuer, client_id, client_secret, scopes, 默认组织, 自动建号开关}`；发现文档自动拉取（`.well-known/openid-configuration`）。
- 账号关联策略（可配）：`link-by-email`（默认，邮箱匹配即绑定）/ `provision`（自动建号入默认组织）/ `deny`（仅预绑定）。
- 登录页渲染已配置的 SSO 按钮；本地账号可并存（NFR7：内网无外联时 OIDC 配置不可达 → 按钮隐藏，本地登录不受影响）。
- SAML（P1）：`python3-saml` 或 OneLogin 库，同一配置模型。

### 5.8 密码修改与重置（A8）

- 自助修改：验证旧密码 → 新密码过策略 → 哈希更新 + 该用户全部会话/刷新族吊销（改密后旧会话全失效）+ 审计。
- 管理员重置：生成一次性随机密码（打印一次）→ `must_change_password=true` → 首登强制改密页。

### 5.9 SCIM 2.0 入站同步（A9，P1 概要设计）

P1 交付，但接口形状 v1 定死（避免后补时破坏兼容）：

- **端点**（iam 暴露 SCIM 服务端，遵循 RFC 7644）：`GET/POST /scim/v2/Users`、`GET/PATCH/DELETE /scim/v2/Users/{id}`、`POST /scim/v2/Groups`（组同步）。Bearer 认证用专用 SCIM Token（长随机串，`settings:write` 管理员签发）。
- **映射**：SCIM User.userName/email/name → `iam_users`；SCIM Groups → `iam_groups` + 成员同步；SCIM `active=false`（或 DELETE）→ 本地用户**停用**（不物理删除，触发 §9 的级联吊销）——语义对齐 C1。
- **冲突策略**：userName 已存在 → 按 `externalId` 匹配更新（upsert）；无 externalId 的冲突返回 SCIM 409；新用户入配置的默认组织与默认角色（组织级设置 B3 的第一个消费者）。
- **自研而非引入 SCIM 框架**：核心就四个资源的 CRUD 映射，引入第三方 SCIM 库（如 python-scim2）的抽象成本高于自写；schema 校验按 RFC 7643 手写 Pydantic 模型。

## 6. 授权设计（AuthZ）

### 6.1 权限点体系（D1）

- 命名规范 **`resource:action`**（`alerts:read`、`cases:write`、`soar:approve`、`orgs:manage`、`memory:admin`、`settings:write`、`audit:read`），resource 复数名词、action 限定动词集（read/write/manage/approve/admin，新动词需评审）。
  - 依据：`action:resource` 与 `resource:action` 是两大主流（[Auth0 社区惯例](https://community.auth0.com/t/permissions-naming-convention/59944)、[GitLab 用 verb_resource](https://docs.gitlab.com/development/permissions/conventions)）；选冒号分隔的 `resource:action` 与全平台已有 API 命名（`/alerts`、`/cases`）一致。
- **注册制**（D1 核心）：各服务在代码中声明 `PERMISSIONS = [PermissionSpec(code="cases:write", description="处置案件"), ...]`；启动时上报 iam，iam 校验与数据库注册清单一致——**未注册的权限点被 API 使用 = 启动失败**（防拼写错误进生产）。新增权限点 SOP 见 §14.1。

### 6.2 预置角色与权限矩阵（D2）

| 权限点 | platform_admin | org_admin | analyst | viewer |
|---|:-:|:-:|:-:|:-:|
| orgs:manage（组织 CRUD） | ✅ | — | — | — |
| users:manage（本组织用户/角色分配） | ✅ | ✅ | — | — |
| settings:write（平台设置） | ✅ | — | — | — |
| intel:write（全局情报写） | ✅ | — | — | — |
| alerts:read / cases:read | ✅ | ✅ | ✅ | ✅ |
| cases:write（研判/立案/时间线） | ✅ | ✅ | ✅ | — |
| soar:approve（处置审批） | ✅ | ✅ | ✅ | — |
| hunt:run（狩猎任务） | ✅ | ✅ | ✅ | — |
| memory:admin（机构记忆管理/纠正） | ✅ | ✅ | ✅ | — |
| connectors:manage（连接器配置） | ✅ | ✅ | — | — |
| audit:read | ✅ | ✅ | — | — |
| iam:tokens（签发 API Token） | ✅ | ✅ | — | — |

> 基线待 PRD 开放问题 1 定稿；矩阵本身是数据不是代码——改矩阵不改代码（D4 验收）。

### 6.3 菜单权限派生（D4）

```
前端路由表：每条路由声明 meta.perm = 'cases:read'（路由级）+ 按钮/操作级 perm
登录/刷新 → GET /me/permissions → { permissions: Set[code], roles: [...], menus: [路由键] }
web 按 menus 渲染导航；直接访问无权限路由 → 403 页
后端仍是权威：前端隐藏只是体验，API 层校验（§7.2）才是边界
```

### 6.4 授权判定算法（authz 模块）

```
user_perms(user, org) =
    permissions(membership(user, org).role)        # 用户直挂角色
  ∪ ⋃ permissions(g.role) for g in groups(user, org)  # 组授权取并集
  ∪ is_platform_admin(user) ? platform_perms : ∅

缓存：iam:permissions:{user_id}:{org_id}（TTL 60s + 变更主动失效 §7.3）
```

## 7. 上下文分发与验签（PDP → PEP①）

### 7.1 JWT（服务间，E3）

- RS256；密钥对启动时生成，**公钥全服务可拉取缓存**（`iam:jwt:pubkey`，轮换支持 kid）。
- Claims：`iss=spx-iam, sub=service:{name}, aud, org_id, jti, iat, exp(15min), permissions`（服务身份的全集权限点）。
- 用途：跨服务 RPC、队列消费者的身份传递（消费侧还原 RequestContext）；**不用于浏览器**（浏览器走会话，§5.2）。

### 7.2 请求中间件（contracts 库，全部服务统一）

```python
# 业务代码的唯一姿势（P2 原则）：
@router.post("/cases/{id}/approve")
async def approve(case_id: UUID, ctx: RequestContext = Depends(get_ctx)):   # ① 上下文
    require(ctx, "soar:approve")                                            # ② 权限点
    ...
```

中间件执行序：解析凭据（cookie sid / API Token / JWT）→ 还原身份与当前组织 → 载入权限快照 → 构造 `RequestContext{user_id, org_id, is_platform_admin, permissions}` 注入依赖上下文。`require()` 失败 → 403（权限点缺失）或 401（凭据无效）；**业务代码中出现裸 `if ctx.perms` 之外的权限判断视为 code review 拦截项**。

### 7.3 权限快照与 iam 宕机可用性（NFR2/NFR3）

```
变更主动失效：任何角色/成员/token 变更 → 删除受影响用户的 permissions 缓存（精确键，写扩散在几百用户量级可忽略）
缓存内容：permissions 集合 + roles（供审计展示）+ 版本号；TTL 60s 兜底
```

**缓存 miss 的重建路径（NFR3 严谨性的关键）**：iam 是单体架构中的一个角色而非独立微服务——contracts 库提供 **`iam_readonly` 模块**（直读 PG 的 iam 目录表，不经过 iam 进程的 HTTP）。中间件的判定顺序：

```
凭据验证 → Redis 会话（活）→ 权限缓存命中？
  ├─ 命中 → 使用缓存
  └─ 未命中 → contracts.iam_readonly 直查（目录表 + 成员 + 角色 + 权限点，4 张表一次 join）
              → 回填缓存 → 使用
```

因此 iam **进程**宕机（≠ PG/Redis 宕机）时：已登录用户判定不受影响（验收场景 5 成立）；仅新登录的密码校验、管理写操作不可用——与 NFR3 的承诺精确一致。若 PG 整体宕机则全平台不可用，不在此讨论范围。

### 7.4 组织切换（C2/G5）

`POST /auth/switch-org {org_id}`：校验成员关系（否则 403）→ 会话重签（新 org 上下文 + 权限快照换装）→ 前端刷新 /me/permissions。切换是显式动作，**不做请求级 org 头**（防越权面：org 只能来自服务端会话，绝不来自客户端参数——与"组织上下文从凭证不从 header"同款纪律）。

## 8. 数据隔离实现（PEP②）

### 8.1 RLS 策略（F3，[AWS canonical 模式](https://aws.amazon.com/blogs/database/multi-tenant-data-isolation-with-postgresql-row-level-security)）

```sql
-- 迁移模板（所有组织隔离层业务表统一适用）
CREATE POLICY org_isolation ON alerts
    USING (org_id = current_setting('app.org_id')::uuid);
ALTER TABLE alerts FORCE ROW LEVEL SECURITY;      -- 表Owner也逃不过

-- 连接角色（应用绝不用超级用户连接）
CREATE ROLE spx_app NOLOGIN;                       -- 由迁移维护
GRANT spx_app TO spx_runtime;                      -- spx_runtime: NOBYPASSRLS（默认）
```

### 8.2 应用层自动注入（主防线，AiSOC 事故教训的对策）

```python
# contracts 库：数据访问总线（全服务自动生效，业务代码零感知）
@event.listens_for(Session, "after_begin")          # ① 每个事务开始
def set_org_guc(session, tx, conn):
    if ctx := current_request_context():
        conn.exec_driver_sql("SELECT set_config('app.org_id', %s, true)",
                             (str(ctx.org_id),))    # true = 事务作用域，连接池归还不泄漏

@event.listens_for(ORMSession, "do_orm_execute")    # ② ORM 查询自动追加过滤
def add_org_filter(execute_state):
    if execute_state.is_select and entity_has_org_id(execute_state):
        execute_state.statement = apply_org_filter(...)
```

**为什么双保险都要**【代码·AiSOC】：RLS 依赖 GUC，一条代码路径忘了 `SET app.org_id` → 策略还在却跨组织读穿。因此：应用层自动注入为主（开发者无感知、不可能忘），RLS 为兜底（应用有 bug 也漏不了）——**两者各自独立可测**（验收场景 1/6 与 §14 隔离测试）。

### 8.3 三层可见性的实现口径（F2）

| 层 | 实现方式 |
|---|---|
| 全局共享表 | 无 org_id 列、无 RLS；API 层 `intel:read` 公开、`intel:write` 仅平台管理员 |
| 组织隔离表 | org_id NOT NULL + RLS + 自动注入（§8.1/8.2） |
| 记忆/实体档案回流全局 | 服务层显式写全局表（如 IOC 命中），写操作走服务身份 JWT 并审计——**回流是代码行为不是权限行为**，不开"跨组织读"口子 |

### 8.4 org 归属的落定（F1）

collector 的连接器配置含 `org_id`（必填）；归一化落库时写入，之后任何 API 不可修改该字段（模型层只读属性 + 审计）。资产映射覆盖：`asset_org_mappings` 表（资产 → org，**属主为 collector 的配置域**，iam 只提供 org 的权威定义——归属规则引用目录但不归目录管），命中则用映射值——归属规则集中一处，可解释。

## 9. API 设计（核心端点）

| 分组 | 端点 | 权限点 | 备注 |
|---|---|---|---|
| 认证 | `POST /auth/login` `POST /auth/logout` `POST /auth/refresh` `POST /auth/switch-org` | 公开（限速） | 登录支持 password+totp 两段 |
| 认证 | `GET /auth/sessions` `DELETE /auth/sessions/{id}` | 会话本人 / `users:manage` | A7 在线会话管理 |
| 自身 | `GET /me` `GET /me/permissions` `GET /me/orgs` `PUT /me/password` `POST /me/totp` | 会话本人 | D4 前端渲染数据源；`/me/orgs` 是组织切换器（C2/G5）的数据源 |
| 组织 | `GET/POST /orgs` `PATCH /orgs/{id}` `POST /orgs/{id}/archive` | `orgs:manage` | B1；列表对 org 成员可见（过滤本组织） |
| 组织 | `GET/POST /orgs/{id}/members` `PATCH/DELETE /orgs/{id}/members/{user}` | `users:manage` | C2 成员与角色分配 |
| 用户 | `GET/POST /users` `PATCH /users/{id}` `POST /users/{id}/reset-password` | `users:manage` | C1/C3；停用 = 级联吊销该用户**全部会话 + 全部 API Token**（会话删 Redis key，Token 批量置 revoked_at）+ 更新 last_login_at 冻结 |
| 用户组 | `GET/POST /orgs/{id}/groups` `PUT /groups/{id}/members` `PUT /groups/{id}/roles` | `users:manage` | B2 |
| 角色 | `GET /roles` `GET /roles/matrix` | `users:manage` | **读**矩阵对 org_admin 开放（分配角色时需要看，G2） |
| 角色 | `PUT /roles/{id}/permissions` | `orgs:manage`（预置角色仅平台管理员） | 矩阵**写**为高危操作，二次确认（H3 的前身） |
| 权限点 | `GET /permissions` | 会话本人 | D6 全量清单 |
| Token | `GET/POST /me/tokens` `DELETE /tokens/{id}` | `iam:tokens` | E2；签发响应含明文（仅一次） |
| SSO | `GET/PUT /settings/oidc` `GET /auth/oidc/{provider}/login` `GET /auth/oidc/{provider}/callback` | `settings:write` / 公开 | A5 |
| 审计 | `GET /audit/iam-events` | `audit:read` | H1（读走 audit 链） |

规范：全部 REST + UUID 路径参数；错误体 `{code, message, request_id}`；分页 `limit/after`；变更类端点返回审计事件 id。

### 8.5 API Token 的权限范围校验（E2）

签发时逐点校验 `permission_scope ⊆ 签发者当前 permissions`（按签发者**此刻**的权限，不是历史权限）；签发后签发者权限被收缩**不追溯**已签发 Token（记录审计可查）——追溯式收缩需要吊销重签，属管理动作而非自动行为。Token 使用时的判定同用户：中间件按 Token 的 scope 而非签发者权限判定。

## 10. 审计（H1）

- 事件集：登录成功/失败/锁定、MFA 绑定/校验失败、组织/用户/组/角色/成员/权限矩阵/API Token（签发/吊销/级联吊销）/SSO 配置的全部变更、组织归档、组织切换、强制下线、刷新令牌重用检测命中。
- 载体：复用全平台 `audit` 哈希链库（append-only），iam 事件 `category='iam'`；记录含 actor（用户或服务身份）、对象、变更摘要（前后值 diff 的 JSON）、来源 IP、request_id。
- 不可抵赖：链式哈希，验证接口随 audit 库提供。

## 11. Bootstrap 自举（H2）

安装脚本（spxctl install）在迁移完成后：检查 `iam_users` 为空 → 创建 `admin`（随机 24 位密码，Argon2id）→ **密码只打印一次**（首登强制修改）→ 创建默认组织 `default` 并将 admin 加入 → 跑"安装即验收"的登录冒烟（admin 能登录 = 验收通过才退出 0）。

## 12. 技术选型

选型原则：每个依赖必须有明确的"为什么不选 X"；优先维护活跃、类型标注完备、与 FastAPI/SQLAlchemy 2.0 生态同源的库；信创 arm64/amd64 双架构可构建（NFR7）。

| 领域 | 选择 | 理由 | 落选者及原因 |
|---|---|---|---|
| 密码哈希 | **argon2-cffi**（Argon2id） | OWASP 首选算法的一流绑定，PHC 串原生支持参数升级 | ❌ passlib：多年停止维护、抽象层反而碍事；❌ bcrypt：内存硬度弱于 Argon2id |
| 主键 UUIDv7 | **uuid-utils** | Rust 实现的时间有序 UUID（索引友好，架构总纲已定应用层生成）；PG16 无原生 v7 | ❌ uuid6 库：纯 Python 慢；❌ UUIDv4：索引随机写放大 |
| JWT | **joserfc** | authlib 同团队的新一代实现：完整类型标注、RFC 严格、持续维护 | ❌ PyJWT：维护节奏放缓、类型支持弱；❌ python-jose：有 CVE 历史、维护停滞 |
| OIDC 客户端 | **authlib** | OAuth2/OIDC 事实标准库，authorization code + PKCE 开箱即用 | ❌ 自写 httpx 流程：安全协议不该手写 |
| TOTP | **pyotp** | RFC 6238 标准实现，轻量无依赖 | — |
| 对称加密 | **cryptography**（AES-256-GCM + HKDF） | Python 密码学标准底座，用于 TOTP secret 与敏感配置 | ❌ pycryptodome：非 FFI 原生、更新慢 |
| ORM / 迁移 | **SQLAlchemy 2.0 / Alembic** | 架构总纲已定；2.0 风格 + async 支撑事件监听器（§8.2） | — |
| Redis 客户端 | **redis-py ≥ 5**（`redis.asyncio`） | 官方维护，asyncio 一等公民 | ❌ aioredis：已并入 redis-py（历史包袱勿引） |
| SAML（P1） | 选型待定（python3-saml 或 pysaml2） | SAML 生态整体偏老；P1 启动时按目标 IdP 实测再定，不提前锁死 | — |
| 集成测试 | **pytest + testcontainers-python** | RLS 是 PG 特性，**不能用 SQLite 替身**——testcontainers 拉真 PG 容器跑隔离测试 | ❌ SQLite 内存库：无 RLS，测了等于没测 |
| 静态检查 | **ruff + mypy** | 当代 Python 工程标配（lint + 格式化 + 类型三合一） | — |
| API 文档 | FastAPI 内建 OpenAPI | 契约即文档，前端与 CI 消费同一份 | — |

登录限速与账号锁定**自实现**（Redis INCR + TTL）：语义特殊（分级阈值、模糊错误、审计联动），通用限速库（slowapi/limits）的抽象反而碍事。

### 12.1 服务语言：为什么是 Python（而非 Go/Java/Rust）

全平台语言版图已定（架构总纲）：Python 承担全部业务角色、TS 前端、Rust 仅用于 agentgateway 数据面（inline 毫秒级）、Go 仅可选用于 UDP 收包。iam 保持 Python 不只是跟随，是三个设计事实的必然：

1. **性能画像不需要**：几十–几百用户、登录低频；每请求验签/权限判定已是本地缓存路径（§7.3，<5ms 不查库）；唯一 CPU 密集点 Argon2id 发生在 C FFI 层（argon2-cffi），语言不是变量。真正高并发的 inline 组件已由 Rust 网关承担——分工正确。
2. **三条总线是共生关系**：数据访问总线（SQLAlchemy 事件）、上下文总线（RequestContext/Depends）、`iam_readonly` 直读降级（NFR3）全部实现在 contracts 库、依赖同进程同运行时。iam 换语言 = 双技术栈 + 总线双份实现 + 单体红利放弃。
3. **协议生态无缺口**：OIDC/SAML/TOTP/加密在 Python 均有一流库（见上表）；Java 的 Spring Security/Keycloak 生态虽最全，代价是 JVM 资源（信创单机预算）与双运行时。

**重审触发条件**：iam 需要做公网海量用户认证中心（百万级）时——正确动作是启用方案 B（前置 Zitadel 承接认证，iam 退化为目录同步层），**升级靠组合不靠重写**（加法演进原则 P5）。

## 13. 安全威胁模型（STRIDE 简表 + 缓解）

| 威胁 | 场景 | 缓解 |
|---|---|---|
| 暴力破解/凭证填充 | 针对登录接口 | Argon2id 成本 + 账号锁定 + IP 限速（§5.4）+ 统一错误信息 |
| 会话劫持/固定 | cookie 被窃或复用 | HttpOnly/Secure/SameSite + 登录与切换组织重签 + TLS（部署层） |
| 令牌泄漏 | API Token / 刷新令牌外泄 | Token 哈希存储 + 即时吊销 + 刷新轮换重用检测（§5.6） |
| 越权（水平） | A 组织用户构造 B 组织 org_id | org 只来自服务端会话（§7.4）+ 自动过滤 + RLS 兜底（§8） |
| 越权（垂直） | analyst 调 admin API | 声明式权限点 + 中间件统一校验 + 启动时权限点注册校验（§6.1） |
| 权限提升链 | org_admin 给自己加平台权限 | 平台/组织权限严格分层（is_platform_admin 独立于角色体系，仅平台管理员可授予）+ 高危变更审计 |
| RLS 失效 | 应用忘设 GUC | 自动注入为主 + RLS 独立测试 + 应用角色 NOBYPASSRLS（§8） |
| 时序侧信道 | 登录接口用户存在性 | 恒定时间路径（不存在用户也跑一次哈希，§5.1） |
| iam 单点被攻破 | iam 数据库泄露 | 密码 Argon2id / TOTP 密钥 AES-GCM / Token 仅哈希——**数据库里没有任何可登录明文** |

## 14. 可维护性与扩展性（NFR4/NFR5 的兑现）

### 14.0 命名规范（全平台强制，CI 检查）

| 对象 | 规范 | 示例 |
|---|---|---|
| 数据库表 | `iam_` 前缀 + 复数名词 + snake_case | `iam_orgs`、`iam_api_tokens` |
| 列 | snake_case；布尔 `is_/has_/must_` 前缀；时间戳 `_at` 后缀；主键 `{实体}_id` | `is_platform_admin`、`revoked_at`、`org_id` |
| 枚举值 | 小写单词，值域在 CHECK 约束中显式声明 | `('active','disabled')` |
| Redis 键 | `iam:{域}:{标识}`，域用全词不用缩写 | `iam:session:{sid}`、`iam:permissions:{uid}:{oid}` |
| Python 类 | PascalCase；权限声明用 `PermissionSpec`（不缩写） | `RequestContext`、`OrgScoped` |
| JWT claims | 标准 claim 短名 + 自定义 `org_id`/`permissions` | `iss, sub, aud, jti, permissions` |
| 权限点 | `resource:action`，resource 复数、action 限动词集 | `cases:write`、`soar:approve` |
| 环境变量 | `SPX_` 前缀全大写 | `SPX_SECRET_KEY`、`SPX_ROLES` |
| API 路径 | 复数资源、连字符分隔、动作用子路径 | `/roles/matrix`、`/auth/switch-org` |


### 14.1 新增权限点 SOP（≤3 步）

1. 服务代码声明 `PermissionSpec(code="x:y", description="说明")` 并在 API 上使用；2. 启动（iam 注册校验自动通过，未声明会失败）；3. 管理界面矩阵中勾选授予角色（或 SQL 种子）。菜单属前端路由 meta，同权限点。

### 14.2 新增业务表自动隔离 SOP（≤4 步）

1. 表加 `org_id UUID NOT NULL`；2. 迁移里套用 RLS 模板（§8.1，模板函数一键生成）；3. ORM 实体打 `OrgScoped` 标记 mixin；4. 复制隔离测试模板改表名（§15）。零过滤代码。

### 14.3 权限模型演进（ABAC，N3）

`iam_role_permissions.conditions` 已预留 JSONB：v2 启用时表达式形如 `{"ip_cidr": [...], "time_window": {...]}`，由 authz 模块求值——**表结构零迁移**，PEP 不变。

### 14.4 认证外包升级路径（N6，方案 B）

内建与外部 IdP 是叠加关系：`settings/oidc` 配置 Zitadel/Keycloak 后，可关闭本地密码登录（`SPX_LOCAL_LOGIN=off`）——认证外包、目录与授权仍在我们手里。反向撤退（撤掉外部 IdP）同样只改配置。

## 15. 测试策略（隔离测试是最高优先级）

| 层 | 内容 |
|---|---|
| **隔离测试（每表必备模板）** | fixture：建 org A/B 各插一行 → 以 A 上下文查询：仅见 A 行；构造 B 的 org_id 参数：拒绝；**关闭应用层注入（只靠 RLS）重跑：仍通过**；RLS 关闭（只靠注入）重跑：仍通过——双保险各自独立验证 |
| 权限矩阵测试 | 参数化：四预置角色 × 全权限点 × 关键 API，断言 200/403 与 PRD §6.2 矩阵一致（矩阵变更测试同步变） |
| 认证流测试 | 锁定阈值/解锁、TOTP 绑定确认、改密全会话吊销、刷新重用检测全族吊销（验收场景 8） |
| 可用性测试 | 停 iam 进程 → 已登录用户全 API 正常（验收场景 5） |
| 注册校验测试 | 声明未注册权限点的服务启动失败（D1） |
| 审计测试 | 每个变更端点产生审计事件且入哈希链 |

CI：隔离与权限矩阵测试进 P0 流水线（任何 PR 必跑）。

## 16. 实施里程碑

| 里程碑 | 内容 | 出口条件 |
|---|---|---|
| M1 骨架 | 领域模型迁移 + RLS 模板 + 数据访问总线（§4/§8） | 验收场景 6 通过 |
| M2 认证 | 本地登录全链（密码/锁定/会话/切换组织/MFA）+ bootstrap | 验收场景 3/5 通过 |
| M3 授权 | 权限点注册制 + RBAC 判定 + /me/permissions + 中间件 | 验收场景 2 通过 |
| M4 管理 | 组织/用户/角色/Token/SSO 管理 API 与页面 + 审计 | PRD §10 全部 8 场景通过 |

## 17. 开放问题

| # | 问题 | 影响 |
|---|---|---|
| 1 | 权限矩阵基线（PRD 开放问题 1）定稿 | M3 前确定 |
| 2 | 服务间 JWT 的吊销名单（jti 黑名单）是否 v1 就要 | 当前 15min 短效可接受，P1 |
| 3 | Redis 持久化策略（AOF everysec）下的会话恢复语义 | 与平台级 Redis 决策合并处理 |

## 18. 变更记录

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-10-24 | 0.1 | 初稿：对齐 PRD 全部 P0 需求；领域模型/双保险隔离/三总线/威胁模型/SOP/测试策略 |
| 2026-10-24 | 0.4 | 语言决策回归：**Python 全栈**（曾改 Go 后否决——LiteLLM/NeMo 内嵌、contracts 单包单源等连锁简化大于多语言收益）；guardrail 检测两层（规则同步/模型异步调 agent-runtime）随平台架构同步 |
| 2026-10-24 | 0.2 | 交叉审计修订 16 项：新增 §5.9 SCIM 概要、§12 技术选型（含落选理由）、§14.0 命名规范；MFA 改为角色属性可配置；会话去 roles；补缓存 miss 直读路径（iam_readonly）；归档/停用级联/Token 范围校验/`/me/orgs`/矩阵读权限修正；DDL 增 mfa_required/last_login_at/settings 列；Redis 键与类名规范化 |
