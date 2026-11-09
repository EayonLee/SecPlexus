# 组织授权继承与成员视图修正

状态：待审核，未实施。

本文承接 [org-tree-and-role-model 的诊断](../org-tree-and-role-model/analysis.md)。只处理需要决策的部分：角色继承模型、成员列表聚合、管理员不变量、组织管理界面。

> **已被取代**：本计划中「每个组织必须保留至少一个管理员」的不变量，已由
> [审计与角色管理简化、设置区合并](../audit-role-settings-consolidation/spec.md) 移除。
> 组织层不再强制管理员，管理员能力改由自定义角色 + 权限达成。本文件保留为当时的设计记录。

## 一、两处需要修正我此前的判断

### 修正 1：depth 从 0 起算；层级上限的结论已被推翻并更正

#### 1a. 命名先澄清

`depth` 从 **0** 开始：

| depth | 通俗说法 | 当前库实例 |
|---|---|---|
| 0 | 根（第 1 层） | `master` / AISOC |
| 1 | 第 2 层 | `test-a` |
| 2 | 第 3 层（当前最深） | `test-a-1` |

「二级组织能不能当父级」：**能**，depth=1 完全可以。不能的是 depth=2（第 3 层），因为子级会到 depth=3。我之前只写结论没解释编号，造成误解。

#### 1b. 更正：我上一轮的行业结论是错的

我此前写「生产系统普遍选择浅而硬性有界」，只举了 AWS 5 / GCP 10 / Slack 1。**这是选择性取证。** 补齐 13 个系统后的事实分布：

| 类别 | 数量 | 系统 |
|---|---|---|
| **无数字上限**（只有结构性约束：单父、禁环） | **8** | GitHub teams、Keycloak groups、AD/LDAP OU、K8s HNC、Jira、ServiceNow、Workday、Salesforce（仅建议值） |
| 硬性数字上限 | 3 | Google Cloud 10、AWS 5、GitLab 20 |
| 完全扁平（不能嵌套） | 2 | Entra ID admin units、Okta groups |

**多数（8/13）不设硬性层级上限**，只给性能建议，典型建议值 3–5。

关键引证：

- **GitHub**：*"You can reflect your group or company's hierarchy … with multiple levels of nested teams."* 全文无深度数字，只有结构约束（单父、secret team 不能再嵌套）。
- **Keycloak**：*"Groups are hierarchical. A group can have multiple subgroups but a group can have only one parent."* 无深度限制。
- **AD/LDAP**：*"While there is no technical limit to the number of levels in your OU structure, for manageability we recommend that you limit your OU structure to a depth of no more than 10 levels."* —— **显式声明无技术上限**。
- **K8s HNC**：*"Is there a limit to how many levels of child namespaces you can have? **No, HNC does not impose any limitation.** We've tested about a hundred … 3-5 levels should probably be more than enough."*
- **GitLab**：*"Be nested up to 20 levels. To avoid performance problems, nest groups to a maximum of five levels or fewer."* 20 硬顶，5 建议。
- **AWS**：*"OU maximum nesting in a root | Five levels of OUs deep under a root."*
- **GCP**：*"You can nest folders up to 10 (ten) levels deep."*，API 用 `FAILED_PRECONDITION` 强制。

#### 1c. 修正后的决定：取消 3 层硬上限

**当前 3 层硬上限比几乎所有这些系统都更严，应移除。**

采纳 GitHub / Keycloak / AD / HNC 的做法——**不限深度，只保留结构性约束**：

- 单父节点（已有 `parent_id`）。
- 禁止成环（已有 `organization_tree_guard` 的 `WHILE` 上溯）。
- 不设业务数字上限；改为文档化建议「建议 ≤5 层」，UI 超深时提示但不阻断。
- **保留数据库兜底** `CHECK(depth BETWEEN 0 AND 1000)`：`depth` 列类型是 `smallint`（`00010:7`，上限 32767），完全取消约束等于裸奔。1000 远高于任何真实用量（最深的 AD 建议 10、GitLab 硬顶 20）。

不选 GitLab 式「硬顶 20 + 建议 5」：20 这个常量要在迁移、触发器、CHECK、测试、文档、前端六处同步维护，与业务无关，换来的保护近乎为零。GitHub 式「不限 + 建议」维护成本最低、行为最可预期。

#### 1d. 移除上限的改动点（已核实）

**必须改**（写在新迁移里，不改 00010）：

| 位置 | 现状 | 改为 |
|---|---|---|
| `migrations/00010:42` | `IF NEW.depth > 2 THEN RAISE` | 移除判断 |
| `migrations/00010:66` | `CHECK(depth BETWEEN 0 AND 2)` | 移除 CHECK |
| `migrations/00010:88` | 列注释「深度上限 3」 | 改注释 |
| `organization_tree_integration_test.go:56` | 断言第 4 层被拒 | 改为「深链可建」+「环被拒」 |
| `src/apps/iam/README.md:45,676` | 「深度上限 3 层」 | 改描述 |

**不需要改**（已递归实现）：

- 前端 `OrganizationTree`：`render(node.id, level + 1)`。
- 后端递归 CTE：`read_queries.go:46`、`organizations.go:48`。
- 环检测：`WHILE` 上溯。

#### 1e. 取消上限的前提护栏（当前一个都没有）

| 风险 | 实测现状 | 必须补 |
|---|---|---|
| 递归查询慢 | `parent_id` **无可用索引**（`single_root` 是部分索引，谓词 `parent_id IS NULL`），EXPLAIN 显示 `Seq Scan` | `CREATE INDEX organizations_parent_id` |
| 树查询无上限 | `OrganizationTree` SQL 无 `LIMIT`（`organizations.go:57`） | 加 `LIMIT` 或分页 |
| 树被静默截断 | `VisibleOrganizations` 有 `LIMIT 500`（`read_queries.go:57`） | 超限显式报错或返回 `truncated`，不静默 |
| 前端递归无保护 | `organization-tree.tsx:94` 无限递归 | 超深时折叠并提示 |
| 环检测每层一次查询 | `00010:47` `WHILE` + `SELECT` | 依赖 `parent_id` 索引 |
| **递归 CTE 遇环无限递归** | 现有 `VisibleOrganizations` 用 `UNION`（去重，环上终止）；实测 `UNION ALL` 在 A→B→A 上**超时** | 所有递归 CTE 一律用 `UNION`，不用 `UNION ALL` |

这五条是取消上限的前提，不是可选项。

### 修正 2：成员列表聚合——这是我此前说错的地方

我在上一轮分析中说「子组织成员不出现在父组织列表」是行业标准。**调研结果不支持这个简化结论**，实际分成两派：

**A 派：授权作用域树**（GitLab、Keycloak）

- 成员是「每个容器一行」，父的成员列表默认**只显示直接成员**。
- GitLab API 明确：`GET /groups/:id/members`「Returns only direct members and not inherited members through ancestor groups」；`/members/all` 加的是**祖先**成员，不是后代。
- GitLab 教程原话：加入 Engineering 子组的人「are not members of the parent group」。
- Keycloak 同样默认直接成员，但 UI 有显式的「**Include sub-groups users**」复选框。

**B 派：目录包含树**（LDAP/AD OU、Google Workspace OU）

- 用户**只属于一个 OU**，树就是它的位置。
- 「列出这个容器下的用户」是一次搜索，聚合与否由调用方选 scope：RFC 4511 定义 `baseObject` / `singleLevel` / **`wholeSubtree`**（「baseObject 命名的条目及其所有下级」）。
- Google 自身也不统一：Directory API 的 `orgUnitPath` 查询**聚合整个子树且不能排除**；而 Data Export 明确「**Only the direct members** of the selected organizational unit are exported—not the sub organizational units」。

**两派都不认为「聚合列表 = 授予权限」**。

**所以用户说的「一级组织下面显示所有成员」是对的，但它是 B 派语义；而我们的数据模型是 A 派**（`memberships` 一行绑定一个 `org_id`，角色按组织定义、向下继承）。

结论：**用户诉求可以实现，但要作为「子树聚合视图」显式提供，而不是偷偷改变默认语义**。采用 Keycloak + RFC 4511 的做法：

- 成员页增加开关：**「包含下级组织成员」**（默认关）。
- 打开后用递归 CTE 聚合子树，并给每行标注**所属组织**列（否则同一人在不同组织的角色无法解释）。
- API 用 `scope=direct|subtree` 参数表达，不用布尔，为以后「只看直接下级」留位置。
- **仅组织成员列表加 scope。角色成员列表不加**：`iam.roles.org_id` 是 NOT NULL 且 `UNIQUE(org_id, code)`，一个角色只属于一个组织，其授予记录不存在于子孙层级，subtree 对角色成员无意义。
- 子树视图**不去重**：`memberships` 的唯一约束是 `UNIQUE(org_id, user_id)`，跨组织可重复，同一用户可能以不同角色出现在父子组织各一行。

这样默认行为仍是 A 派（不制造「列出即授权」的错觉），用户想要的聚合也拿得到，且与 RFC/Keycloak 的既有做法对得上。

## 二、其余决策

### 决策 1：祖先角色向下继承

GitLab / GitHub / Keycloak 一致：**父组织授予的角色在全部后代自动生效；反向不成立；最高角色生效；子节点只能加强不能削弱。**

被否：不继承（树成装饰）、纯显式共享（要额外建表建 UI，收益不如默认）。

### 决策 2：成员资格不向上

子组织成员不出现在父组织成员列表。这是 GitLab 与 GitHub 的明确语义，也是 A 派核心。与修正 2 不冲突：修正 2 讲「父看子」（向下聚合，可开关），这里讲「子看父」（向上不可见，永远不行）。

### 决策 3：平台超管不伪造 membership

GitLab（Administrator 是 user type）、GitHub（org owner 隐式拥有全部仓库 admin）、Keycloak（master realm 的 `admin`）都不把超管塞进每个组织。

保持不出现在成员表，但成员页加只读说明行：「平台管理员对本组织拥有全部权限，不计入成员席位」。

### 决策 4：管理员不变量改为按组织检查，并从改名移除

`ProtectLastAdministrators` 当前是全局扫描，且挂在组织改名上。改名不改变成员关系，却因别处脏数据失败——纯设计错误。

- 新增 `ProtectOrganizationAdministrators(ctx, tx, orgID)`，只查目标组织。
- 改名路径完全移除管理员检查。
- `ProtectLastAdministrators` 只保留全局超管存在性检查。

### 决策 5：管理员不变量放应用层，不用数据库触发器

**这一条在缺陷审查中被推翻重写**（见 [review-findings.md](review-findings.md) 缺陷 1）。

原设计「加 DELETE 触发器保证每组织 ≥1 org_admin」会与「删除组织需零成员」构成死锁：`repository.DeleteOrganization` 先删 `memberships` 再删 `organizations`，删 memberships 的那一刻组织行还在，触发器必然抛错 → 任何组织永久不可删除。

GitLab / GitHub / Keycloak 全部把该不变量放在**应用层**，且**删除组织路径不检查它**。GitLab 是 `GroupMemberPolicy` 的 `prevent :destroy_group_member`，`Groups::DestroyService` 只校验删除权限。

因此：

- 不变量实现为 `ProtectOrganizationAdministrators(ctx, tx, orgID)`，**只在成员移除、降权、角色分配路径调用**。
- **删除组织路径不调用任何管理员检查**，且移除「零成员才能删」前置条件。
- **不建 DELETE 触发器**。若将来要数据库兜底，必须用 `DEFERRABLE INITIALLY DEFERRED` 约束触发器，且体内先判断父组织是否仍存在；禁止用 `pg_trigger_depth()`（PostgreSQL 作者明确说它在 deferred trigger 下不可靠）。
- `VerifySchema` 的零管理员自检改为**记录 error 日志并继续**，不硬失败——它只在 `serve` 调用，硬失败会让历史脏数据锁死服务且无修复入口。修复动作放 `maintain`。

本次 409 的根因是不变量被错误地挂在改名路径上（决策 4），而非「缺少数据库约束」。

### 决策 6：删除脏数据（用户已授权）

已复核，删除安全：

```
FK 引用 organizations 的只有三处：memberships.org_id、roles.org_id、organizations.parent_id，
均 ON DELETE NO ACTION（必须手工按序删）

test-a-1  depth=2  children=0 memberships=0 roles=0 role_perms=0 member_roles=0 audit=0
test-a    depth=1  children=1 memberships=0 roles=0 role_perms=0 member_roles=0 audit=0
```

先删 `test-a-1`，再删 `test-a`。

### 决策 6.5：reparent 必须递归重算后代 depth

审查发现 `iam.organization_tree_guard()`（`00010:30-58`）只对被 UPDATE 的那一行设置 `NEW.depth`，**不更新后代**。把 depth=1 的组织改挂到 root 下，它的子节点仍存着 depth=2（实际应为 1），导致：

- `read_queries.go:57` / `organizations.go:57` 的 `ORDER BY depth` 排序错乱；
- `organizations.go:54` 的面包屑 `string_agg(name, ' / ' ORDER BY depth)` 顺序错乱；
- 未来 reparent 可能撞上 `CHECK(depth BETWEEN 0 AND 2)`。

reparent 必须在同一触发器内用递归 CTE 重算整棵子树，并在超深时整体回滚。这是第 6 组的强制要求，不是可选项。

### 决策 7：界面

| 项 | 决定 |
|---|---|
| 标签顺序 | 当前组织 → 全部组织 → 组织成员 |
| 当前组织页 | 只读，改名移到全部组织 |
| 层级展示 | 已有 `OrganizationTree` 替换面包屑 |
| 父组织选择 | 树形下拉，**任意层级可选**（上限已取消），超建议深度时提示 |
| 树数据截断 | `OrganizationTree` 返回 `total` + `truncated`，超限显式提示，禁止静默截断 |
| `Navigation.users` 文案 | 「平台成员」（它管平台账号，不是当前组织成员） |

关于「账号→成员」：直接叫「成员」会与「组织成员」撞名。该页 scope 是 `platform`，叫「平台成员」才准确。角色页本身无「账号」字样（全部走 `t()`，值都是「成员」）。

## 三、影响面

### 后端

| 文件 | 变更 |
|---|---|
| `repository/authorization_queries.go` | `OrgAuthSnapshot` 改递归取祖先链角色并集 |
| `repository/lists.go` | 仅 `ListMembers` 加 `scope`（`ListRoleMembers` 不加，角色只属于一个组织）；两者投影增加 `org_slug`/`source` |
| `service/lists.go` | 透传 scope；子树模式下的授权判定 |
| `handler/management/organizations.go` | 解析 `scope` 查询参数 |
| `repository/roles.go` | 新增 `ProtectOrganizationAdministrators` |
| `service/{organizations,members,accounts,roles}.go` | 管理员检查改为按组织；改名移除检查 |
| `model/http.go` | `APIOrganizationUpdate` 加 `ParentID` |
| 新迁移 `00012` | 取消深度上限 + `organizations_parent_id` 索引 + `OrganizationTree` 加 LIMIT |
| `repository/store.go` | `VerifySchema` 零管理员只记日志，不硬失败 |

### Web

| 文件 | 变更 |
|---|---|
| `spx/iam/navigation.ts` | 分组与顺序 |
| `spx/pages/management.tsx` | 当前组织只读 + 树 |
| `spx/pages/organizations.tsx` | 创建/编辑加父组织选择器 |
| `spx/pages/members.tsx` | 「包含下级组织成员」开关、所属组织列 |
| `spx/pages/roles.tsx` | 角色成员表加来源列 |
| `spx/components/organization-tree.tsx` | 加 `readOnly`、`disabledIDs`、受控选中模式 |
| `spx/api/iam/management.ts` | 接 `GET /organizations/tree`；成员列表加 scope |
| `messages/{en,zh-CN}.json` | 新增键 |

## 行为契约补充

### 成员子树聚合的授权主体

`GET /orgs/{id}/members?scope=subtree` **只校验请求组织自身的 `members.read`**，不逐个校验其后代组织。

理由：继承生效后，对组织 X 有 `members.read` 的调用者，对其全部后代同样有效。这与 GitLab 父 group 管理员语义一致。

**约束**：若将来引入逐组织 deny 通道（当前 `authorization.Evaluate` 无 deny，只有并集），此规则必须重新评审，否则会成为越权枚举入口。

### 递归查询必须用 `UNION`

所有沿 `parent_id` 递归的 CTE（祖先链、子树）一律用 `UNION` 而非 `UNION ALL`。实测 `UNION ALL` 在成环数据上会无限递归至超时；`UNION` 因去重自然终止。组织表规模小，去重开销可忽略。

## 四、验收标准

1. 在 `master` 授权后，子组织成员自动获得同等权限，成员页标注「继承自 AISOC」。
2. 子组织成员不出现在父组织成员列表（默认）。
3. 打开「包含下级组织成员」后，父组织列表含后代成员，且每行显示所属组织。
4. 子组织只能加强角色；移除继承角色被拒绝并给明确错误码。
5. 修改任意组织名称不因其他组织状态失败。
6. 存在零 `org_admin` 组织时服务拒绝启动并指出 slug。
7. `test-a`、`test-a-1` 已删除。
8. 标签顺序：当前组织、全部组织、组织成员、平台成员。
9. 当前组织页无可编辑控件。
10. 当前组织页与全部组织页均树形展示，当前节点高亮。
11. 创建/编辑组织可用树形选择父级，**任意层级均可选**；超过建议深度（5 层）时给提示但不阻断。
12. 平台超管以只读说明行表达。
13. 创建组织可选父级，任意层级；可改父级（环检测）。
14. 父组织管理员可读取子组织成员列表（继承生效的自然结果，有测试锁定）。
15. `OrganizationTree` 超限时返回 `truncated` 标志，前端显式提示，不静默缺节点。

## 五、明确不做

- 不引入外部策略引擎。
- 不做行级/对象级数据权限。
- 不做成员资格向上继承。
- 不把「聚合视图」变成默认。
- 不设数字深度上限（采纳 GitHub/Keycloak/AD/HNC 做法），仅文档建议 ≤5 层。
- 不做继承链可视化（超深时仅提示）。
- 不伪造超管 membership 行。

## 六、风险

1. **递归查询性能**：上限已取消，递归 hop 数不再有界。**必须先补 `organizations_parent_id` 索引**，否则每层 `Seq Scan`（已实测）。需 `EXPLAIN ANALYZE` 确认走索引。
2. **继承是行为破坏**：现有依赖「不继承」的前端假设要一起改。
3. **子树聚合分页**：`count(*) OVER()` 在递归 CTE 上仍可用，但需确认总数口径（聚合后行数）。
4. **递归查询需索引**：`parent_id` 当前无可用索引（`single_root` 是部分索引，谓词为 `parent_id IS NULL`），实测递归会 `Seq Scan on organizations`。必须补 `organizations_parent_id`。
