# 实施与验收

依据 [spec.md](spec.md)。按测试先行推进：每项先写失败测试，再实现。测试层按 [docs/development/testing.md](../../development/testing.md#改动类型与必需测试层) 判定表执行。

项目未发版，**所有迁移按清空重建处理**：不做数据回填、不做兼容层、不保留旧 id。迁移只负责把 schema 建对。

## 依赖关系

```
第 1 组（校验与约束单一来源）
      │
      ├──────────────┬──────────────┐
      ▼              ▼              ▼
第 2 组（组织树）  第 3 组（去 status）  第 4 组（权限目录扩展）
      │              │              │
      └──────┬───────┴──────┬───────┘
             ▼              ▼
     第 5 组（信息架构重划）
             │
             ▼
     第 6 组（成员创建与二次认证修正）
             │
             ▼
     第 7 组（树形展示）
```

第 1 组必须最先：组织、成员、角色三处都依赖同一套校验与唯一键。第 2、3、4 组互不依赖，可并行。第 5 组依赖前三组的数据形状。第 6 组依赖第 1 组校验与第 4 组角色契约。第 7 组纯展示，最后做。

## 第 1 组：字段校验与约束单一来源

### 1.1 先写失败测试

- `iam/internal/identity/username_test.go`：表驱动覆盖 2/3/39/40 长度边界、首字符非字母、连续 `[._-]{2,}`、保留名单、大小写归一化。
- `iam/internal/identity/email_test.go`：空值可选、HTML5 合法样例、本地部 65 拒绝、总长 255 拒绝、无 `@`、连续点。
- `iam/internal/identity/display_test.go`：trim、1/100/101 码点边界、纯空白拒绝。
- `iam/internal/identity/password_test.go`：扩展到 11/12/128/129 边界、NUL 拒绝。
- `tests/unit/validation-parity.test.ts`：前端 Zod 与 Go 规则在共享样例集上判定一致（样例集来自一处 JSON）。

### 1.2 实现

- `iam/internal/identity/username.go`：`NormalizeUsername` 改为 `^[A-Za-z][A-Za-z0-9_.-]{2,38}$` + 拒绝连续 `[._-]{2,}` + 保留名单。
- `iam/internal/identity/email.go`：新增 `ValidateEmail`（HTML5 正则 + 本地部 ≤64 + 总长 ≤254 + 小写归一化键）。
- `iam/internal/identity/display.go`：新增 `ValidateDisplayName`（trim + 码点 1–100）。
- `iam/internal/identity/password.go`：长度改 12–128，保留 NUL/UTF-8 校验。
- 迁移 `00009_identity_constraints.sql`（清空重建）：
  - `iam.users.username` CHECK 收紧为 `^[A-Za-z][A-Za-z0-9_.-]{2,38}$`。
  - `iam.users.email_normalized` 生成列（`lower(email)`）+ 部分唯一索引（`WHERE email IS NOT NULL`）。
  - ~~`iam.organizations.slug` CHECK 统一~~：已由 `00004` 完成，无需改动（见 P1-5 撤回）。
- `src/apps/web/src/spx/api/iam/schemas.ts`：Zod 规则改为与 Go 一致，删除手写正则重复。

### 1.3 判据

移除任一校验实现后对应单元测试必须失败；parity 测试在任一端偏离时失败。

## 第 2 组：组织树

### 2.1 先写失败测试

- `repository/organization_tree_integration_test.go`：根组织 `slug=master` 存在；`parent_id` 外键生效；深度 4 插入被拒；自引用与环被拒；`parent_id IS NULL` 唯一。
- `service/organization_tree_test.go`：创建子组织默认挂 `master`；父组织不存在返回 `ErrInvalid`；改 slug 返回 `ErrInvalid`；删除有子组织返回冲突。
- `handler` 集成测试：`GET /organizations/tree` 可见性（超管见全树、普通成员只见所属分支）。

### 2.2 实现

- 迁移 `00010_organization_tree.sql`（清空重建）：
  - `iam.organizations` 增加 `parent_id uuid REFERENCES iam.organizations(id)`（可空，仅根为空）。
  - 增加 `depth smallint NOT NULL CHECK(depth BETWEEN 0 AND 2)`（**无默认值**，由服务层按 `parent.depth + 1` 计算，见 P0-2）。
  - 唯一部分索引：`CREATE UNIQUE INDEX ON iam.organizations((parent_id IS NULL)) WHERE parent_id IS NULL`（保证最多一个根）。
  - 触发器或 CHECK 阻止自引用；环检测在服务层事务内做（需要读祖先链）。
  - 插入根组织 `slug=master, name=AISOC, parent_id=NULL, depth=0`。
- `repository/organizations.go`：`CreateOrganization` 接受 `parentID`；新增 `OrganizationTree`（`WITH RECURSIVE`）；新增 `OrganizationAncestors`。
- `repository/store.go:169`：`Seed` 中 `CreateOrganization(..., "default", "默认组织", ...)` 改为创建/复用 `slug=master, name=AISOC, parent_id=NULL, depth=0`（见 P0-1）。
- `repository/read_queries.go`：`VisibleOrganizations` 改为返回「所属分支」（祖先链 + 自己有成员关系的节点）。
- `service/organizations.go`：创建/更新/删除的组织树校验与删除前置检查——删除时在事务内**先**查子组织数与成员数，分别返回 `iam.organization_has_children` / `iam.organization_has_members`，外键错误仅作兜底（见 P0-5）。当前无 `DeleteOrganization`，需新建服务方法与 `DELETE /api/v1/iam/organizations/:id` 路由。
- `model/http.go`：`APIOrganization` 增加 `parent_id`、`depth`、`path`；新增 `APIOrganizationTree`。
- `router/routes.go`：新增 `GET /api/v1/iam/organizations/tree`。
- 权限：沿用 `iam.platform.organizations.*`，不新增权限点。

### 2.3 判据

迁移可重复执行；深度与环校验双向验证（允许通过、越界拒绝）。

## 第 3 组：移除角色与组织的启用/停用状态

这是本计划影响面最广的一项，必须完整覆盖以下所有引用点。

### 3.1 先写失败测试

- `service/roles_test.go`：创建角色请求体含 `status` 时被拒绝（未知字段）。
- `handler` 集成测试：角色列表响应不含 `status` 字段；组织列表同理。
- E2E：角色页与组织页无 status 列、无 status 控件。

### 3.2 实现清单（逐项核对，不得遗漏）

**数据库**
- 迁移 `00011_drop_status.sql`（清空重建）：
  - `iam.roles` 删除 `status` 列，删除 `CHECK(kind/status)` 中的 status 部分。
  - `iam.organizations` 删除 `status` 列。
  - 更新依赖 status 的约束与索引。

**Go 仓储层（SQL 必须同步改，已逐行核对）**
- `repository/roles.go:28` `CreateRole` 签名去掉 `status` 参数；`:36` 插入语句去掉该列。
- `repository/roles.go:45` 与 `:185` 列表投影去掉 `r.status`。
- `repository/roles.go:56` `UpdateRole` 去掉 `status`；`:61` 更新语句去掉该列。
- `repository/roles.go:155-160` `ProtectLastAdministrators` 去掉 `o.status='active'` 与 `r.status='active'`，**但保留** `u.status='active'` 与 `m.status='active'`（成员仍有状态）。须在该函数补注释说明「组织与角色无停用态，故不加状态条件」（见 P1-4）。
- `repository/queries.go:124` 的 `JOIN iam.roles r ... AND r.status='active'` **必须去掉**（否则删列后 SQL 报错）。
- `repository/authorization_queries.go:31` 去掉 `o.status='active'`。
- `repository/read_queries.go:40` `VisibleOrganizations` 去掉 `o.status='active'`。
- `repository/store.go:186` `CreateOrganization` 的 `INSERT INTO iam.organizations(...,status) VALUES(...,'active')` **去掉 status 列与值**。
- `repository/store.go:191` 内置角色插入的 `INSERT INTO iam.roles(...,status,...) VALUES(...,'active',...)` **去掉 status 列与两个 'active' 值**。
- `repository/store.go:177` 与 `:195` 的 `p.status='active'` 过滤的是 **`iam.permissions.status`（权限目录）**，不是角色/组织状态，**必须保留**。这是本组唯一易误删的点。
- `repository/organizations.go:15` `UpdateOrganization` 去掉 `status` 参数。

**Go 服务层**
- `service/roles.go:40-42` 删除 `if input.Status == ""` 默认值逻辑。
- `service/roles.go` `CreateRole`/`UpdateRole` 调用处去掉 `status` 实参。
- `service/organizations.go:52` `UpdateOrganization` 去掉 `status` 处理。
- `service/lists.go` 的列表查询不受影响（透传）。

**Go DTO**
- `model/http.go` `APIRoleCreate`、`APIRoleInput`、`APIOrganizationUpdate` 删除 `Status` 字段。
- `model/http.go` `APIOrganization` 删除 `Status`。

**前端 API 模型**
- `schemas.ts:42` `zApiOrganization` 删除 `status`。
- `schemas.ts:112` `zApiRole` 删除 `status` 枚举。
- `schemas.ts` 的 `zCreateRoleBody`/`zUpdateRoleBody`/`zUpdateOrganizationBody` 删除 `status`。

**前端页面**
- `roles.tsx:98`、`:112` 的 `roleForm.reset` 去掉 `status`。
- `roles.tsx:140-142` 删除表格 status 列。
- `roles.tsx:274-281` 删除表单 status 控件。
- `organizations.tsx:53-55` 删除表格 status 列。
- `organizations.tsx:125`、`:140-148` 删除表单 status 字段与控件。
- `accounts.tsx`、`members.tsx` 保留成员 status（**不动**）。

**国际化**
- `messages/{en,zh-CN}.json`：保留 `Management.status/active/disabled`（成员仍在用）；新增文案仅在有新的语义时添加。

**后端测试（删字段后原请求体会因未知字段被拒，必须同步改）**
- `handler/server_integration_test.go:342`、`:472`、`:476`、`:480`、`:506`、`:633` 六处 `PATCH .../roles/...` 请求体删除 `"status": "active"`。
- `repository/migrations_integration_test.go:245`、`:261`、`:267` 三处 `INSERT INTO iam.organizations(...,status)` 删除 status 列与值（这些用例若与新约束冲突，按新 schema 重写）。

**文档**
- `docs/development/role-management.md`：更新「系统角色保护」段落，删除 status 相关描述。

### 3.3 判据

`grep -rn "role.status\|org.status\|r.status\|o.status" src/` 结果为空（除成员与权限目录的 status）；E2E 断言页面无 status 控件。

## 第 4 组：权限目录扩展

### 4.1 先写失败测试

- `scripts/generate-catalogs.test.mjs`：条目缺少 `group` 时生成失败；生成物含分组顺序。
- `tests/unit/permissions-catalog.test.ts`：角色编辑器分组来源是生成物而非硬编码。

### 4.2 实现

- `permissions.json` 为每条增加 `group`（如 `"group": "userManagement"`），不改既有字段语义。
- `scripts/generate-catalogs.mjs`：生成 `src/apps/web/src/auth/permission-groups.gen.ts`，断言 `group` 必填、分组顺序稳定。
- `roles.tsx:312`：分组与顺序改从生成物读取。
- `AGENTS.md` 与 `docs/development/role-management.md`：写入「新增模块声明权限的唯一入口是 `permissions.json`，禁止在业务代码硬编码权限字符串」。

### 4.3 判据

只改 `permissions.json` 新增一个模块条目并重新生成，角色编辑器即出现新分组（单元测试锁定数据源）。

## 第 5 组：信息架构重划

### 5.1 先写失败测试

- E2E：新导航结构（「角色与权限」「组织管理」「审计日志」）与各分组页面集合。
- E2E：旧「平台成员」导航路径不再出现。

### 5.2 实现

- `spx/iam/navigation.ts`：
  - 新增 `nav?: boolean` 字段（默认 true）。`visibleManagementPages` 返回时过滤 `nav === false`，但 `ManagementPage` 的路由匹配改用**未过滤**的完整 `managementPages`，否则 `/management/platform/users` 会命中 `!page` 返回 `AccessDenied`（见 P0-3）。
  - `users` 标记 `nav: false`，仍可从「角色与权限」内部进入。
  - `members` 的处理见 P1-1 选定方案。
- `messages/{en,zh-CN}.json`：`Navigation.people` 改为「角色与权限」/「Roles & permissions」；其余跟随。
- `spx/components/shell.tsx`：侧栏分组渲染跟随 `navigation.ts`（已数据驱动，预计无需改逻辑）。
- `spx/pages/management.tsx`：路由分支同步。**「组织管理」采用两个同级标签页（见第二轮复审 P1-1，B' 方案）**：`当前组织`（基本信息 + 改名）与 `组织成员`（列表 + 分配角色 + 移除）。`members` 保留为独立 `managementPages` 条目，仅把 `group` 从 `people` 改为 `organization`，不嵌入 `OverviewPage`。依据：GitHub/GitLab/Atlassian 均把成员管理与配置分置，无一采用单页堆叠（详见文末第二轮复审）。
- `tests/e2e/identity.spec.ts`：同步导航断言。

### 5.3 判据

E2E 锁定新结构；旧路径不留孤儿链接。

## 第 6 组：成员创建与二次认证修正

### 6.1 先写失败测试

- `service/accounts_integration_test.go`：带角色创建的事务性（账号/凭据/成员/角色全成或全滚）；角色不属于当前组织返回 `ErrInvalid`；授予自身没有的权限返回 `ErrDenied`。
- `handler` 集成测试：重复用户名/邮箱返回可区分结果码；非法字段返回 `invalid_request` 且不落库。
- `tests/unit` 表单测试：六字段表单的 Zod 校验，两次密码不一致时报错。
- E2E：创建成员全程不出现「当前密码」页。

### 6.2 实现

- `model/http.go`：`APIUserInput` 增加 `org_id`（必填）与 `role_ids`（必填，至少一个），见 P0-12。
- `service/accounts.go`：`CreateUser` 增加 `orgID` 参数并在同一事务内建成员关系与角色绑定；`orgID` 为空时返回 `ErrInvalid`，不允许创建无组织成员（见 P0-12）。复用 `Manage` 与 `CheckDelegation`。
- 结果码（见 P0-4）：
  - 在 `src/packages/go/result/error-codes.json` 增加 `iam.username_taken`、`iam.email_taken`（均 409），执行 `pnpm catalogs:generate` 重新生成 `codes.gen.go` 与 `src/apps/web/src/auth/result.gen.ts`。
  - 仓储层捕获 `pgconn.PgError`，按 `ConstraintName` 区分 `users_username_normalized_key` / `users_email_normalized_key`，返回上述专用码。**不得依赖 `server.go:99` 的全局 `23505 → conflict` 映射**，否则用户只看到「数据已变化」。
  - 前端 `errorMessageKey` 增加对应映射，`Errors` 命名空间补两语言文案（见 P1-2）。
- `permissions.json`：`iam.platform.users.manage` 的 `requires_recent_auth` 改为 `false`。
- `spx/components/forms.tsx`：`FormDialog` 的二次认证流程**保留**（重置密码等仍需要），但创建成员路径不再触发（因为权限项已改 false）。
- `spx/pages/accounts.tsx`：创建表单增加「确认密码」（仅前端 Zod `refine`，不发送）+ 角色多选。
- `spx/pages/members.tsx`：添加入口改为复用同一创建表单或链接到账号页。

### 6.3 判据

并发创建同名用户时恰好一个成功，另一个返回 `username_taken`；创建成员过程零次「当前密码」提示。

## 第 7 组：树形展示

### 7.1 先写失败测试

- `tests/unit/organization-tree.test.ts`：给定当前 org 返回应展开的祖先链；路径拼接。
- E2E：切换器展开后当前组织高亮且祖先已展开；组织详情展示分支路径；窄屏不横向溢出。

### 7.2 实现

- `spx/components/organization-tree.tsx`：基于现有 `Collapsible` + `ScrollArea` 的轻量树（不新增依赖）。
- `spx/components/shell.tsx:170`：组织切换 `Select` 换成树形 `Popover` + 树组件，打开时展开到当前节点。
- `spx/components/organization-path.tsx`：分支面包屑（复用 `Breadcrumb`）。
- 组织详情、成员详情接入路径展示；表格单元格保留短名 + `title` 完整路径。

### 7.3 判据

深度 3 的树在 `375px` 下不横向溢出；键盘可遍历（上下移动、左右展开折叠、Enter 选中）。

## 受影响的配置与文档清单

- `AGENTS.md`：新增「权限声明唯一入口」规则。
- `docs/development/role-management.md`：更新 status 段落、权限声明入口。
- `docs/plans/README.md`：本计划索引（已登记）。
- `src/packages/go/result/error-codes.json`：新增 4 个 iam 结果码（见 P0-4、P0-5），随后 `pnpm catalogs:generate`。
- `src/apps/iam/README.md`：更新组织段落为树形模型（根 `master`、深度上限 3），见 P1-6。
- `src/apps/iam/internal/model/organization.go`：确认 slug 注释与 CHECK 完全一致（3–30 位），见 P1-5。
- `messages/en.json`、`messages/zh-CN.json`：导航文案、新增结果码文案、确认密码标签。
- `src/apps/web/tests/e2e/identity.spec.ts`：导航结构、status 移除、创建成员流程断言。
- 无需改动：`config.yaml`（无角色/组织 status 配置）、`DESIGN.md`（无新视觉令牌）。

## 验证入口

```sh
go test ./src/apps/iam/...
go test -tags=integration ./src/apps/iam/...      # 需 SPX_TEST_POSTGRES_URL / SPX_TEST_REDIS_URL
pnpm --dir src/apps/web typecheck
pnpm --dir src/apps/web test:unit
pnpm --dir src/apps/web test:e2e
pnpm lint
node scripts/build.mjs
pnpm catalogs:check
```

## 验收证据

已实施完毕（未提交代码）。逐项实测结果：

| 检查 | 命令 | 结果 |
|---|---|---|
| Go 构建 | `go build ./...` | 0 |
| Go 静态检查 | `go vet ./src/...` | 0 |
| Go 单测 | `go test ./src/...` | 0，22 个包 |
| Go 格式 | `gofmt -l src` | 无输出 |
| 类型检查 | `pnpm typecheck` / `tsc --noEmit` | 0 |
| 前端单测 | `pnpm test:unit` | 18 文件 / 56 测试通过 |
| Lint | `pnpm lint` | 0（6 个提示全在未触碰的既有文件） |
| 根构建 | `node scripts/build.mjs` | 0 |
| 业务目录 | `pnpm catalogs:check` | 0 |
| E2E | `node tests/e2e/run.mjs` | 13/13 通过 |
| 集成测试 | `go test -tags=integration ./src/apps/{iam,console}/...` | 除 2 项既有环境限制外全部通过 |

集成测试的两个失败均为既有环境限制，非本轮引入：

- `TestBackupRestoreRevokesRestoredCredentialsBeforeServing`：本机未安装 `pg_dump`（`command -v pg_dump` 为空），该用例要求与服务器主版本匹配的客户端工具。
- `TestConfigurationPrecedence`：同一 shell 里 `source .env` 污染了该用例的断言环境；在干净环境下通过。

### 实施中修正的计划偏差

| 项 | 原计划 | 实际 | 原因 |
|---|---|---|---|
| P1-5 slug 规则不一致 | 需统一 DB 与 Go | **无需改动** | 迁移 `00004` 已统一；我之前误读了被覆盖的 `00001` |
| 保留名单位置 | 在原计划中未细化 | 移入 `ValidateUsernameAvailable` | 若放在 `NormalizeUsername` 会拦住登录与配置托管账号（`admin`） |
| `role_ids` 必填 | 计划写「至少一个」 | 改为可选 | 创建组织时需先建账号再设为管理员，此时目标组织内尚无角色可授 |
| `members` 归属 | B' 方案独立标签页 | 已按 B' 实现 | 与 GitLab `Manage > Members` 同构 |
| `users` 页面 | 计划暗示移出导航 | 保留为「角色与权限」下的「账号」标签页 | 否则该能力完全不可达 |
| 组织切换器 | `Select` 保持 | 改为 `Popover` + 树 | 扁平列表无法表达层级 |

### 实施中发现的缺陷

- **`/me` 返回 `path: null`** 导致前端 Zod 校验失败、登录后不跳转。已改为前端接受 `nullish`，并明确 `/me` 不返回 path（面包屑由前端用 `parent_id` 组装）。
- **`OrganizationTree` / `VisibleOrganizations` 的 `Path []string` 无法被 GORM 扫描**（`unsupported data type: &[]`）。已改为 `string_agg` + 中间结构体 + Go 侧拆分。
- **密码规则收紧到 12 位后，默认 bootstrap 密码 `secplexus`（9 位）使服务无法启动**。已同步更新 `config.yaml` 默认值、`.env.example` 与 `config_test.go` 断言。

## 风险与约束

- **去 status 是本计划最易出错的一项**：`iam.permissions.status`（权限目录）与角色/组织的 status 同名但语义不同，改动时不可误删权限目录的过滤条件。第 3 组清单已标注该边界。
- **可见性语义变更**：`VisibleOrganizations` 从「平铺可见集」改为「分支可见集」，会影响顶部切换器、组织选择与 E2E 断言，需一次性同步。
- **深度上限 3 是产品决策**：以 CHECK 硬编码，未来加深需改触发器与递归查询。
- **不引入 tree 组件依赖**：自建树需自行保证键盘可达与窄屏不溢出，成本高于装第三方组件，但避免引入未审阅的外部代码。
- **二次认证保留**：只对创建成员关闭，重置他人密码仍要求；这是刻意的安全分级，不因本次简化而全面移除。
- **清空重建**：迁移不做兼容层，部署时直接重建数据库；这与「未发版」的前提一致，发版后此策略不再适用。

## 审查发现的 P0 / P1 问题与处置（实施前必读）

本节由计划复审产出，逐条对照真实源码验证。实施时按此清单逐项核对，不得跳过。

### P0（不处理会导致实施失败或语义错误）

| # | 问题 | 证据 | 处置 |
|---|---|---|---|
| P0-1 | `Seed` 硬编码创建 `slug="default"`，与「根组织必须是 `master`」冲突 | `repository/store.go:169` `CreateOrganization(ctx, tx, orgID, "default", "默认组织", userID, userID)` | 第 2 组必须修改该调用：改为创建/复用 `slug=master, name=AISOC`。同时把「最多一个根组织」唯一索引放在这条路径可覆盖的位置。集成测试需断言 `Seed` 执行后 `SELECT count(*) FROM iam.organizations WHERE parent_id IS NULL` 恰为 1 |
| P0-2 | `depth DEFAULT 0` 无法约束深度 | 计划原写 `depth smallint NOT NULL DEFAULT 0 CHECK(BETWEEN 0 AND 2)`；子组织默认取 0，CHECK 永不触发 | 改为**不设默认值**：`depth smallint NOT NULL CHECK(depth BETWEEN 0 AND 2)`；`CreateOrganization` 必须先读父节点 `depth` 再写 `parent.depth + 1`，父节点为 `NULL` 时写 0。集成测试覆盖「父 depth=2 时创建子组织被 CHECK 拒绝」 |
| P0-3 | 「users 移出导航但保留路由」未定义实现 | `management.tsx:46` 用 `managementPages.find(entry => entry.path(auth.org_id) === pathname)` 分发；从数组移除后命中 `!page` 直接返回 `AccessDenied` | 明确做法：`managementPages` 保留 `users` 条目（仍能匹配路由与权限），新增 `nav: false` 标记；`visibleManagementPages` 过滤时排除 `nav === false`，但 `ManagementPage` 的路由匹配仍用未过滤的完整列表。E2E 断言直接访问 `/management/platform/users` 仍可用 |
| P0-4 | 新增结果码未提注册表，且 `23505` 已被全局映射为 `conflict` | 码源 `src/packages/go/result/error-codes.json`（17 条，`module` 仅 `common`/`iam`）；`handler/server.go:99` 把 `23505`/`23503` 一并映射为 `CodeConflict` | 两件事：① 在 `error-codes.json` 增加 `iam.username_taken`（409）与 `iam.email_taken`（409），重新生成 `codes.gen.go` 与前端 `result.gen.ts`；② **不能依赖全局映射**——在仓储层捕获 `pgconn.PgError` 并按 `ConstraintName` 区分 `users_username_normalized_key` / `users_email_normalized_key`，分别返回上述专用码。集成测试断言重复用户名返回 `iam.username_taken` 而非 `conflict` |
| P0-5 | 删除组织的三种失败原因无法区分 | 全部外键无 `ON DELETE`（全库 `ON DELETE` 出现次数为 0），默认 `NO ACTION`；`server.go:99` 已把 `23503` 映射为 `409 conflict` | 不会 500（此前担心过重），但 `409` 无法告诉用户「因为有子组织」还是「因为有成员」。处置：`DeleteOrganization` 在事务内**先显式检查**子组织数与成员数，分别返回 `iam.organization_has_children` / `iam.organization_has_members`，并把它们加入 `error-codes.json`。外键错误仅作为兜底 |

### P1（不处理会导致返工、遗漏或体验不一致）

| # | 问题 | 证据 | 处置 |
|---|---|---|---|
| P1-1 | `members` 移入 `organization` 组后未定义页面组织方式 | `navigation.ts:46` 的 `overview` 在数组中先于 `organizations`；`visibleManagementSections` 取 `pages[0]` 作为分组入口 | **已由第二轮复审推翻 A 方案，改为 B'**：`当前组织` 与 `组织成员` 作为两个同级标签页，`members` 保留独立条目仅改 `group`。依据 8 个成熟产品无一采用单页堆叠，详见文末「推翻第一轮的 P1-1 结论」 |
| P1-2 | `Errors` 命名空间缺新结果码文案 | `zh-CN.json` 的 `Errors` 仅 10 项（generic/conflict/invalidRequest 等） | 为全部新增结果码补 `en`/`zh-CN` 两语言文案，并在 `errorMessageKey` 映射中登记。文案要能指导操作，例如「该用户名已被占用」而非「冲突」 |
| P1-3 | 「修改 slug 一律拒绝」超出需求，属功能收缩 | spec 原句；用户只要求 `master` 不可改 | 改为：仅 `slug=master` 拒绝修改；其他组织的 slug 修改也拒绝，但理由写清「slug 是 URL 与审计标识，改名请只改 name」。若后续需要改 slug，另立计划。**这一条已按收窄处理**：保留「一律拒绝」，但在 spec 补一句理由，避免被误读为遗漏 |
| P1-4 | 删 `status` 后 `ProtectLastAdministrators` 语义漂移 | `repository/roles.go:155-160` 中 `o.status='active'` 是「有效组织」的唯一判据 | 删列后该查询改为对所有组织生效（因为没有停用态）。需在实现时**显式确认**：这是期望行为（未删除即有效），并在该函数注释里写明「组织无停用态，故不加组织状态条件」。同时保留 `u.status='active'` 与 `m.status='active'`（成员仍有状态） |
| P1-5 | ~~两处 slug 正则长度不一致~~ **误报，已撤回** | 迁移 `00004_organization_identifiers.sql` 已将 `organizations_slug_check` 改为 `^[a-z][a-z0-9_-]{1,28}[a-z0-9]$`，与 Go `model/organization.go:35` 的 `organizationSlugPattern` **完全一致**。我初读的是 `00001` 的旧定义，被 `00004` 覆盖 | 无需改动。已用真实 PostgreSQL 查询 `pg_constraint` 确认线上约束即为该正则。第 1 组迁移中**不再包含** slug 规则统一项 |
| P1-6 | 未提 IAM README 与架构文档的组织模型更新 | `src/apps/iam/README.md` 描述现有平级组织 | 第 2 组完成后更新 `src/apps/iam/README.md` 的组织段落（树形、深度上限、根 `master`）；若 `docs/design/architecture/` 中有组织模型描述，同步更新。列入「受影响的配置与文档清单」 |

### 复审后对原计划的修正

- 第 2 组迁移中 `depth` 定义改为无默认值（P0-2）。
- 第 2 组新增对 `repository/store.go:169` 的修改（P0-1）。
- 第 3 组需在 `ProtectLastAdministrators` 加注释说明为何不再按组织状态过滤（P1-4）。
- 第 5 组明确 `managementPages` 的 `nav` 标记方案（P0-3）与「当前组织」合并展示方案（P1-1）。
- 第 6 组新增 `error-codes.json` 与 `result.gen.ts` 的改动，以及按约束名区分唯一键冲突的实现要求（P0-4）。
- 新增删除组织的专用错误码与前置检查（P0-5）。
- 「受影响的配置与文档清单」补充 `src/packages/go/result/error-codes.json`、`src/apps/iam/README.md`（P1-2、P1-6）。

---

## 第二轮全局复审（数据结构 / 关联 / 底层设计）

第一轮复审偏重业务逻辑与引用点，本轮补审数据库设计、表关系、缓存一致性与前端信息架构。

### P0（数据结构与关联层面）

| # | 问题 | 证据 | 处置 |
|---|---|---|---|
| P0-6 | **`depth` 无法用 CHECK 跨行校验** | PostgreSQL CHECK 不允许子查询，`CHECK(depth = (SELECT depth+1 FROM iam.organizations WHERE id=parent_id))` 非法 | 必须用 `BEFORE INSERT OR UPDATE` 触发器实现，复用现有 `check_organization_permission()` 的写法（`00001_identity.sql:58-67`）。触发器内：`depth = COALESCE((SELECT depth+1 FROM iam.organizations WHERE id = NEW.parent_id), 0)`，并在同一处校验 `depth <= 2`。服务层的计算保留作为快速失败，DB 触发器是最终保证 |
| P0-7 | **环检测缺失具体实现方案** | 环检测需要读祖先链；纯 CHECK 不可能做到 | 触发器内用递归 CTE 检测：从 `NEW.parent_id` 向上遍历，若遇到 `NEW.id` 则 `RAISE EXCEPTION ... ERRCODE='23514'`。必须放在同一触发器函数里，否则 UPDATE 换父节点仍可成环 |
| P0-8 | **组织树的权限语义未定义** | `repository/queries.go:120-128` 的 `GetOrganizationPermissions` 严格 `Where("m.org_id = ?")`，**权限不跨组织** | 明确写入契约：**父组织权限不继承到子组织**。子组织成员只有在子组织有 membership 才有权限；超管例外。这与 AiSOC 的 `org_scope` 教训一致（absent ≠ all）。集成测试必须覆盖「用户在父组织有 org_admin，访问子组织被拒」 |
| P0-9 | **`authz_epoch` 无树形传播策略** | `identityScope`（`query.tsx:21-28`）把当前组织 `authz_epoch` 放进所有查询键；`BumpOrganizationAuthzEpoch` 只 bump 单个组织（`authorization_queries.go:46`） | 明确：**组织树变更只 bump 被修改组织自身**，不传播到子组织。理由：权限不跨组织继承（P0-8），子组织缓存无需因父组织改名而失效。但「删除组织」必须 bump 其父组织，否则父组织视图（子组织列表）会陈旧——这一条要单独实现 |
| P0-10 | **`/me` 的 `organizations` 是 `map[string]string`，加 `depth` 会变字符串** | `service/self.go:36,44` 用 `map[string]string` 组装 | 改为具名结构体 `model.APIOrganization`，`depth` 用 `int`、`parent_id` 用 `*string`。前端 `zApiOrganization`（`schemas.ts:37-43`）同步为 `z.int()`。这是**破坏性类型变更**，需一次性改完所有消费方 |
| P0-11 | **删除组织缺少依赖清理顺序** | 依赖链：`membership_roles → memberships`、`role_permissions → roles`、`roles → organizations`；全库无 `ON DELETE` | 删除必须在单个事务内按序执行：先校验无子组织 → 删 `membership_roles` → 删 `role_permissions` → 删 `roles` → 删 `memberships` → 删 `organizations`。任何一步失败整体回滚。`audit_events.org_id` **无外键（`00001_identity.sql:88`）是正确设计，不要加外键**——审计必须能在组织删除后留存 |

### P1（数据结构与前端架构层面）

| # | 问题 | 证据 | 处置 |
|---|---|---|---|
| P1-7 | 组织列表 SQL 漏了两处 status | `lists.go:74` 的 `Select("id,slug,name,status,authz_epoch")`；`authorization_queries.go:63` 的 `r.status = 'active'` | 两处都要改。第一轮清单只列了 `roles.go`/`queries.go`/`store.go` |
| P1-8 | `Path` 字段的生成位置未定 | 组织树需要在详情页展示 `AISOC / test-a / test-a-2` | 由服务端在 `OrganizationAncestors` 里一次算好并在 `APIOrganization.path` 里返回（`[]string` 或已拼接的 `string`），**不要让前端逐级请求**。前端只做渲染 |
| P1-9 | 组织树查询的 N+1 风险 | `VisibleOrganizations` 目前是单条 SQL | 树用单条 `WITH RECURSIVE` 返回全部节点，前端按 `parent_id` 组装。禁止每层一次查询。对 500 上限内的规模足够 |
| P1-10 | `VisibleOrganizations` 的「分支可见」语义需要精确化 | 现状是「自己所属的组织」平铺 | 明确定义：普通成员可见 = 自己所属组织 + 其**祖先链**（用于面包屑定位），不含兄弟节点与其后代。超管可见全树。这个定义要写进集成测试，避免实现时理解偏差 |

### 推翻第一轮的 P1-1 结论（前端信息架构）

第一轮我推荐「A 方案：当前组织页内分「基础信息 + 成员」两个区块」。**调研后推翻该结论**，改为 **B' 方案：两个同级页面**。

**证据（8 个成熟后台产品，无一采用单页堆叠）**：

| 产品 | 模式 | 证据 |
|---|---|---|
| GitHub | org 级 `People` 页与 `Settings` 页**同级**；Settings 内部才有嵌套侧栏 | [People 页文档](https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-personal-account-on-github/managing-your-membership-in-organizations/viewing-peoples-roles-in-an-organization) |
| GitLab | `Manage > Members` 与 `Settings > General` **分属两个侧栏组** | [Group 文档](https://docs.gitlab.com/user/group/) |
| Atlassian | `Directory`（用户/组）与 `Organization settings` **两个顶级分区** | [Atlassian 管理文档](https://support.atlassian.com/organization-administration/docs/explore-an-atlassian-organization/) |
| Google Workspace | OU 页是**树浏览器**，成员在 Users 列表里编辑 | [Admin console map](https://knowledge.workspace.google.com/admin/getting-started/admin-console-map) |
| Vercel / Linear / Stripe / Keycloak | 单一 Settings 面 + 嵌套侧栏/标签，Members 是其中一项 | [Vercel RBAC](https://vercel.com/docs/rbac/managing-team-members)、[Linear](https://linear.app/docs/members-roles)、[Stripe Team](https://docs.stripe.com/get-started/account/teams) |

**结论**：成熟产品**没有一个**把「基础信息 + 配置 + 成员列表」堆在同一页的分区块里。原因是成员管理是列表密集型工作（搜索、筛选、批量操作、分页），配置是表单，两者权限门槛也不同（GitHub: People 所有成员可见，Settings 仅 owner）。

**因此采用 B' 方案**：

- 「组织管理」组下两个**同级标签页**：`当前组织`（基本信息 + 改名）与 `组织成员`（列表 + 分配角色 + 移除）。
- 这与 GitLab 的 `Manage > Members` / `Settings > General` 同构，也与本仓库「路由即功能」的约定一致。
- 删除第一轮写的「把 `MembersPage` 嵌入 `OverviewPage`」；`members` 保留为独立 `managementPages` 条目，仅改变其 `group` 归属（`people` → `organization`）。

### 对第一轮结论的修订

- P1-1 由「A 方案合并展示」改为「B' 方案同级标签页」，理由见上。第 5 组实现清单已同步。
- 第 3 组补充 `lists.go:74` 与 `authorization_queries.go:63`（P1-7）。
- 第 2 组补充触发器实现（P0-6/P0-7）与权限不继承契约（P0-8）。
- 第 6 组补充 `/me` 结构体化改造（P0-10）。
- 新增删除组织的依赖清理顺序（P0-11）。

---

## 第三轮最终审查（实施前阻塞项）

本轮对前两轮的结论做了实测验证（真实 PostgreSQL），并补出两个遗漏。

### P0-12：创建成员时「加入哪个组织」未定义

**问题**：`service/accounts.go:46` 的 `CreateUser` 完全没有组织上下文，只创建全局账号。新需求要求「在角色与权限页创建成员并选择角色」，而**角色是组织内的**（`iam.roles.org_id`）。当前端位于某个组织上下文时，创建出来的成员应加入哪个组织？计划未定义。

**处置**：

- 请求携带 `org_id`（必填）。前端从 `useIdentity().org_id` 取当前组织，不在界面上让用户选。
- 后端 `CreateUser` 增加 `orgID` 参数，在同一事务内：
  1. 校验请求者对 `orgID` 有 `iam.members.manage`；
  2. 创建 `iam.users` + `iam.password_credentials`；
  3. 创建 `iam.memberships(org_id, user_id)`；
  4. 按 `role_ids` 写 `iam.membership_roles`，每一条都走 `CheckDelegation`。
- **若当前组织是根 `master`**：允许，成员就属于 `master`（超管所在的组织）。
- **若用户当前无组织上下文**（`org_id` 为空）：拒绝，返回 `invalid_request`。不允许创建「无组织成员」。
- 集成测试必须覆盖：在 `test-a` 创建的人，在 `test-b` 不可见；角色必须属于 `test-a`，传 `test-b` 的角色返回 `ErrInvalid`。

### P0-13：三个迁移的执行顺序有隐含依赖

**问题**：`00009`（约束）、`00010`（组织树）、`00011`（删 status）在本计划中按编号顺序执行，但存在依赖：

- `00010` 的组织树触发器函数若引用 `status`，而 `00011` 才删 `status`，则顺序反了会失败。
- `00010` 要创建根组织 `slug=master`，而 `organizations.slug` 的 CHECK 在 `00009` 收紧。若 `00009` 的规则不接受 `master`，则 `00010` 失败。

**处置**（已实测确认可行性）：

- 固定顺序：`00009` → `00010` → `00011`。
- `00009` 的 slug CHECK 必须接受 `master`（实测：`^[a-z][a-z0-9_-]{1,28}[a-z0-9]$` 接受 6 位 `master`）。
- `00010` 的触发器函数**不得引用 `roles.status` 或 `organizations.status`**，只操作 `parent_id` 与 `depth`。这样 `00011` 删列时不会破坏触发器。
- `00011` 删 status 后，需确认 `00010` 创建的触发器仍有效（用 `\d+ iam.organizations` 或集成测试验证）。

### 已实测验证的结论（真实 PostgreSQL）

以下三条此前是推断，本轮已实跑确认：

| 结论 | 实测结果 |
|---|---|
| `CREATE UNIQUE INDEX ON t.o((parent_id IS NULL)) WHERE parent_id IS NULL` 可保证唯一根 | 通过；插入第二个根被拒（`duplicate key value violates unique constraint`） |
| CHECK 约束不能含子查询，故 `depth` 必须用触发器 | 通过；`ALTER TABLE ... CHECK(depth = (SELECT 1))` 报 `cannot use subquery in check constraint (SQLSTATE 0A000)` |
| 单个 `BEFORE INSERT OR UPDATE` 触发器可同时实现「算 depth + 限深 3 + 防环」 | 通过；`root→child→grandchild` 成功，第 4 层被拒，`UPDATE` 成环被拒，`grandchild.depth=2` 正确 |
| `audit_events` 确实无外键，删组织后审计留存 | 通过；`pg_constraint` 查询 FK 数量为 0 |
| `memberships`/`roles` 对 `organizations` 均为 `NO ACTION` | 通过；删除组织必须先清理这两张表 |

### 触发器参考实现（实测通过，可直接落地）

```sql
CREATE FUNCTION iam.organization_tree_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_depth int; cursor_id uuid;
BEGIN
  IF NEW.parent_id IS NULL THEN
    NEW.depth := 0;
    RETURN NEW;
  END IF;

  SELECT depth INTO parent_depth FROM iam.organizations WHERE id = NEW.parent_id;
  IF parent_depth IS NULL THEN
    RAISE EXCEPTION 'parent organization not found' USING ERRCODE = '23503';
  END IF;

  NEW.depth := parent_depth + 1;
  IF NEW.depth > 2 THEN
    RAISE EXCEPTION 'organization depth exceeds limit' USING ERRCODE = '23514';
  END IF;

  cursor_id := NEW.parent_id;
  WHILE cursor_id IS NOT NULL LOOP
    IF cursor_id = NEW.id THEN
      RAISE EXCEPTION 'organization cycle detected' USING ERRCODE = '23514';
    END IF;
    SELECT parent_id INTO cursor_id FROM iam.organizations WHERE id = cursor_id;
  END LOOP;

  RETURN NEW;
END $$;

CREATE TRIGGER organization_tree_guard_biu
  BEFORE INSERT OR UPDATE ON iam.organizations
  FOR EACH ROW EXECUTE FUNCTION iam.organization_tree_guard();
```

注意：`depth` 列不加 `DEFAULT`，由触发器赋值（P0-2）。`WHILE` 循环在深度上限 3 的约束下最多迭代 3 次，无性能问题。

### 第三轮对前文的修订

- 第 6 组补充 `org_id` 必填与「无组织上下文拒绝」（P0-12）。
- 第 2 组补充迁移顺序约束（P0-13）与触发器参考实现。
