# 实施计划：组织授权继承与成员视图修正（v2，已按缺陷审查修正）

对应 [spec.md](spec.md)。**v2 修正了 v1 的 7 个缺陷**，详见 [review-findings.md](review-findings.md)。

按组执行，每组独立可验收；前一组判据未满足不进入下一组。

## 相对 v1 的关键修正

| 缺陷 | v1 做法 | v2 做法 |
|---|---|---|
| 1 删除锁死 | 加 DELETE 触发器 + 保留「零成员才能删」 | **删掉触发器与前置条件**；不变量放应用层，只挡成员移除 |
| 2 depth 腐坏 | reparent 后「触发器重算」 | 触发器内**递归重算整棵子树** depth |
| 3 全表扫描 | 未提 | 补 `organizations_parent_id` 索引 |
| 4 角色成员 scope | 加 subtree | **移除**；角色只属于一个组织 |
| 5 重复行 | 未提 | 子树视图**不去重**，每行带 `org_slug` |
| 6 授权放行 | 只改语义 | 补**三条负向测试**（兄弟/子孙不得越权） |
| 7 伪需求 | 实现「不能削弱」 | **删除**该任务（无 deny 通道） |

## 依赖关系

```
第 1 组（脏数据 + 索引 + 不变量归位）
    ↓
第 2 组（解耦改名路径）
    ↓
第 3 组（授权继承：后端）
    ↓
第 4 组（成员子树聚合）
    ↓
第 5 组（界面，纯前端，可与 3、4 并行）
    ↓
第 6 组（父选择器 + reparent 递归重算）
```

第 1、2 组是止血。

---

## 第 1 组：脏数据、索引与不变量归位

### 1.1 先写失败测试

`internal/repository/administrator_integration_test.go`：

- `TestRemovingLastOrganizationAdministratorIsRejected`：移除某组织唯一 active `org_admin` → 拒绝（服务层）。
- `TestDeletingOrganizationWithMembersSucceeds`：组织有成员时仍可删除，且成员/角色/授权同事务清除。
- `TestDeletingOrganizationDoesNotTripAdministratorInvariant`：删除整个组织不会因不变量失败。

第三条是防回归的核心——v1 的设计会让它永远失败。

另需改 `organization_tree_integration_test.go:56` 的 `TestOrganizationTreeEnforcesDepthLimit`：原断言「第 4 层被拒」随上限取消一并失效，改为 `TestOrganizationTreeAllowsDeepChains`（建 5 层成功）与 `TestOrganizationTreeRejectsCycle`（环仍拒绝）。

### 1.2 清理脏数据

已复核：`test-a-1`（depth 2）、`test-a`（depth 1）的 children/members/roles/role_perms/member_roles/audit 全为 0（`test-a` 有 1 个 child 即 `test-a-1`）。

```sql
DELETE FROM iam.organizations WHERE slug = 'test-a-1';
DELETE FROM iam.organizations WHERE slug = 'test-a';
```

执行前重跑一次依赖计数查询确认，不盲删。

### 1.3 实现

1. 新迁移 `00012_organization_tree_relax.sql`（**合并原计划的 00012 与 00013**，避免两次重建同一触发器函数导致功能丢失，见缺陷 11）：
   - 重建 `organization_tree_guard()`（`CREATE OR REPLACE`，可在线执行）：**只去掉 `IF NEW.depth > 2`**，保留 `NEW.depth := parent_depth + 1` 与环检测。**不在此加递归重算**——reparent 的递归重算在 Go 服务层完成（见第 6 组，避免触发器自激）。
   - `ALTER TABLE ... DROP CONSTRAINT organizations_depth_check`，改为宽松兜底 `CHECK(depth BETWEEN 0 AND 1000)`（缺陷 10：`depth` 是 `smallint`，取消约束后需防溢出）；
   - `CREATE INDEX organizations_parent_id ON iam.organizations(parent_id);`
   - 更新列注释。
   - `OrganizationTree` 查询（`organizations.go:46-57`）是**扁平列表**（每组织一行，前端 `buildTree` 组装），加 `LIMIT` 不等于「限制树高」。改为：按 `depth, created_at, id` 排序后 `LIMIT`，**响应携带 `total` 与 `truncated` 标志**，前端显式提示，禁止静默截断（缺陷 13）。
   - 不建 DELETE 触发器（缺陷 1）。
   - **`Down` 必须先检测是否存在 depth>2 的组织**，有则拒绝回滚并给出清晰错误（缺陷 12）。
2. `repository/roles.go`：
   - `ProtectLastAdministrators` 收窄为**只查全局超管**。
   - 新增 `ProtectOrganizationAdministrators(ctx, tx, orgID)`。
3. `service/members.go` / `roles.go` / `accounts.go`：改为调按组织检查。
4. `service/organizations.go`：
   - `UpdateOrganization` **移除**管理员检查。
   - `DeleteOrganization` **移除** `members == 0` 前置条件，保留 `isRoot` 与 `children > 0` 检查（有子组织仍拒绝，与 GitLab/Keycloak 的「先删子树」一致）。
5. `repository/store.go` `VerifySchema`：零管理员组织改为**记录 error 日志并继续**，不硬失败。
   - 理由：`VerifySchema` 只在 `serve.go:44` 调用，硬失败会让历史脏数据锁死服务，且没有修复入口。修复动作放到 `maintain`。

### 1.4 判据

- 三个新测试通过。
- `master` 改名返回 204。
- 新建组织 → 加成员 → 删除组织，全链路成功。
- **可建 5 层深链**；环仍被拒；`EXPLAIN` 递归走 `organizations_parent_id` 索引。
- `pnpm test:integration --docker` 全绿。

---

## 第 2 组：解耦改名路径

### 2.1 先写失败测试

`TestOrganizationRenameSucceedsWhenOtherOrganizationLacksAdministrator`：用直接 SQL 造零管理员兄弟组织，改名另一组织 → 204。

### 2.2 实现

`service/organizations.go` 的 `UpdateOrganization` 已不含管理员检查（第 1 组完成）。本组只补测试与确认 `UpdateCurrentOrganization` 路径同样干净。

### 2.3 判据

- 新测试转绿。
- 原「最后一个管理员不可移除」测试仍绿（`authorization_integration_test.go:248`）。

---

## 第 3 组：祖先角色向下继承

### 3.1 先写失败测试（含三条负向）

`repository/organization_tree_integration_test.go`：

- `TestAncestorRoleGrantsReachDescendants`：`master` 授权 → 子组织生效。
- `TestSiblingOrganizationDoesNotInherit`：**兄弟组织不得获得**（防横向泄漏）。
- `TestDescendantDoesNotGrantAncestor`：子孙授权不上溯（防反向泄漏）。
- `TestInactiveAncestorMembershipGrantsNothing`：祖先 membership 为 disabled 时不得生效。

后三条是缺陷 6 要求的负向保护。

另补 `TestAncestorAdminCanReadDescendantMembers`：父组织管理员因继承可读子组织成员列表——锁定这个新行为，防止将来悄悄变化（缺陷 15）。

### 3.2 实现

1. `repository/authorization_queries.go`：`OrgAuthSnapshot` 改递归 CTE。

```sql
WITH RECURSIVE ancestry AS (
  SELECT id, parent_id FROM iam.organizations WHERE id = $1
  UNION            -- 必须 UNION（去重），不能 UNION ALL：脏数据成环时 UNION ALL 会无限递归（已实测超时）
  SELECT p.id, p.parent_id FROM iam.organizations p
  JOIN ancestry a ON p.id = a.parent_id
)
-- 权限取 ancestry 全集成员关系的并集；m.status='active' 才计入
```

子树 CTE 同理一律用 `UNION`，与现有 `VisibleOrganizations`（`read_queries.go:51`）保持一致。

2. `authorization/policy.go`：`MembershipActive` 语义改为「本组织或祖先存在 active membership」。补注释说明这是**放行条件扩大**，属安全敏感变更。
3. `service/service.go:98` 对应调整。

### 3.3 判据

- 四个新测试通过。
- `EXPLAIN (ANALYZE)` 确认递归走 `organizations_parent_id` 索引，无 Seq Scan。
- 现有授权测试全绿。

---

## 第 4 组：成员子树聚合

### 4.1 先写失败测试

- `TestMemberListScopeDirectIsDefault`：不传 scope 等价 direct。
- `TestMemberListScopeSubtreeReturnsDescendants`：含子孙，且每行带 `org_slug`。
- `TestMemberListSubtreeDoesNotDeduplicate`：同一用户在父子各一条 membership → 返回两行。
- `TestMemberListSubtreeRequiresCurrentOrgPermission`：无权者不能借 scope 越权。

### 4.2 实现

1. `repository/lists.go`：
   - **`ListMembers` 加 `scope` 参数**；`scope=subtree` 用递归 CTE 取子树。
   - **`ListRoleMembers` 不加 scope**（缺陷 4）。
   - 两者投影增加 `org_slug` 与 `source`（`direct` / `inherited`）。
2. `service/lists.go`：透传 scope；授权判定**只针对请求组织自身**（`members.read on orgID`），不逐个校验后代。这与 GitLab 的父组管理员语义一致，且依赖继承成立。**若将来引入逐组织 deny 通道，此规则必须重新评审**（缺陷 14）。
3. `handler/management/organizations.go`：解析 `scope`，非法值 `invalid_request`。
4. `model/http.go`：成员投影字段补齐。

`listJSON` 已支持可变参数（`lists.go:14`），无需重构分页。

### 4.3 判据

- 四个新测试通过。
- 子树模式分页与 `total` 口径正确。
- `EXPLAIN` 无 Seq Scan。

---

## 第 5 组：界面（纯前端，可并行）

### 5.1 先写失败测试

- 单测：`managementPages` 顺序为「当前组织、全部组织、组织成员、平台成员」。
- 单测：`OrganizationTree` `readOnly` 不触发 `onSelect`；`disabledIDs` 不可点。

### 5.2 实现

1. `navigation.ts`：`organization` 组顺序 `overview` → `organizations` → `members`；`users` 移入该组，文案「平台成员」。
2. `management.tsx`：`OverviewPage` 去表单改只读；`OrganizationPath` 换 `OrganizationTree`（`readOnly`）。
3. `organizations.tsx`：编辑对话框保留改名（唯一入口）。
4. `organization-tree.tsx`：加 `readOnly`、`disabledIDs`、受控 `selectedID`。
5. `management.ts`：接 `GET /organizations/tree`。
6. `members.tsx`：「包含下级组织成员」开关（接 `scope`）+「所属组织」列。
7. `roles.tsx`：角色成员表加来源列（**不加 scope**）。
8. 消息目录 `en`/`zh-CN` 同步。

### 5.3 判据

- 标签顺序正确。
- 当前组织页无可编辑控件。
- 树可折叠、当前节点高亮。
- 成员页开关切换生效。
- 语言目录键对齐。

---

## 第 6 组：父选择器与 reparent（含递归重算）

### 6.1 先写失败测试

- 集成：创建时指定 `parent_id` 建出下级；可建 5 层。
- 集成：reparent 后**后代 depth 同步更新**（缺陷 2 的回归测试）。
- 集成：reparent 挂到自己后代 → 拒绝（环）。
- 集成：reparent 造成环 → 整体回滚，depth 不变。
- 单测：`disabledIDs` = 自身 + 全部后代（防环）。**深度不再置灰**。

### 6.2 实现

1. 触发器已在第 1 组的 `00012` 中一并重建（含递归重算），本组**不再新建迁移**，只做 DTO 与 service 变更（缺陷 11）。

**递归重算后代 depth 放在 Go 服务层，不放进触发器。**

原因：`organization_tree_guard()` 是 BEFORE ROW 触发器，对它自己发出的递归 `UPDATE` 会再次触发，形成自激。若用 `pg_trigger_depth()` 判断规避，则与本文档缺陷 1 中「禁止依赖 `pg_trigger_depth()`」的理由冲突，且行为难以推理。

正确做法：在 `Manage()` 事务内，改完 `parent_id` 之后由 Go 侧执行一次递归 UPDATE（同一事务，原子）：

```sql
WITH RECURSIVE d AS (
  SELECT id, (SELECT depth FROM iam.organizations WHERE id = $1) AS depth
  FROM iam.organizations WHERE id = $1
  UNION                        -- 去重，防脏数据成环时无限递归
  SELECT o.id, d.depth + 1 FROM iam.organizations o JOIN d ON o.parent_id = d.id
)
UPDATE iam.organizations t SET depth = d.depth
FROM d WHERE t.id = d.id AND t.id <> $1;
```

注意：该 UPDATE 会再次触发 `organization_tree_guard()`，但触发器对每一行只做 `NEW.depth = parent.depth + 1`，与递归算出的值一致，**幂等收敛**，不会自激。实现时必须用一个「reparent 后 depth 全部正确」的集成测试验证这一点。

深度上限已取消，不再校验 `MAX(depth)`；溢出由 `CHECK(depth BETWEEN 0 AND 1000)` 兜底。

2. `model/http.go`：`APIOrganizationUpdate` 加 `ParentID *string`。
3. `service/organizations.go`：`UpdateOrganization` 支持改父级。
4. `organizations.tsx`：创建/编辑加树形父选择器，`disabledIDs` 含自身+后代（防环）；超建议深度仅提示。
5. `schemas.ts`：`zUpdateOrganizationBody` 加可选 `parent_id`。

### 6.3 判据

- 可建 5 层及以上组织（不再有硬上限）。
- reparent 后 tree 接口返回的 depth 全部正确，面包屑顺序正确。
- 环被前後端双侧拦截；超建议深度仅提示。

---

## 验证命令

```sh
pnpm test:unit
pnpm typecheck
make build
pnpm test:integration --docker
pnpm test:e2e --docker
```

E2E 补三条：继承在子组织生效、兄弟组织看不到、成员页子树开关。

## 进度

| 组 | 状态 |
|---|---|
| 1 脏数据 + 索引 + 不变量归位 | 未开始 |
| 2 解耦改名 | 未开始 |
| 3 授权继承 | 未开始 |
| 4 成员子树聚合 | 未开始 |
| 5 界面 | 未开始 |
| 6 父选择器 / reparent | 未开始 |
