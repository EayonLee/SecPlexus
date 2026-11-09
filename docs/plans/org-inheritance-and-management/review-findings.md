# 计划缺陷审查记录

审查对象：[spec.md](spec.md)、[plan.md](plan.md)。审查方式：逐条对着源码、迁移脚本和实际数据库跑验证，不靠推理。本文记录**已确认的缺陷**与**修正后的做法**。

结论先说：原计划有 **7 个真实缺陷**，其中 3 个会导致功能完全不可用或数据损坏。原第 1 组「加触发器」的设计如果照做，会把「删除组织」永久锁死。

---

## 缺陷 1（致命）：触发器 + 「删除需零成员」= 组织永久不可删除

**证据**：`repository.DeleteOrganization` 的删除顺序（`src/apps/iam/internal/repository/organizations.go:106-118`）：

```go
DELETE FROM iam.membership_roles WHERE org_id = $1
DELETE FROM iam.role_permissions WHERE org_id = $1
DELETE FROM iam.roles WHERE org_id = $1
DELETE FROM iam.memberships WHERE org_id = $1
DELETE FROM iam.organizations WHERE id = $1
```

删 `memberships` 时 `organizations` 行**还在**。若按原计划加「每个组织必须 ≥1 active org_admin」的 DELETE 触发器，此刻必然违反 → 抛异常 → 永远删不掉任何组织。

再叠加两处条件：

- `DeleteOrganization` 要求 `members == 0`（`service/organizations.go:147`）。
- 而把成员清到 0 只能靠 `UpdateMember`，它对 `admin && !IsSuperAdmin` 直接 `ErrDenied`（`members.go:138`），且末尾调 `ProtectLastAdministrators`。

**三条约束一起构成闭环死锁**：组织永远删不掉。

**调研结论**（GitLab / GitHub / Keycloak 官方文档 + 源码）：

- GitLab：删除 group **不需要**清空成员。`Groups::DestroyService` 只校验 `can?(:remove_group)`，从不查 last-owner；`dependent: :destroy` 级联删成员。
- GitHub 文档原文：*"If you're the only owner in the organization, you must transfer ownership to another person **or delete your organization**."* —— 删组织就是留给「最后一个管理员」的出口。
- Keycloak：`DELETE /groups/{id}` 无任何成员前置条件。
- **没有一家**用数据库触发器实现「≥1 管理员」不变量；GitLab 是应用层 policy（`GroupMemberPolicy`: `prevent :destroy_group_member` if `last_owner`）。
- PostgreSQL 官方文档明确警告：触发 FK 级联删除的触发器若阻止删除，**会破坏引用完整性**——「It is the trigger programmer's responsibility to avoid that.」

**修正**：

1. **删除组织 = 单一事务级联删除**（现有 `repository.DeleteOrganization` 已正确级联），**移除 `members == 0` 前置条件**。
2. 不变量只在**成员移除 / 降权**路径检查，且**组织本身正在删除时跳过**。
3. **不变量放应用层**（`ProtectOrganizationAdministrators`），与 GitLab 一致。
4. 数据库兜底**可选**，若要做必须用 `CREATE CONSTRAINT TRIGGER ... DEFERRABLE INITIALLY DEFERRED`，且触发器体内先判断父组织是否仍存在：

```sql
-- 仅当组织仍然存在时才校验
IF NOT EXISTS (SELECT 1 FROM iam.organizations WHERE id = OLD.org_id) THEN
  RETURN OLD;
END IF;
```

**禁止**用 `pg_trigger_depth()` 判断「这是级联、跳过检查」——PostgreSQL 作者 Tom Lane 明确说它在 deferred trigger 下不可靠。

---

## 缺陷 2（致命）：reparent 不会重算后代 depth，树会静默腐坏

**证据**：`iam.organization_tree_guard()`（`migrations/00010_organization_tree.sql:30-58`）只对**被 UPDATE 的那一行**设置 `NEW.depth := parent_depth + 1`。它**不会**更新该行的后代。

把 A（depth 1，有两个 depth 2 的子节点）改挂到 root 下 → A 变 depth 0，但它的子节点仍存着 depth 2，实际应为 1。

而 depth 被三处直接依赖：

- `read_queries.go:57` `ORDER BY depth, created_at, id`（组织可见性排序）
- `organizations.go:57` `ORDER BY o.depth`（树的扁平投影顺序）
- `organizations.go:54` `string_agg(name, ' / ' ORDER BY depth)`（面包屑路径拼接）

后果：面包屑顺序错乱、树渲染顺序错乱，且 `organizations_depth_check CHECK(depth BETWEEN 0 AND 2)` 在未来某次 reparent 时可能直接抛错。

**修正**：reparent 必须在同一触发器内**递归重算整棵子树**的 depth，并对新 depth 超限报错：

```sql
-- reparent 时，递归更新所有后代的 depth = 父 depth + 层级偏移
WITH RECURSIVE d AS (
  SELECT id, NEW.depth AS depth FROM iam.organizations WHERE id = NEW.id
  UNION ALL
  SELECT o.id, d.depth + 1 FROM iam.organizations o JOIN d ON o.parent_id = d.id
)
UPDATE iam.organizations t SET depth = d.depth FROM d WHERE t.id = d.id AND t.id <> NEW.id;
```

并在更新后校验 `MAX(depth) <= 2`，超限整体回滚。

同时**加索引**（见缺陷 3）。

---

## 缺陷 3（高）：`parent_id` 没有可用索引，递归查询全表扫描

**证据**：实测 `iam.organizations` 只有三个索引：

```
organizations_pkey          btree (id)
organizations_slug_key      btree (slug)
organizations_single_root   btree ((parent_id IS NULL)) WHERE parent_id IS NULL
```

`single_root` 是**部分索引**，谓词是 `parent_id IS NULL`，对 `WHERE parent_id = $1` 完全不可用。

实测 `EXPLAIN`：

```
->  Recursive Union
      ->  Seq Scan on organizations            <- 起始点
      ->  Hash Join
            ->  Hash
                  ->  Seq Scan on organizations o   <- 每层都全表扫描
```

**修正**：新迁移中补：

```sql
CREATE INDEX organizations_parent_id ON iam.organizations(parent_id);
```

组织表规模小（深度 ≤3），短期不致命，但递归查询是本计划的核心路径，不应以全表扫描实现。

---

## 缺陷 4（高）：原计划第 4 组把 subtree scope 套到「角色成员」上，语义错误

**证据**：`iam.roles.org_id NOT NULL` + `UNIQUE(org_id, code)`（`migrations/00001_identity.sql:35-42`）。**一个角色只属于一个组织**。

因此「角色成员列表」回答的是「谁被直接授予了这个组织的这个角色」。父组织的 org_admin 角色，其成员永远只在父组织内。

给 `ListRoleMembers` 加 `scope=subtree` 是**无意义**的：角色本身不存在于子组织，子孙层级里没有这个角色的任何授予记录。

**修正**：

- `ListMembers`（组织成员）→ 加 `scope=direct|subtree`。
- `ListRoleMembers`（角色成员）→ **不加 scope**，只加 `source` 标注（这条授予是直接授予还是从祖先继承生效）。因为一旦继承生效，一个角色在子组织「实际生效于哪些人」= 该角色所属组织的成员 ∩ 子组织可见范围，这是另一个查询，不是 subtree。

---

## 缺陷 5（中）：同一用户可在父、子组织各有一条 membership，子树聚合会出重复行

**证据**：`memberships` 的唯一约束是：

```
UNIQUE(org_id, user_id)   -- 同组织唯一
UNIQUE(org_id, id)
```

**跨组织不唯一**。同一用户可同时是 `master` 和 `test-a` 的成员。

子树聚合直接 `JOIN memberships` 会让同一用户出现多行。若前端按用户去重，又会丢失「他在两个组织各有不同角色」这一信息。

**修正**：子树视图**不去重**，每行显式带 `org_slug` + 该组织内的 `role_ids`。UI 文案说明「同一成员可能在多个组织出现」。这是 GitLab 的 `Source` 列思路（一个成员多来源就多行）。

---

## 缺陷 6（中）：`MembershipActive` 语义变更会影响授权主路径，原计划未评估

**证据**：`MembershipActive` 只被两处使用：

- `service/service.go:98` 由 `OrgAuthSnapshot` 填充
- `authorization/policy.go:55` `if !s.Principal.IsSuperAdmin && (!s.MembershipActive || ...)`

把语义从「本组织有 active membership」改成「本组织或祖先有」是**授权放行条件的扩大**。这是安全敏感的破坏性变更，原计划只写「语义改为」，没写验证策略。

**修正**：必须补三条负向测试：

1. 祖先有 membership、本组织无 → 允许（新行为）。
2. 兄弟组织有 membership、本组织无 → **拒绝**（不能横向泄漏）。
3. 子孙有 membership、本组织无 → **拒绝**（不向上继承）。

第 2、3 条是防止递归查询写错导致横向越权的关键。

---

## 缺陷 7（中）：「不能削弱继承角色」是伪需求，不需要实现

**证据**：`authorization.Evaluate` 无 deny 通道，权限是**并集**语义（`policy.go:55` 只判断 `slices.Contains`）。

**修正**：从计划里**删掉** `TestInheritedRoleCannotBeWeakened` 与相关实现。子组织无法「削弱」继承权限——它本来就只有 add 能力，没有 subtract。原计划的这条测试会测一个不存在的机制。

---

## 修正后的计划调整

### 组 1 重写（原设计会锁死删除）

**删除**：`members == 0` 前置条件；DELETE 触发器。

**改为**：

1. 新增 `ProtectOrganizationAdministrators(ctx, tx, orgID)`（应用层），只在成员移除/降权/角色分配路径调用。
2. 新增 `organizations_parent_id` 索引。
3. `VerifySchema` 自检**改为警告而非硬失败**——硬失败会让一个历史脏组织把整个服务锁死，而 `serve` 之外没有修复入口（`VerifySchema` 只在 `serve.go:44` 调用）。改为：启动时记录 error 级日志并继续，`maintain` 子命令提供修复动作。
4. 清理脏数据 `test-a-1` / `test-a`（已在 spec 决策 6 复核：依赖全为 0）。

### 组 6 重写（原设计会腐坏树）

**增加**：reparent 触发器内递归重算后代 depth + 超限回滚。

**增加**：`disabledIDs` 计算必须包含「自身 + 全部后代」以防环。深度不再置灰（上限已取消，见缺陷 8）。

### 组 4 修正

- `ListRoleMembers` 移除 scope。
- 子树聚合不去重，每行带 `org_slug`。
- 补三条 `MembershipActive` 负向测试。

### 组 3 修正

删除「不能削弱」相关实现与测试。

---

## 需要复核的假设（未验证，暂不写进计划）

- ~~超过 3 层的用户诉求~~：已按缺陷 8 处置——取消硬上限，改为建议 ≤5 层。
- **`ListQuery.OrgID` 字段**（`model/http.go:200`）当前未被成员列表使用，可能是历史残留；新增 `scope` 时一并确认是否清理。

---

## 缺陷 8（结论性错误）：我此前「生产系统普遍浅而硬性有界」的判断被推翻

**这是本文档最重要的更正。** 原结论只引用了 AWS 5 / GCP 10 / Slack 1，属选择性取证。

补齐 13 个系统后的真实分布：

| 类别 | 数量 | 系统 |
|---|---|---|
| 无数字上限（仅单父 + 禁环） | **8** | GitHub teams、Keycloak groups、AD/LDAP OU、K8s HNC、Jira、ServiceNow、Workday、Salesforce（建议值非强制） |
| 硬性数字上限 | 3 | GCP 10、AWS 5、GitLab 20 |
| 完全扁平 | 2 | Entra ID admin units、Okta |

**多数不设硬上限。** 决定性引证：

- AD/LDAP：*"While there is no technical limit to the number of levels in your OU structure, for manageability we recommend … no more than 10 levels."* —— 官方明确说无技术上限。
- K8s HNC：*"No, HNC does not impose any limitation. We've tested about a hundred."*
- GitHub teams、Keycloak groups：文档通篇无深度数字。

**修正**：取消 3 层硬上限，采纳「不限深度 + 结构性约束（单父、禁环）+ 建议 ≤5 层」。理由：3 层比几乎所有这些系统都严；GitLab 式硬顶 20 需要在六处维护一个与业务无关的常量，收益近乎为零。

**前提护栏**（当前全缺，必须一并补）：

1. `parent_id` 无可用索引（`single_root` 是部分索引，谓词 `parent_id IS NULL`）→ 必须建 `organizations_parent_id`。
2. `OrganizationTree` SQL 无 `LIMIT`（`organizations.go:57`）。
3. `VisibleOrganizations` 的 `LIMIT 500` 会静默截断（`read_queries.go:57`）。
4. 前端递归渲染无深度保护（`organization-tree.tsx:94`）。
5. 环检测每层一次 `SELECT`，依赖上述索引。

**连带影响**：`TestOrganizationTreeEnforcesDepthLimit`（`organization_tree_integration_test.go:56`）断言的「第 4 层被拒」随之上限取消而失效，必须改写为「深链可建」+「环被拒」。

---

# 最终评审（实施前）

二审对象：spec（修正 1a–1e、决策 1–7）+ plan v2。方法：对着源码／迁移／实测库逐项验证。

## 新增缺陷 9（高）：`ancestry` 递归 CTE 用 `UNION ALL` 会因环无限递归

**实测证据**（临时表 A→B→A）：

```
UNION ALL 结果: timeout: context deadline exceeded
UNION     结果: 行数=2 => 终止，安全
```

计划 3.2 写的是 `UNION ALL`。现有 `VisibleOrganizations`（`read_queries.go:51`）用的是 `UNION`——不是风格差异，而是**去重保证在环上终止**。

虽然触发器防环，但：
- 触发器可被超级用户／直接 SQL 绕过；
- 迁移期间（`00010` 重跑）可能瞬时无触发器；
- 现有代码已经用 `UNION`，改成 `UNION ALL` 是**主动降级**。

**修正**：`ancestry` 与子树 CTE 一律用 `UNION`（非 `UNION ALL`）。组织表规模小，去重开销可忽略。

## 新增缺陷 10（中）：`depth` 是 `smallint`，取消 CHECK 后无溢出保护

`00010:7` `ADD COLUMN depth smallint`（上限 32767）。取消 `CHECK(depth BETWEEN 0 AND 2)` 后，理论上深链可溢出到 `smallint` 越界（不现实，但无保护）。

**修正**：新 CHECK 保留一个宽松上界，例如 `CHECK(depth BETWEEN 0 AND 1000)`。这既满足「不设业务上限」，又保留数据库层的溢出兜底。1000 远高于任何真实使用（最深的 AD 建议 10，GitLab 硬顶 20）。

## 新增缺陷 11（中）：两次迁移重建同一个触发器函数，易丢失

计划里 `00012` 取消上限重建 `organization_tree_guard()`，`00013` 又重建一次以加 reparent 递归。**两次完整复制同一段 plpgsql**，谁先谁后写错就丢功能。

**修正**：`organization_tree_guard()` 只在**一个**迁移里重建一次（`00012`），同时包含「取消上限 + reparent 递归重算后代」。`00013` 只做 reparent 的 API/DTO 变更，不动触发器。

## 新增缺陷 12（中）：Down 迁移无法回滚

取消上限后若已存在深链，`Down` 重新加 `CHECK(depth BETWEEN 0 AND 2)` 会失败。

**修正**：`00012` 的 `Down` 显式检查——存在 depth>2 的组织则拒绝回滚并给出清晰错误，或直接标注该迁移不可自动回滚（goose 支持，但需在注释中写明运维步骤）。

## 新增缺陷 13（中）：`OrganizationTree` 加 `LIMIT` 会截断子树

`OrganizationTree` 返回的是**扁平组织列表**（`organizations.go:46-57`，每个组织一行，前端 `buildTree` 自行组装）。直接加 `LIMIT` 会随机截断，导致树缺节点。

**修正**：`LIMIT` 语义要正确——它不是「树的前 N 层」，而是「前 N 个组织」。因此：
- 要么按 `depth, created_at, id` 排序后 `LIMIT` 并**在响应里带 `total`/`truncated`**，前端显式提示；
- 要么改为真正的分页接口（与 `ListQuery` 一致）。
- 无论哪种，都**不能静默截断**。

## 新增缺陷 14（低）：subtree 模式下的授权主体未定义

计划 4.2 写「授权判定始终针对当前组织」，但没定义「当前组织」在 subtree 请求里指哪个。

实测结论：`ListMembers(orgID=X, scope=subtree)` 校验 `members.read on X`，然后返回 X 及其全部后代的成员——**不逐个校验后代组织权限**。

这与 GitLab 一致（父 group 管理员因继承可管理子 group），在**有继承的前提下自洽**。但必须写进契约，否则将来若支持「逐组织撤销继承」，这里会成为越权入口。

**修正**：在 spec 行为契约里明确：

> `scope=subtree` 只校验请求组织自身的 `members.read`。返回其全部后代成员，前提是调用者对该组织有直接或继承权限。若将来引入逐组织拒绝（deny）通道，此规则必须重新评审。

## 新增缺陷 15（低）：计划缺「继承影响可见性」的测试

继承落地后，父组织管理员会因祖先权限而对子组织通过 `Authorize`，从而能读子组织的成员列表（即使没有该子组织的直接 membership）。这是继承的自然结果，但**计划里没有对应测试**，行为会悄悄变化。

**修正**：补 `TestAncestorAdminCanReadDescendantMembers`，锁定这个新行为。

## 复核通过的部分（无缺陷）

| 检查项 | 结论 |
|---|---|
| `Manage()` 已取 advisory lock，reparent 与成员变更串行 | 并发安全，无需额外加锁 |
| `CREATE OR REPLACE FUNCTION` 可在线执行，不阻塞在途事务 | 可安全部署 |
| `DROP CONSTRAINT` 的 ACCESS EXCLUSIVE 锁 | 组织表小，可接受 |
| 所有迁移都有 `goose Down` | 格式一致 |
| `testutil.Database` 已隔离独立库 | 新测试可直接复用 |
| 角色成员不加 scope（缺陷 4） | 已确认正确 |
| 删除组织级联顺序（缺陷 1） | 现有实现正确，只需去掉前置条件 |
