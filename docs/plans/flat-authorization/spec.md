# 扁平授权：权限只由用户与角色表达

## 背景

IAM 之前把授权挂在分层范围上：关系挂在范围节点上、角色归属于范围、权限按 `scope` 分成
platform / 范围两级，范围之间通过祖先链继承权限。这套模型带来的是：

- 每个受保护请求都要解析范围链并合并祖先授权（一个递归 CTE）；
- 同一用户在多范围时，列表、切换器、关系、角色继承四套语义叠加；
- 范围标识（slug）、范围树、根范围不变量、范围管理员等一批只服务于该模型的字段与页面；
- 平台超管与范围权限两套判定分支，任何一处漏判都会越权。

产品实际只需要「一台设备、一个安全运营团队、一套权限」：没有外部租户，没有多客户隔离
（README「不是 SaaS 多客户」）。这一层因此只增加了复杂度，没有任何业务收益。

## 目标

1. 删除范围相关内容：数据库表与列、Go 模型/服务/仓储/路由、Web 页面与导航、Console 的范围上下文、
   配置项、文档。代码与文档里不再出现范围概念：表名、列名、路由段、权限键一律扁平。
2. 授权只由「用户 → 角色 → 权限」表达：用户直接持有角色，角色全局存在，权限目录扁平。
3. 内置数据全部来自数据库：权限目录、超级管理员角色及其全部权限、配置指定的超级管理员用户，
   在启动时按配置幂等写入数据库；前端只读接口，不在前端写死任何内置行。
4. 内置数据不可删除、不可修改（可查看）：超级管理员角色不可改名称/说明/权限、不可删除；
   配置指定的管理员用户不可删除、不可停用、不可在页面改密码。

## 数据模型（最终态）

```
iam.users(id, username, display_name, email, status, timezone, auth_epoch, configured, created_at, updated_at)
iam.password_credentials(user_id, password_hash, changed_at, failed_attempts, locked_until, updated_at)
iam.user_sessions(id, user_id, token_hash, auth_epoch, state, authenticated_at, ...)
iam.roles(id, name, description, builtin, created_by, created_at, updated_at)  -- 全局，无范围列
iam.permissions(permission_key, description, risk_level, delegable, requires_recent_auth, status, ...)              -- 不再有 scope
iam.role_permissions(role_id, permission_key)
iam.user_roles(user_id, role_id, created_by, created_at)                               -- 取代旧范围的角色绑定表
iam.audit_events(..., target_type, target_id, metadata)                                -- 无范围列
```

- `roles.builtin` 标记内置角色（超级管理员），删除/改名/改权限均被服务层拒绝。
- `users.configured` 标记配置指定的管理员，删除/停用/改密/摘除内置角色被拒绝。
- 角色名全局唯一（`UNIQUE(lower(name))`）；列表里内置角色固定排在最前。
- 权限键是唯一稳定标识，固定为 `module.resource.action`：这三段不单独存列，由键派生，
  从结构上消除「键与元数据漂移」这一类缺陷。目录里已删除的键在启动同步时标记 `retired`，
  历史授权保留但立即失效。
- 不再有 `users.is_super_admin`：超级管理员就是「持有内置超级管理员角色」这一条数据。
- 历史归属用 `ON DELETE SET NULL` 表达：`roles.created_by`、`user_roles.created_by` 可空，
  删账号只置空创建人，不阻塞删除、也不丢角色绑定；`user_sessions`、`password_credentials`、
  `user_roles`、`role_permissions` 随主体级联删除。

## 权限目录（扁平，7 项）

所有键都可委派；标记「近期认证」的键要求操作者先在 30 分钟内确认过密码。

| key | 说明 | 风险 | 近期认证 |
| --- | --- | --- | --- |
| iam.users.read | 查看用户 | normal | 否 |
| iam.users.manage | 创建、修改、删除用户 | high | 是 |
| iam.users.reset | 重置用户密码 | high | 是 |
| iam.users.roles | 分配用户角色 | high | 是 |
| iam.roles.read | 查看角色与权限目录 | normal | 否 |
| iam.roles.manage | 创建、修改、删除角色与角色权限 | high | 是 |
| iam.audit.read | 查看审计 | normal | 否 |

删除的键：分级权限键全部删除（`iam.platform.*` 与范围前缀键）、`iam.roles.assign`（并入 `iam.users.roles`）。

## HTTP 契约（全部扁平，无范围段）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/v1/iam/auth/context | 会话上下文（CSRF） |
| POST | /api/v1/iam/auth/login · logout · reauthenticate | 登录与会话 |
| GET | /api/v1/iam/me | 账号、角色、有效权限与时区偏好 |
| PATCH | /api/v1/iam/me | 更新时区偏好 |
| POST | /api/v1/iam/me/password | 修改自己的密码 |
| GET | /api/v1/iam/users | 用户列表（含角色） |
| POST | /api/v1/iam/users | 创建用户 |
| GET/PATCH/DELETE | /api/v1/iam/users/:id | 用户详情/修改/删除 |
| PUT | /api/v1/iam/users/:id/roles | 设置用户角色 `{role_ids}` |
| POST | /api/v1/iam/users/:id/reset-password | 重置密码 |
| GET | /api/v1/iam/roles | 角色列表（含内置行、用户数） |
| POST | /api/v1/iam/roles | 创建角色 |
| GET/PATCH/DELETE | /api/v1/iam/roles/:id | 角色详情（含权限）/修改/删除 |
| PUT | /api/v1/iam/roles/:id/permissions | 全量替换角色权限 |
| GET | /api/v1/iam/permissions | 权限目录（含 can_delegate） |
| GET | /api/v1/iam/audit-events | 审计 |
| GET | /api/v1/console/context | Console 账号上下文 |
| GET | /api/v1/console/events | Console 事件流 |

`/api/v1/iam/me` 新形状：

```json
{
  "principal": { "user_id": "...", "session_id": "...", "username": "admin", "display_name": "管理员",
                 "configured": true, "auth_epoch": "1", "authenticated_at": "..." },
  "roles": [{ "id": "...", "name": "超级管理员" }],
  "permissions": ["iam.users.read", "..."],
  "timezone": "Asia/Shanghai"
}
```

只保留前端实际消费的端点：角色详情已含权限，因此没有单独的 `GET /roles/:id/permissions`；
持有某角色的用户通过用户列表的 `roles` 字段体现，因此没有 `GET /roles/:id/users`；
授权影响预览没有任何调用方，已随本轮一并删除。

`/api/v1/iam/authorize`（服务间）只接收 `{action}`，返回 `{allowed, decision_id, principal}`：扁平授权没有范围字段。

## 启动时写入的内置数据

1. 权限目录：以代码中的 `permissions.json` 为准，启动时补齐缺失键、更新描述/风险/可委派/近期认证等元数据。
2. 超级管理员角色：`name='超级管理员', builtin=true`，权限 = 权限目录全部有效键；只补不删。
3. 配置管理员：`bootstrap.username/password` 写入 `users.configured=true`，并授予超级管理员角色；
   改配置 → 下次启动更新用户名/密码并撤销旧会话。

## 不变量（服务层强制，均有集成测试）

| 权限确认失败不累计登录失败 | 密码确认（sudo 模式） | 400 `iam.invalid_current_password`，不锁定账号 |
| 审计只追加 | 任何 UPDATE/DELETE | 数据库拒绝；保留期清理通过事务变量 `iam.audit_purge` 开闸 |
| 不变量 | 触发点 | 结果码 |
| --- | --- | --- |
| 至少一名启用中的账号持有内置角色 | 删除用户、停用用户、摘除某人的内置角色 | `iam.last_super_admin` |
| 配置管理员账号不可被页面改变 | 删除、停用、重置密码、摘除内置角色 | `iam.configured_user_immutable` |
| 内置角色只读 | 改名、改说明、改权限、删除 | `iam.builtin_role_immutable` |
| 不能操作当前登录账号 | 删除自己、停用自己 | `iam.self_target_forbidden` |
| 仍有用户持有的角色不可删除 | 删除角色 | `iam.role_in_use` |
| 角色名唯一 | 创建、改名 | `iam.role_name_taken` |

审计只记录差异：角色权限变更写 `permissions_added` / `permissions_removed`，
用户角色变更写 `roles_added` / `roles_removed`，避免整份集合撑破 8192 字节的 metadata 上限。

## 删除面

- 数据库：旧范围表（范围、范围用户、范围角色绑定、范围树守卫）、
  单根索引、slug、authz_epoch、所有范围列。
- 后端：范围模型/服务/处理器/仓储、范围树、关系、范围权限快照、范围上下文、
  范围相关权限键、`users.is_super_admin`。
- 前端：范围管理页、用户页的范围树与关系抽屉、范围切换器、范围路由与
  范围相关 API/schema/i18n。
- Console：带范围段的 `context|events` 路由改为扁平的 `/api/v1/console/context|events`。
- 配置：`service_clients.console.actions` 改为扁平权限键。

## 会话与审计的职责

- `user_sessions` 只存认证状态：令牌摘要、账号版本、两次到期时间、最近活动时间、撤销原因。
  不存 IP 与 User-Agent——没有设备管理界面时这些列没有消费者；登录来源写进登录审计的 metadata。
- `audit_events` 与业务变更同事务（写失败即回滚），追加写由触发器保证：UPDATE 一律拒绝，
  DELETE 只在保留期清理事务里通过 `iam.audit_purge` 放行。
- 保留期默认 365 天，覆盖《网络安全法》要求的六个月下限。

## 登录防护

- 账号维度：连续 `login_max_failures` 次失败后锁定 `login_lock_duration`（默认 5 次 / 15 分钟），
  到期自动解锁；密码确认（sudo 模式）的失败**不计入**，避免一次输错把操作者锁在门外。
- 来源维度：每 IP 每分钟 `login_ip_rate` 次；账号维度另有每 15 分钟 `login_account_rate` 次。
  两个阈值都在 `config.yaml`，不同暴露面的部署可以自行收紧。

## 迁移

项目未发版，按「清空重建」处理：迁移文件合并为单一基线 `00001_accounts.sql`，
部署侧删除 `iam` schema 与 goose 版本表后重新执行迁移。不保留历史迁移、不做数据回填。

## 非目标

- 多租户 / 外部客户隔离（产品明确不是 SaaS 多客户）。
- 按范围区分的 SSO、SCIM、登录域（如需，未来引入独立不可变键，而不是复用名称）。
- 角色继承、角色组、审批流。
