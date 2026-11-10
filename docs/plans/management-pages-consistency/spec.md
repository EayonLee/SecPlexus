# 管理页一致性与用户列表去重

## 1. 背景

管理区把用户与角色拆成各自页面后，页面各自拼操作区，留下这些问题：

1. **标题不一致**：个人资料页是 `h1 text-2xl font-semibold`，其余管理页各自渲染 `h2 text-base font-medium`。原先统一渲染 `h1` 的容器在去掉标签页时一并丢掉了。
2. **内容宽度偏好失效**：`management.tsx` 外层写死 `mx-auto max-w-6xl`，把全局「内容布局：居中/全宽」偏好覆盖掉了。个人资料页没有这个限制，所以只有它能响应开关。
3. **重复说明文案**：页面、抽屉和表格上方常驻复述标题与字段含义的说明；列表还显示「名称 / N 项」的灰色摘要行。
4. **契约漂移**：IAM 已把 `members → users` 改名（`user_count`、`affected_users`），Web 仍按 `member_count`、`affected_members` 解析，导致角色列表整块触发 `api.protocol_failed`。
5. **用户列表重复行**：一行曾是一个「用户-关系」，同一个人持有多条关系就在同一屏出现多行相同账号。
6. **抽屉形态不统一**：创建角色是右侧抽屉但只有 `sm:max-w-lg`，权限分组被拉成一条窄列；编辑角色抽屉里还塞了「角色用户」列表，与用户管理重复。
7. **列对齐不一致**：同一张表里部分列居中、部分左对齐，阅读时视线来回跳跃。

## 2. 决策

### 2.1 页面标题统一

- 每个管理页只渲染一个 `h1`，样式取模板基线 `text-3xl tracking-tight`（`arhamkhnz/next-shadcn-admin-dashboard` 的 `dashboard/roles` 用法）。
- 标题文案复用左侧菜单的 `Navigation.*` 键，不再维护 `Management.*Title` 的第二套文案。
- 适用页面：个人资料（`/settings`）、角色与权限（`/settings/roles`）、用户（`/settings/users`）、审计日志（`/settings/audit`）。

### 2.2 查询条件与主操作同一行

- 标题下固定一行工具条：左侧查询/筛选控件（`InputGroup`，`h-8`），右侧主操作与次要操作，主操作在最右且 `size="sm"`。
- 工具条由 `components/toolbar.tsx` 统一渲染：`children` 是查询条件，`actions` 靠右成组。各页不再自行拼 flex，按钮位置与尺寸不会各页漂移。
- 页面主操作不再放在标题右侧，避免同一页出现两个操作区。

### 2.3 宽度交给全局偏好

- `management.tsx` 去掉 `mx-auto max-w-6xl`，与个人资料页统一为 `@container/main flex flex-col gap-4 md:gap-6`。
- 页面自身不再设置 `max-w-*`；「居中/全宽」由此对全部管理页一致生效。

### 2.4 删除重复说明

- 不在页面、抽屉与表格上方常驻解释性文案：标题、字段标签和占位符已表达的内容不再复述。
- 列表不再显示「名称 / N 项」的灰色摘要行：选中对象已由高亮行与页面标题表达。
- 删除说明后首次使用者仍应看出下一步操作与结果；必要的约束与后果提示留在对应控件旁，危险操作（删除、移除）用明确后果的确认框。

### 2.5 Web 契约与 IAM 对齐

- `zApiRole.member_count` → `user_count`；`zApiPreview.affected_members` → `affected_users`。
- 角色页「用户数」列改读 `user_count`；文案键 `memberCount` → `userCount`、`deleteRoleHasMembers` → `deleteRoleHasUsers`。

### 2.6 用户列表按用户聚合

- `GET /api/v1/iam/users` 一行一个用户：身份字段（`id`、`username`、`display_name`、`email`、`status`、`configured`）加该用户直接持有的角色（`roles: [{ id, name }]`）。
- 授权只有「用户 → 角色 → 权限」一条路径，没有中间关系对象，同一用户不会因角色数量重复成多行。
- 列表的 `q` 按用户名前缀或显示名模糊匹配，语义与分页一致。
- 角色的持有者列表 `GET /api/v1/iam/roles/:id/users` 使用同一用户投影，前端用独立模型解析。

### 2.7 抽屉统一

- `forms.tsx` 的 `FormDialog` 增加 `variant="sheet"`，与对话框共用同一套 RHF 草稿、重新认证与提交路径。
- 抽屉骨架固定为「头部（标题/描述，底边框） + 正文（`flex-1 overflow-y-auto overflow-x-hidden`） + 底栏（按钮右对齐，顶边框）」，只允许纵向滚动。
- 宽度只在 `spx/components/sheet-panel.tsx` 的 `SheetPanel` 里定义，页面不再各自指定 `sm:max-w-*`。`components/ui/sheet.tsx` 默认类含 `data-[side=right]:sm:max-w-sm`（属性选择器特异性 0,2,0），页面的 `sm:max-w-4xl`（0,1,0）压不过它，抽屉曾被压回 24rem；`SheetPanel` 必须用同一变体声明宽度：`data-[side=right]:w-full data-[side=right]:max-w-none data-[side=right]:sm:w-[min(72rem,88vw)]`。
- 权限勾选区在宽抽屉里按两列排布，避免单列被拉长。
- 编辑角色抽屉不再展示「角色用户」：用户归属用户管理，避免同一数据两处维护。
- 危险操作（删除角色、删除用户）使用 `confirmVariant="destructive"` 的对话框确认。

### 2.8 列表页布局文法

- 统一为「`PageHeader`（`h1`，无操作） + 工具条（查询条件在左，主操作靠右） + 表格 / 空状态」。
- 所有工具条控件同高：输入框 `h-8`，按钮 `size="sm"`（h-8），主操作在最右。
- 表格列全部左对齐，表头与单元格同向；状态用徽标表达，不靠居中吸引注意。
- 空状态与加载态占据表格区域，不改变工具条位置。

## 3. 验收

1. 四个管理页（个人资料、角色与权限、用户、审计日志）标题字号、字重、位置一致；每页工具条一行，主操作最右。
2. 「居中/全宽」开关对四个管理页都生效。
3. 页面、抽屉与表格不再出现复述标题/字段的常驻说明，也不再出现「名称 / N 项」摘要行。
4. 打开角色与权限页不再出现 `api.protocol_failed`，「用户数」列有值。
5. 用户列表一行一个用户：同时持有多个角色的用户只出现一行，角色以徽标展示。
6. 抽屉宽度一致、正文可纵向滚动、无横向滚动；编辑角色抽屉不含「角色用户」。
7. 表格列全部左对齐，表头与单元格对齐一致。
8. 单测、类型检查、Lint、构建通过。

## 4. 不做

- 不引入新的筛选系统；审计页仍只有操作者、时间、结果三个条件。
- 不改动工作台与登录页。
- 不引入移动端专用布局与对应测试，本轮只面向桌面浏览器。
