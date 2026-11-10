# 实施计划：审计与角色管理简化、设置区合并

## 组 1 — 角色模型（无 code / kind）

1. 基线迁移 `00001_accounts.sql`：`iam.roles` 只有 `id, name, description, builtin, version, created_by, created_at, updated_at`，不存在 `code`/`kind` 列及其约束。
2. `repository/roles.go`：`Role` 结构只保留上述字段；`CreateRole` 签名不含 code；`ListRoles` 不再查询 code/kind。
3. `service/roles.go`：`CreateRole` 不校验 code、不写 metadata code；内置角色（`builtin=true`）的改名、改说明、改权限、删除由服务层拒绝。
4. 删除按角色 code/kind 识别内置身份的分支：内置身份只由 `roles.builtin` 表达，超级管理员角色与权限在启动时幂等写入。

## 组 2 — 角色 API 契约

1. `model/http.go`
   - `APIRoleCreate`：`name`、`description`、`permissions`
   - `APIRole`：`id`、`name`、`description`、`builtin`、`user_count`、`version`、`permissions`；无 `code`、`kind`
   - 创建时的权限字段校验复用 `iam_permissions` 校验器
2. `handler/management/roles.go`：创建时在同一事务写入权限。
3. `service/roles.go`：`CreateRole` 返回 id 并落权限。
4. 删除角色前置：`user_count > 0` 或 `builtin` → 冲突（错误码与文案）。

## 组 3 — 审计单一端点

1. 只注册 `GET /api/v1/iam/audit-events`，不存在带范围段的路由或 handler。
2. `ListAuditEvents` 支持 `actor`、`from`、`to`、`outcome` 过滤参数。
3. `repository.ListAuditEvents` 接受过滤结构；可见范围只由 `iam.audit.read` 决定，不追加范围条件。
4. `model.AuditQuery` 承载分页与筛选条件。

## 组 4 — 前端路由与设置区合并

1. 路由收敛：
   - `/settings` → 单一设置页（个人信息 + 账号安全两段）
   - `/settings/roles`、`/settings/users`、`/settings/audit`
   - 删除 `settings/profile`、`settings/account` 与 `/management/**`
2. `iam/navigation.ts`：`settingsPages` 使用 `/settings/...` 前缀；分组收敛。
3. `shell.tsx`：头像菜单只留 `Settings` 一项；左侧菜单设置项提到首位。
4. 更新所有指向 `/management` 的跳转与 E2E。

## 组 5 — 前端角色页重写

1. 删除顶部角色标签
2. 搜索行：名称输入（左）+ 刷新/创建角色（右）
3. 列表列：名称、说明、用户数、操作（编辑/删除）；内置行带「内置」徽标且无编辑/删除
4. 创建：`SheetPanel` 抽屉，名称 + 说明 + 权限勾选
5. 编辑：`SheetPanel` 抽屉两段式（信息/权限）
6. 删除：有用户持有时禁用并说明；内置角色不可删除

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
- 残留引用全面搜索：`code`、`kind`、`/management`、`profile`、`account`

## 风险

- 迁移不可回滚：基线重建意味着整库重建，回退从备份恢复。
- 路由大迁移容易漏改，需全量搜索验证。
