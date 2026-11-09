# 实施计划：审计与角色管理简化、设置区合并

## 组 1 — 数据库与角色模型（移除 code / kind）

1. 新迁移 `00013_roles_drop_code_kind.sql`
   - 删除 CHECK：`roles_system_code_check`（00011 重建的那个）
   - 删除 `UNIQUE(org_id,code)`
   - 删除列 `code`、`kind`
   - Down：无法恢复原数据（已丢弃的 code），明确拒绝回滚并说明
2. `repository/store.go`
   - 删除 `CreateOrganizationRoles` 里铸内置角色的逻辑；组织创建不再写入任何角色
   - 删除 `SyncAdministrator` 中按 `kind='system' AND code='org_admin'` 授权同步
   - 删除 `OrganizationsWithoutAdministrator` 调用与启动告警
3. `repository/roles.go`
   - `Role` 结构去掉 `Code`/`Kind`
   - `CreateRole` 签名去掉 `code`
   - 所有 `r.kind='system' AND r.code='org_admin'` 条件删除
   - 删除 `ProtectOrganizationAdministrators`、`ProtectAdministeredOrganizations`
   - `ListRoles` SQL 去掉 code/kind
4. `service/roles.go`
   - `CreateRole` 去掉 code 校验与 metadata["code"]
   - `UpdateMemberRoles` 去掉 org_admin 特判
   - 删除 `ProtectOrganizationAdministrators` 调用
5. `service/preview.go` 去掉 org_admin 特判
6. `service/members.go` 删除 `ProtectOrganizationAdministrators` 调用
7. `repository/members.go` `LockMembershipAdministrator` 改为仅锁定 membership 并返回 user_id（不再判断 admin）

## 组 2 — 角色 API 契约

1. `model/http.go`
   - `APIRoleCreate`：`name`、`description`、`permissions`（新增）
   - `APIRole`：去掉 `code`、`kind`
   - 新增创建时的权限字段校验（复用 `iam_permissions`）
2. `handler/management/roles.go`：创建时在同一事务写入权限
3. `service/roles.go`：`CreateRole` 返回 id 并落权限
4. 删除角色前置：`member_count > 0` → 冲突（已有，确认错误码与文案）

## 组 3 — 审计单一端点

1. 删除 `GET /orgs/:org_id/audit-events` 路由与 `ListOrganizationAuditEvents` handler
2. `ListAuditEvents` 增加 `actor`、`from`、`to`、`outcome` 过滤参数
3. `repository.ListAuditEvents` 改为接受过滤结构；可见范围由 service 授权决定：
   - 平台审计权限 → 不加 org 过滤
   - 仅组织审计权限 → `org_id = ANY(可见组织)`
4. `model.ListQuery` → 新增 `AuditQuery`

## 组 4 — 前端路由与设置区合并

1. 路由收敛：
   - `/settings` → 单一设置页（个人信息 + 账号安全两段）
   - 删除 `settings/profile`、`settings/account`
   - 管理区从 `/management/**` 迁到 `/settings/**`
2. `iam/navigation.ts`：`path` 前缀改 `/settings/...`；分组收敛
3. `shell.tsx`：头像菜单只留 `Settings` 一项；左侧菜单设置项提到首位
4. 更新所有指向 `/management` 的跳转与 E2E

## 组 5 — 前端角色页重写

1. 删除顶部角色标签
2. 搜索行：名称输入（左）+ 刷新/创建角色（右）
3. 列表列：名称、说明、成员数、操作（编辑/删除）
4. 创建：`Sheet` 抽屉，名称 + 说明 + 权限勾选
5. 编辑：`Sheet` 抽屉三段式（信息/权限/成员只读）
6. 删除：有成员时禁用并说明

## 组 6 — 前端审计页

1. 删除标签，改筛选行：操作者、时间范围、结果
2. 单表渲染

## 组 7 — i18n 与文档

1. `en` / `zh-CN` 键对齐，删除孤立键，新增筛选与抽屉文案
2. `iam/README.md` 路由表、角色模型说明
3. `docs/plans/README.md` 登记
4. 模块 README 与架构文档中涉及 `/management`、角色 code/kind 的描述

## 验证

- gofmt / go vet / go build / go test
- 集成测试（含角色与审计路径改写）
- tsc / vitest / next build
- 浏览器实测：设置页、角色抽屉、审计筛选
- 残留引用全面搜索：`code`、`kind`、`org_admin`、`/management`、`profile`、`account`

## 风险

- **删除组织管理员不变量**是唯一削弱既有保证的改动，已在 spec 明示并获用户确认。
- 迁移不可回滚（列已删除），Down 明确报错。
- 路由大迁移容易漏改，需全量搜索验证。
