# 组织树、角色权限模型与成员管理重构

状态：待审核，未实施。范围限于组织层级、管理信息架构、成员创建与角色权限归属；不做数据权限（行级/对象级授权）的实现，只预留其接入点。

项目尚未发布，**不做数据迁移**。所有迁移按「清空重建」处理：新约束直接收紧，旧数据不保留、不回填、不做兼容层。

## 背景

当前「系统管理」把三件事混在同一组「成员与权限」下：组织成员、角色与权限、平台成员。职责重叠导致两个问题：

- 组织成员是「某个组织内的成员关系」，却和全局的平台账号管理并列，用户无法从信息架构上判断二者边界。
- 角色与权限页面既管角色，又通过成员页分配角色，同一个概念分散在两处。

用户要求：

1. 「成员与权限」改名为「角色与权限」，只管角色和「创建平台成员」。
2. 组织成员移到「组织管理」，与组织基础信息放在一起。
3. 「全部组织」只负责组织的创建、编辑、删除。
4. 创建成员时录入用户名、显示名称、联系邮箱、密码、角色，字段校验要专业、前后端一致、有去重；密码需两次输入确认。
5. 权限挂在角色上，不挂在组织上。
6. 组织改为树形：全局唯一根组织，标识固定 `master` 不可改，名称可改；其下可有子组织、孙组织。
7. 顶部组织切换器改为树形下拉并定位当前组织；其他详情展示当前分支而非单个组织名。
8. 角色与组织**取消启用/停用状态**，只要未删除即可用；成员保留 `active/disabled`。
9. 取消创建成员时的「当前密码二次确认」流程，改为「密码 + 确认密码」两次输入。

## 调研依据

### 组织层级（含本地 AiSOC 源码核实）

| 系统 | 容器层级 | 最大深度（有据） | 继承语义 |
|---|---|---|---|
| GitHub | Enterprise → Orgs → 嵌套 team | 组织不嵌套；team「multiple levels」，官方未给数值上限 | 向下单向、叠加；子团队继承父团队仓库权限，成员不向上传递 |
| Google Cloud | Organization → Folders → Projects | 文件夹 **10 层**，每层最多 300 个直接子项 | allow 策略向下取并集；拒绝走独立 deny 通道，deny 优先 |
| AWS Organizations | Root → OU → Account | **5 层 OU**，全局最多 2000 个 OU | SCP 向下继承，作为权限上限，任意层 deny 不可被下方覆盖 |
| Kubernetes + HNC | 扁平 namespace + HNC 树 | 官方「无硬限制」 | 控制器把 RBAC/配额向下同步，实现复杂、运维脆弱 |
| Slack Enterprise Grid | Org → Workspace | 仅 1 层 workspace | 不嵌套；跨工作区靠共享频道（显式授权）而非层级 |
| AiSOC（本地源码） | 扁平 tenant + organization 运营商组合 | 第 1 代 `parent_tenant_id` 自引用树已废弃 | 两轴分离：breadth（可见范围）+ authority（可做什么） |

**本地 AiSOC 源码证据**（本地仓库 commit `b0797314`）：

`services/api/app/models/organization.py` 文件头注释直接写明第 1 代为何被废止：

> `tenants.parent_tenant_id` (migration 012) makes the managing provider a tenant, which is enough to draw a tree and not enough to run a managed service.

即：父租户指针「足以画一棵树，但不足以运营托管服务」。第 2 代拆成四张表——`Organization`（运营商）、`OrganizationMember`（谁属于它、什么权限）、`OrganizationTenant`（托管组合）、`OrganizationMemberTenant`（把单个成员限定到组合的子集）。

`services/api/app/services/org_scope.py` 的核心不变量，是该仓库最值得借鉴的部分：

> A member with no grants resolves to an **empty** portfolio, never an unfiltered one. Every cross-tenant leak this codebase has had took the same shape — a scope that was absent rather than narrow, and a query that treated absent as "no filter".

并列举了三次真实越权：`get_entity_neighbors` 接受 `tenant_id` 却从不绑定；`_events_of_interest` 把 API 生成的 SQL 交给面向不可信 SQL 的重写器且不做校验，导致每个新租户看到同一份全局漏斗；`rewrite_for_tenant` 返回未限定的 SQL，解析器移动一个 dict key 就报告成功。

它还刻意把两轴分开，理由写在同一个 docstring 里：

> breadth: owner/admin reach the whole portfolio. operator/viewer reach only what `organization_member_tenants` grants them.
> authority: owner/admin/operator may act. viewer is read-only.

**结论**：生产系统普遍选择「浅而硬性有界」（AWS 5、GCP 10 但强引导、Slack 1）。AiSOC 从自引用树退回到「边界与运营商分离 + 单点范围解析」。本项目取**深度 3 层**，并把「范围解析只有一个入口」写进契约，为后续数据权限留出正确形状。

### 权限目录架构

| 系统 | 权限声明位置 | 新功能如何进入角色编辑器 |
|---|---|---|
| SpiceDB | schema 文件（relation/permission 表达式） | 改 schema 并部署；关系是数据 |
| Ory Keto | OPL schema；RBAC 指南把权限建模为对象上的具名 relation | 权限属 schema、角色属数据 |
| Keycloak | 每个 resource-server client 下的 scopes/policies/permissions | 管理端从 client 配置发现 |
| Casbin | 策略是存储里的数据，模型是配置 | 权限字符串自由填写，无目录保证一致性 |
| AiSOC（本地源码） | `permissions` 表是**平台级目录**，注释写 `Seeded via migration`，带 `category` | 单一 effective-permissions 接口供 UI 使用 |

AiSOC `services/api/app/models/rbac.py` 源码：

```python
class Permission(Base):
    """Platform-wide permission catalogue.  Seeded via migration."""
    __tablename__ = "permissions"
    ...
    category: Mapped[str | None] = mapped_column(String(100), nullable=True)

class Role(Base):
    """Tenant-scoped role (e.g. admin, analyst, viewer or custom)."""
```

即权限是平台级目录（不属于任何租户），角色按租户隔离，`RolePermission` 多对多。这与本项目现状同构。

本项目现状（已核对源码，无需重建）：

- `src/apps/iam/internal/authorization/permissions.json` 是唯一权限源，含 9 个字段。
- `scripts/generate-catalogs.mjs` 生成 Go/TS 常量，并校验重复 key、scope 合法性（`platform` 必 `delegable=false`）、导出名冲突。
- `iam.permissions` 表与 JSON 同步，`GET /api/v1/iam/permissions` 返回目录。
- `src/spx/pages/roles.tsx:312` 已按 `resource_type` 从目录动态分组渲染。

**结论**：目录链路已具备「声明一次、UI 自动出现」，本计划只补两个缺口（分组显示名、把唯一入口写进规范），不替换该机制。

### 字段校验

| 字段 | GitHub | GitLab | Entra ID（UPN） | 建议规则 |
|---|---|---|---|---|
| 用户名 | 字母数字 + 单连字符，首尾非连字符，≤39 | ASCII 字母数字 `_-.`，2–255，首尾非特殊符，无连续特殊符，保留路由前缀 | 不适用（UPN 本地部 `A-Za-z0-9'._-!#^~`，≤64） | **3–39 位，`[A-Za-z0-9_.-]`，字母开头，无连续特殊符，小写唯一键，保留名单** |
| 显示名称 | 自由文本 | 长度校验 | 必填 String，无格式约束 | **1–100 字符，trim，任意 Unicode，不做正则** |
| 邮箱 | HTML5 级语法 + 验证邮件 | 同 | 不得含重音符 | **HTML5 正则 + 本地部 ≤64 + 总长 ≤254 + 唯一键** |
| 密码 | — | — | 默认 8–256，四类取三（与 NIST 相左） | **12–128，任意可打印字符，无组合规则** |

邮箱校验依据：WHATWG HTML 规范对 `input type=email` 的校验明确标注为「a willful violation of RFC 5322」，被业界当作务实标准；RFC 5322 全量正则不可行（Haack/is_email 均有论述）；长度上限 254 来自 RFC 5321，本地部 64。

去重依据：`username_normalized` 已存在（`lower(username)` + UNIQUE）；邮箱**缺唯一约束**，需补 `lower(email)` 唯一索引。不做 Gmail 点号归一（该规则仅对 gmail.com 消费者账号成立，无法外部判断），不剥离 `+tag`（RFC 5233 子地址合法）。

密码依据：NIST SP 800-63B-4 §3.1.1.2——最小 8（多因素）/15（单因素），SHOULD 支持至少 64，禁止组合规则，禁止定期改密，必须全量比对黑名单。

**关于泄露名单的取舍**：NIST 要求比对黑名单，但那是「推荐实现」而非发版前必做。本项目尚未发版，引入离线字典会带来持续维护成本与体积。本计划**不做泄露名单**，只把密码长度提到 12，并在 spec 记录该决定与理由，避免过度设计。

### 二次认证 UX（本次要修的真实缺陷）

现状因果链（已核对源码）：

1. `permissions.json` 给 `iam.platform.users.manage` 标了 `requires_recent_auth: true`。
2. `authorization/policy.go:34` 的 `Evaluate` 在认证窗口过期时返回 `ErrRecentAuthRequired`。
3. `forms.tsx:112` 捕获该错误码，把 `FormDialog` 切到「确认当前密码」页。
4. 创建成员时用户填的是**新成员密码**，界面却要求「当前密码」，且标签是「当前密码」、提示「密码不正确」。

结论：安全机制本身没错（重新认证是真需求），错在**流程位置与文案**：它嵌在创建表单之后，让用户把「新成员密码」误当成「当前密码」。按用户要求，创建成员**取消这段二次认证**；二次认证保留给「重置他人密码」这类真正高危的操作。

## 现状核实

- `iam.organizations` 无 `parent_id`，确为平级；当前库只有 1 个组织 `slug=default, name=AISOC`，无 `master` 概念。
- 组织 slug 规则**两处不一致**：DB CHECK `^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`（允许数字开头、不允许下划线）vs Go `^[a-z][a-z0-9_-]{1,28}[a-z0-9]$`（要求字母开头、允许下划线）。既有缺陷，顺带统一。
- 用户名规则三处一致但各自手写：DB `^[A-Za-z][A-Za-z0-9_.-]{2,63}$`、`identity.usernamePattern`、前端 Zod。
- `iam.users.email` 无唯一约束，仅长度上限 320。
- 密码规则散落 5 处 DTO `binding:"min=8,max=128"` 与 `identity.ValidatePassword`。
- 角色与组织的 `status` 被**大量查询**当作有效过滤条件：`read_queries.go:40`、`authorization_queries.go:31`、`roles.go:155-160`、`store.go:177/195`、`roles.go:45/185`（列表投影）。删字段需同步改这些 SQL。
- 前端 `roles.tsx`/`organizations.tsx` 均有 status 列与表单字段；`schemas.ts` 有 `zApiRole.status` 枚举与 `zApiOrganization.status`。
- 已存在委派上限校验 `authorization.CheckDelegation`：只能授予自己已拥有且 `delegable` 的权限；`platform` scope 永不可 delegable（生成器已断言）。
- 顶部组织切换器 `shell.tsx:170` 是扁平 `Select`，直接渲染 `auth.organizations`。
- 组织可见性由 `repository.VisibleOrganizations` 决定：超管可见全部，否则仅自己所属组织。
- shadcn 官方**无 tree 组件**（`npx shadcn search @shadcn -q tree -t registry:ui` 返回 `No items found`）；仅 `sidebar-11` 用 Collapsible 演示文件树。项目未配置社区 registry。
- `FormInput`/`FormDialog` 已正确使用 `Field data-invalid` + `aria-invalid` + `FieldError`。

## 范围

### 一、信息架构重划

| 分组 | 页面 | 职责 |
|---|---|---|
| 角色与权限（原「成员与权限」） | 角色 | 创建、编辑、删除角色；编辑角色权限；查看角色成员 |
| 角色与权限 | 账号 | 平台账号列表；创建成员（含角色）；编辑、重置密码 |
| 组织管理 | 当前组织 | 组织基础信息 + 改名 |
| 组织管理 | 组织成员 | 该组织成员列表（分配角色、移除成员） |
| 组织管理 | 全部组织 | 组织树浏览 + 创建/编辑/删除组织 |
| 审计日志 | 组织审计 / 平台审计 | 不变 |

「平台成员」独立导航页**取消**（已确认），其能力并入「角色与权限 → 账号」。

### 二、组织树

- 全局唯一根组织，`slug` 固定 `master`，创建后不可修改；`name` 可改（默认 `AISOC`）。
- 其余组织均为 `master` 的后代，单父节点。
- **深度上限 3 层**（根 → 子 → 孙）。理由见调研。
- 禁止把父节点挂到自己的后代下（环检测）。
- 删除组织需先处理子组织与成员，禁止级联删除。
- **取消组织的启用/停用状态**，`status` 列删除。

### 三、角色

- **取消角色的启用/停用状态**，`status` 列删除；只要未删除即可用。
- 保留系统角色保护（`org_admin`/`member` 不可删、不可改 code）。
- 创建/编辑角色的权限沿用现有委派上限。

### 四、权限目录扩展契约

保留 JSON → codegen → DB → UI 链路，补齐：

1. `permissions.json` 增加 `group`（面向用户的分组显示名键）；生成 `permissionGroups.gen.ts`，角色编辑器分组与顺序从生成物读取。
2. 把「新增业务模块声明权限的唯一入口」写进 `AGENTS.md` 与 `docs/development/role-management.md`。

### 五、字段校验统一

Go `identity` 包作为唯一事实来源，DB 约束与前端 Zod 由它派生，并用单元测试锁定三处一致。

### 六、成员创建

- 六字段：用户名、显示名称、联系邮箱、密码、确认密码、角色。
- **密码两次输入**，前端用 Zod `refine` 校验一致，后端只收一次 `password`。
- **取消创建流程的二次认证**：创建成员不再触发「确认当前密码」。重置他人密码保留二次认证。

### 明确不做

- **不做数据迁移**（项目未发版，直接清空重建）。
- 不做数据权限（行级/对象级）实现，只为它预留 `org_id` 与祖先集。
- 不引入 SpiceDB/Ory Keto/Casbin 等外部策略引擎。
- 不做组织级权限绑定（权限只挂角色）。
- 不做组织 reparent 的完整 UI。
- 不引入 tree 组件依赖，用现有 Collapsible 组合。
- **不做密码泄露名单比对**（理由见调研）。
- 不引入邮箱验证邮件流程（本轮只做语法校验与唯一性）。

## 行为契约

### 组织

- `GET /api/v1/iam/organizations/tree` 返回当前用户可见的组织树（`id/slug/name/parent_id/depth/children`）。
- 根组织 `slug=master` 由迁移创建（清空重建，无需保留旧 id）。
- 创建组织必须指定 `parent_id`，未指定时默认挂 `master`。
- 深度超过 3 拒绝，返回 `invalid_request`。
- 修改 slug 一律拒绝（含 `master` 与普通组织）。理由：slug 是 URL 路径与审计记录里的稳定标识，改名会切断既有链接与历史关联，改展示名请只改 `name`。此处是对现状的功能收窄，不是遗漏。
- 删除：存在活跃子组织时拒绝；存在成员时要求先移除成员。

### 角色

- 创建/更新角色不再接受 `status` 字段；请求体带 `status` 视为未知字段拒绝（`DisallowUnknownFields` 已启用）。
- 角色可用性只由「是否存在」与「是否系统角色」决定。

### 成员创建

- `POST /api/v1/iam/users` 扩展 `org_id`（必填）与 `role_ids`（必填，至少一个），在同一事务内创建账号、密码凭据、成员关系与角色绑定。
- `org_id` 取当前组织上下文，前端不让用户选择；`org_id` 为空时拒绝（不允许创建不属于任何组织的成员）。角色必须属于该 `org_id`。
- 用户名：3–39 位，字母开头，`[A-Za-z0-9_.-]`，无连续特殊符，不区分大小写唯一，保留名单拒绝。
- 显示名称：trim 后 1–100 码点。
- 邮箱：可选；HTML5 正则 + 本地部 ≤64 + 总长 ≤254；`lower(email)` 唯一。
- 密码：12–128 码点，不接受 NUL，无组合规则。
- 角色：至少一个，必须在当前组织内存在、`delegable` 且请求者自己已拥有（复用 `CheckDelegation`）。
- **不触发二次认证**：`iam.platform.users.manage` 改为 `requires_recent_auth: false`。重置他人密码保留 `true`。

### 顶部组织切换

- 展示当前组织名称（单行）。点击展开树形下拉，自动展开到当前分支并高亮。

### 详情展示

- 组织上下文出现处展示分支面包屑（`AISOC / test-a / test-a-2`）。
- 紧凑场景（表格单元格、徽章）显示短名，`title` 提供完整路径。

## 验收标准

1. 「角色与权限」下不再有「组织成员」与「平台成员」两个独立页；角色与账号同组。
2. 「组织管理」下「当前组织」与「组织成员」为两个同级页面：前者管基础信息与改名，后者管成员列表与角色分配。依据 8 个成熟后台产品的调研，成员管理（列表密集）与配置（表单）分置是主流做法，无一采用单页堆叠。
3. 「组织管理 → 全部组织」只提供树浏览与创建/编辑/删除。
4. 创建成员表单为六个字段（含确认密码），两次密码不一致时前端就地报错。
5. 创建成员全程**不出现**「当前密码」确认页。
6. 用户名/邮箱重复返回可区分结果码，前端映射为字段级错误。
7. 组织为树形，深度上限 3，根 slug 恒为 `master` 不可改，名称可改。
8. 角色与组织都没有启用/停用状态，前端无 status 列与控件。
9. 成员保留启用/停用，前端可切换。
10. 只改 `permissions.json` 新增一个模块条目并重新生成，角色编辑器自动出现新分组。

## 后续

- 数据权限：基于祖先集与显式授权实现行级过滤，`resolveScope` 收敛为唯一入口。
- 组织 reparent、批量迁移。
- 邮箱验证邮件与自助找回。
- 密码泄露名单（发版后按需引入）。
