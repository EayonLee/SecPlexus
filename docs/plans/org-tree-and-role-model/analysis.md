# 组织与角色模型：问题诊断与设计分析

状态：分析完成，未改代码。本文解释五个已报缺陷的真实成因，并对「层级组织中角色与成员应该怎样关联」给出行业依据与建议方案。结论需要确认后才进入实施。

诊断基于当前 `HEAD` 源码与本地已连接数据库（`iam.organizations` 实际三行数据）实测，不是推演。

## 一、五个已报缺陷的成因

### 1. 改名提示「数据已变化或仍被使用，请刷新后重试」

**结论：与重名无关。是「至少一个有效管理员」不变量被触发，而且这个检查被错误地挂在组织改名上。**

证据链：

- 前端 `PATCH /api/v1/iam/orgs/{id}` 携带 `authz_epoch`（`src/apps/web/src/spx/pages/management.tsx:174`）。
- 后端 `UpdateOrganization` 在事务内先校验版本，再改名字，**最后无条件调用 `ProtectLastAdministrators`**（`src/apps/iam/internal/service/organizations.go:100-107`）。
- `ProtectLastAdministrators`（`src/apps/iam/internal/repository/roles.go:149`）的语义是：**任一组织没有 active 的 `org_admin` 即判定失败**，返回 `ErrConflict`。
- `ErrConflict` 在所有 handler 里统一映射为 409「数据已变化或仍被使用」（`src/apps/iam/internal/handler/server.go:92`，文案在 `src/packages/go/result/error-codes.json:41`）。

实测当前库：

```
slug=master    name=AISOC   members=1 active_org_admins=1
slug=test-a    name=新名字   members=0 active_org_admins=0   <-- 触发点
slug=test-a-1  name=测试A1   members=0 active_org_admins=0   <-- 触发点
```

把 `ProtectLastAdministrators` 里那条 SQL 原样跑一遍：

```
ProtectLastAdministrators => invalid=true  (true => ErrConflict => HTTP 409)
```

所以：**只要库里存在任何一个没有 org_admin 的组织，任何一次组织改名都会 409**——即使用户只是把名字从「新名字」改成「test-a」。

用户说的「没有叫 test-a 的组织名称」这一点是对的：`organizations.name` 根本没有唯一约束（`00001_identity.sql` 只对 `slug` 有 UNIQUE）。名字和 slug 是两回事，`slug=test-a` 而 `name=新名字` 完全合法。

**为什么这两个组织没有管理员？** 它们是 11-08 15:26:05 同一秒创建的，`roles=0`、`members=0`，且没有任何 `iam.*organizations*` 审计事件。正常的 `CreateOrganization` 一定会写内置角色和管理员成员关系并留审计（`repository/store.go:225 CreateOrganizationRoles`）。所以这两行不是走 API 建出来的，是直接写库或早期脚本遗留。**这暴露了第二个真实缺陷：数据库层允许存在零角色、零成员的组织。**

### 2. 组织成员列表是空的

**结论：成员列表严格按当前组织过滤，不包含祖先。这是当前设计的既定行为，不是 bug；但「超管不显示」这一点是设计缺口。**

- `ListMembers` 查询写死 `WHERE m.org_id=$1`（`src/apps/iam/internal/repository/lists.go:89`），只列本组织成员。
- 前端 `MembersPage` 用 `auth.org_id` 取数（`src/apps/web/src/spx/pages/members.tsx:42`）。
- 超管 `admin` 只有一条 `membership`，在 `master` 组织下。因此在 `test-a` 的成员页里看不到他——**因为超管的权限来自 `users.is_super_admin` 标记，不来自 membership**（`authorization/policy.go:55`：`if !s.Principal.IsSuperAdmin && (!s.MembershipActive || ...)`）。

所以「超级管理员只属于 1 级，不属于下级吗」——**是的，当前就是只属于 master。超管能操作下级，但在下级的成员列表里不出现。** 这与 GitHub/GitLab/Keycloak 的做法一致（见第三节），但**没有在 UI 上表达出来**，所以看起来像数据丢失。

真正该修的是：组织成员页应该明确显示「平台管理员（隐式，不占席位）」这一类，而不是让人以为列表坏了。

### 3. 当前组织页展示面包屑，应该用树

**结论：树组件已经存在，只是没有用在这个页面。**

- 面包屑：`src/apps/web/src/spx/components/organization-path.tsx`（`OrganizationPath`），用在 `management.tsx:106`。
- 树：`src/apps/web/src/spx/components/organization-tree.tsx`（`OrganizationTree`，带展开/折叠、祖先链自动展开、键盘可达），**目前唯一消费者是顶部组织切换器** `shell.tsx:172`。
- 后端已有 `GET /api/v1/iam/organizations/tree`（`router/routes.go:30`），但**前端没有任何地方调用它**（`management.ts` 里没有 tree 的 API 函数）。

所以这不是「需要新造」，而是「已有组件没接进来 + tree 接口没接前端」。

### 4. 标签顺序应该是：当前组织、全部组织、组织成员

**结论：顺序由 `managementPages` 数组决定，与分组交错有关。**

`src/apps/web/src/spx/iam/navigation.ts` 里 `organization` 分组的实际出现顺序是：

| 顺序 | id | 标题 |
|---|---|---|
| 1 | `members` | 组织成员 |
| 2 | `overview` | 当前组织 |
| 3 | `organizations` | 全部组织 |

`management.tsx:59` 用 `filter(entry.group === page.group)` 保序渲染，所以用户看到的是「组织成员、当前组织、全部组织」。要达到「当前组织、全部组织、组织成员」，改数组顺序即可，不涉及组件结构。

另外注意：`roles` 被放在 `people` 分组（与「账号」同组），`members` 被放在 `organization` 分组。这个分组本身是可疑的——「角色与权限」里同时有平台账号和组织角色，跨越了两个 scope。

### 5. 角色与权限里叫「账号」，应该叫「成员」

**结论：这一条需要先澄清指代，因为角色页自己并没有写「账号」。**

`roles.tsx` 全部文案走 `t(...)`，用到的键没有一个解析成「账号」：`memberCount`=成员数、`member`=成员、`membersTab`=成员（N）。角色页是干净的。

真正带「账号」的是 **`Navigation.users` = 「账号」**（`zh-CN.json:68`），即「角色与权限」分组下的那个平台账号页。它与 `Navigation.people` = 「角色与权限」组成一组，所以用户看到的是：

```
角色与权限
  ├─ 角色
  └─ 账号        <-- 用户认为这里该叫「成员」
```

所以用户的诉求实质是：**「角色与权限」组下面的第二个页签不该叫「账号」**。但注意：这个页面管的是**平台账号**（创建用户、重置密码、启用停用），它的 scope 是 `platform`，与当前组织的成员是两个不同概念。直接改叫「成员」会和「组织成员」撞名，反而更混乱。

这一条需要用户裁决：是改名，还是把这个页签从「角色与权限」挪走（见第五节建议）。

## 二、为什么「下级组织没有上级的角色」

这是当前实现的真实语义，不是渲染问题。

**数据模型现状：**

```
iam.roles(org_id NOT NULL)                 -- 每个角色只属于一个组织
iam.memberships(org_id, user_id)           -- 成员关系按组织隔离
iam.membership_roles(org_id, membership_id, role_id)  -- 角色绑定按组织隔离
```

三重外键都是 `(org_id, ...)` 复合键，从 schema 层就禁止跨组织引用。

**授权求值现状：**

`OrgAuthSnapshot` 只查 `WHERE m.org_id = o.id`（`repository/authorization_queries.go:55-70`），即**只算当前组织自己的成员关系和角色权限**，不向上递归。

**可见性现状：**

`VisibleOrganizations`（`repository/read_queries.go:44`）的注释写得很明确：

> 普通成员可见：自己所属组织，加上这些组织的祖先链（用于面包屑定位）；不含兄弟节点与其后代。**父组织权限不会继承给成员。**

所以在 `test-a` 里看不到 `master` 的角色，是**被刻意设计成不继承**的。

**但这个决定从未在 spec 里被论证过。** `docs/plans/org-tree-and-role-model/spec.md` 的「组织树」章节只规定了深度、环检测、删除约束，**对「父子之间权限是否继承」一字未提**。这是设计缺口，也是用户「搞得很迷糊」的根源——因为没人知道正确答案是什么。

## 三、行业怎么做（调研结论）

官方文档依据，不是推测：

| 系统 | 祖先授权的角色是否向下继承 | 子节点成员是否算父节点成员 | 能否在子节点降低权限 |
|---|---|---|---|
| **GitLab** | **是**。加到 group 的成员自动进入所有 subgroup 和 project | **否**（但能看到父级名称/层级） | 不能；只能加更高角色覆盖 |
| **GitHub** | **是**。子 team 继承父 team 的仓库权限 | **否**（明确写「child team 成员不是 parent team 的直接成员」） | 不能 |
| **Keycloak** | **是**。挂在父 group 的角色映射被所有子 group 成员继承 | 语义上是（子组成员「自动属于」父组） | — |
| **OpenFGA / SpiceDB** | **需显式建模**。`parent` 元组本身不产生继承，必须写 `editor from parent` / `parent->read` | — | — |
| **平台超管** | GitLab 是 user type；GitHub 是 org owner（隐式拥有所有仓库 admin）；Keycloak 是 master realm 的 `admin` 角色 | **都不作为普通成员行出现在每个组织里** | — |

**行业共识（四条）：**

1. **祖先授予的角色向下级联，是默认做法**（GitLab/GitHub/Keycloak 一致）。
2. **最高角色生效**，可以在子节点加更强的角色，但不能削弱继承来的。
3. **必须在 UI 暴露「直接 / 继承」来源**。GitLab 成员表有 `Source` 列（Direct / Inherited from X），Keycloak 角色映射有 `Inherited: true` 标记。
4. **平台超管是特殊身份，不是合成出来的成员行**。授权隐式生效，但成员列表里不伪造 membership。

**与我们现状对比：**

| 维度 | 我们的实现 | 行业默认 | 差距 |
|---|---|---|---|
| 祖先角色向下继承 | ❌ 不继承 | ✅ 继承 | **相反** |
| 子节点成员算不算父节点成员 | ❌ 不算 | ❌ 不算 | 一致 |
| 子节点能否降低权限 | 不适用（无继承） | ❌ 不能 | — |
| 直接/继承来源可视化 | ❌ 无 | ✅ 有 | **缺失** |
| 超管是否出现在成员列表 | ❌ 不出现 | ❌ 不出现（但 UI 会表达） | 一致，但**未表达** |
| 层级展示方式 | 面包屑 | 树 | 不一致 |

## 四、被遗漏的其它问题（用户未提及）

按严重程度排序：

1. **数据库允许创建「零角色、零成员」的组织**。`CreateOrganization` API 路径会补齐，但直接写库或任何绕过路径都能造出这种组织，且一旦存在就**全局锁死所有组织管理操作**（因为 `ProtectLastAdministrators` 是全局扫描）。这是本次 409 的放大器。

2. **`ProtectLastAdministrators` 挂在组织改名上，语义错位**。改名不改变任何成员关系，却因为别处的不变量失败而报「数据已变化」。这是错误的设计耦合。

3. **`ProtectLastAdministrators` 是全局事务级检查，跨组织耦合**。B 组织缺管理员，会导致 A 组织改名失败。应改为「只检查本次操作涉及的组织」。

4. **`CreateOrganization` 不会继承父组织的角色**。新组织只建 `org_admin` + `member` 两个系统角色（`store.go:225`），没有任何来自父级的角色可见性。

5. **创建组织对话框没有 `parent_id` 字段**。`zCreateOrganizationBody` 有 `parent_id`（schemas.ts:226），但 `defaultValues` 只有 name/slug/admin_user_id，UI 里 `parent_id` 出现 0 次。所以**所有通过 UI 建的组织都挂在 default 分支上**——后端 `CreateOrganization` 在 `ParentID` 为空时挂到 root（`service/organizations.go:59`）。这与用户看到的两级结构吻合，但也意味着**用户无法通过 UI 建孙组织**，只有直接调 API 才行。

6. **`test-a-1` 是 `test-a` 的子组织，但两个组织都没有角色/成员**。层级建起来了，内容全空。

7. **`roles` 属于 `people` 分组，`members` 属于 `organization` 分组**。分组划分把「给成员分配角色」的入口（成员页）和「角色定义」的入口（角色页）拆到了两个导航分组里，割裂了概念。

8. **`APIOrganizationUpdate` 不支持 `parent_id`**。DTO 只有 `authz_epoch` + `name`（`model/http.go:93`）。spec 里写「不做组织 reparent 的完整 UI」，但**连 API 都没有**，所以父级关系一旦建错就无法修正，只能删了重建（而有子组织又删不掉 → 死锁）。

9. **`OrganizationTree` 组件同时被 `shell.tsx` 当切换器用**。如果要在管理页复用，需要区分「可选中跳转」和「只读展示」两种模式。

10. **角色成员表 `ListRoleMembers` 也只按 `org_id` 过滤**（`lists.go:101`）。一旦决定继承，这里会漏掉继承来的成员。

## 五、建议方案（需要确认）

### 方案 A：改成行业默认（祖先角色向下继承）

- 授权求值时沿 `parent_id` 递归，取祖先链上的角色集合与当前组织角色取并集。
- 成员列表增加 `source` 字段（`direct` / `inherited_from: <org>`），UI 加来源列。
- 角色成员表同样合并继承成员。
- 子节点只能加更高权限，不能削弱。
- **代价**：授权 SQL 要改成递归 CTE；所有「按 org 过滤」的列表查询都要重新审视；`VisibleOrganizations` 也要跟着改。

### 方案 B：保持不继承，但把语义说清楚

- 在 UI 上明确写「下级组织不继承上级角色」，并在成员页用「平台管理员」徽章标注超管。
- 补上「直接 / 继承」的概念空缺——直接说明「本组织独立」。
- **代价**：与行业默认相反，用户每次跨组织操作都要重新分配角色；组织的树形就只剩「分类」作用，没有「授权作用」。

### 方案 C（推荐）：树只做组织归类，授权显式跨组织授予

- 保留「角色属于单个组织」的现状，不搞隐式继承。
- 新增「跨组织成员关系」：允许同一个 membership 在多个组织下有角色，或允许把某个组织的角色「共享」给子组织（显式操作，不是自动继承）。
- 这样既不违反最小权限，又解决了「下级没有上级角色」的实际需求，而且**行为可预测**（用户看得见共享关系）。

我倾向 C 或 A，**不建议 B**——B 会让树形变成纯装饰，用户会更迷糊。

无论选哪个，以下四项是**独立于方案选择、必须修的**：

1. `ProtectLastAdministrators` 从「组织改名」中移除，或收窄为「只检查受影响组织」。
2. 数据库层阻止「零角色/零成员」组织（迁移期约束 + 启动自检）。
3. 修 `test-a`/`test-a-1` 这类脏数据（补管理员或删除）。
4. 创建组织 UI 补 `parent_id` 选择器；`APIOrganizationUpdate` 补 `parent_id`（reparent）。
5. 标签顺序、当前组织只读、面包屑换树、`Navigation.users` 命名——这四条是纯 UI，与模型无关，可以立即做。

## 六、需要用户决策的问题

1. 角色继承：选 A / B / C？
2. 「角色与权限 → 账号」这个页签，是改叫「成员」，还是移出该分组？
3. `test-a` / `test-a-1` 这两条脏数据，是补管理员还是删掉？
4. 是否允许 UI 建孙组织（需要 parent 选择器）？
5. 是否允许 reparent（改父级）？

## 七、附：验证方法

本文所有结论均可在本地复现：

```sh
# 1. 确认 409 的真实原因（不是重名）
#    用 .env 里的 DSN 跑 ProtectLastAdministrators 的原查询，看 invalid 是否为 true

# 2. 确认 name 无唯一约束
grep -n "UNIQUE" src/apps/iam/migrations/00001_identity.sql
#    -> 只有 slug 有 UNIQUE

# 3. 确认成员列表按 org 隔离
grep -n "WHERE m.org_id=\$1" src/apps/iam/internal/repository/lists.go

# 4. 确认树接口已有但前端未用
grep -rn "organizations/tree" src/apps/iam/internal/router/routes.go src/apps/web/src/
```
