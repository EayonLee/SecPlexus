# 管理页一致性实施计划

## 任务

| # | 任务 | 文件 | 状态 |
|---|---|---|---|
| 1 | 新增 `PageHeader`（统一标题行和页级操作） | `spx/components/page-header.tsx` | 完成 |
| 2 | 管理页容器去掉 `max-w-6xl` | `spx/pages/management.tsx` | 完成 |
| 3 | 四个管理页改用 `PageHeader` 与 `Navigation.*` 标题 | `spx/pages/{profile,roles,users,audit}.tsx` | 完成 |
| 4 | 删除页面、抽屉与列表中的重复说明 | `spx/pages/{profile,roles,users,audit}.tsx`、`spx/components/forms.tsx` | 完成 |
| 5 | 契约改名 `user_count`/`affected_users` | `spx/api/iam/schemas.ts`、`spx/pages/roles.tsx`、`tests/unit/response-models.test.ts` | 完成 |
| 6 | 文案键改名、新增与清理，双语同步 | `messages/{en,zh-CN}.json` | 完成 |
| 7 | 用户列表按用户聚合（IAM SQL + 类型 + 页面） | `iam/internal/repository/lists.go`、`service/lists.go`、`handler/management/users.go`、`spx/api/iam/schemas.ts`、`spx/pages/users.tsx` | 完成 |
| 8 | `FormDialog` 增加 `variant="sheet"` 与 `confirmVariant` | `spx/components/forms.tsx` | 完成 |
| 9 | 角色抽屉按 Products 尺寸、权限按资源分组、编辑底栏固定 | `spx/pages/roles.tsx` | 完成 |
| 10 | 工具条抽取为 `Toolbar`（查询在左、操作靠右），四页统一 h-8 / size=sm | `spx/components/toolbar.tsx`、`spx/pages/{roles,users,audit,profile}.tsx` | 完成 |
| 11 | Sheet 底层读取宽度令牌，业务抽屉复用 `SheetPanel` 的头部、滚动区、底栏和焦点恢复 | `spx/components/sheet-panel.tsx`、`spx/components/forms.tsx` | 完成 |
| 12 | 表格列全部左对齐 | `spx/components/data.tsx`、`spx/pages/{roles,users,audit}.tsx` | 完成 |
| 13 | 验证 | 见下 | 完成 |

## 验证证据

| 检查 | 命令 | 结果 |
|---|---|---|
| Web 类型检查 | `pnpm --dir src/apps/web typecheck` | 通过 |
| Web Lint | `pnpm --dir src/apps/web lint` | 通过（无错误） |
| Web 单测 | `pnpm --dir src/apps/web test:unit` | 通过 |
| Web 构建 | `pnpm --dir src/apps/web build` | 通过 |
| Go 构建 | `go build ./src/...` | 通过 |
| Go 测试 | `go test -count=1 ./src/apps/iam/...` | 通过（config 需在无 `SPX_*` 环境下运行，否则默认值断言失败） |
| 浏览器几何实测 | Playwright + 本机 dev 栈 | 页首操作与标题成组，筛选与表格共享边框；工作台和六个菜单页面的一级标题均为 24px、600 字重、32px 行高；390px 视口中页面无横向溢出 |

## 展示约定

- 用户身份和操作列在窄列表中保留。角色和其他次要列按列表容器宽度收起，记录详情提供完整字段。

## 状态与抽屉回归

| 检查视角 | 场景与验收 |
| --- | --- |
| Persuade · 首次使用 | 空的接入点列表为有写权限的用户提供三个协议入口，只读用户看到当前状态。 |
| Operate · 完成任务 | 初次请求失败保留导航和工具条；可以重试原查询。清除筛选恢复记录。刷新失败保留可读数据和编辑草稿。 |
| Read · 理解结果 | 接入点、映射、用户、角色、审计和工作台复用状态层级，标题、图标、操作按照相同顺序排列。 |
| Experience · 编辑与返回 | 在宽屏检查字段和页脚的对齐、窄屏检查纵向滚动；从触发按钮打开，Escape 关闭并恢复键盘焦点。 |

系统设置抽屉使用 Products 32rem 最大宽度。验收覆盖 1440、820、390px 视口的明暗主题、页首、列表可达性及焦点恢复。列表失败后重试同一查询。

## 模板基线与防漂移

以工作台为标题基准，从 Roles 和 Products 复用列表布局、从 Profile 复用身份信息布局。页级操作放在标题旁，筛选与结果共享内容区。Biome 在页面层限制独立一级标题及绕过抽屉或集合状态的导入，已有控制台布局 E2E 同时比较各菜单及创建页的实际标题样式。业务参考用于验证流程，模板目录和组件对应关系见 Web README。


## 系统设置模板对齐

1. 统一入口 `/settings/profile`、`/settings/roles`、`/settings/users`、`/settings/audit`。为旧 `/settings`、`/profile` 增加精确重定向。头像菜单、侧栏分组和面包屑父级使用「系统设置」。
2. 采用 Profile 的身份区和线形标签、Users 的 Card 页首和身份列、Roles 的分组表格，以及 Tasks 的筛选栏和紧凑事件列表。审计保留游标分页。
3. Products 的 32rem 右侧抽屉用于用户和角色。角色详情统一展示权限，编辑资料和权限独立保存。短表单及危险操作分别使用普通弹框和 shadcn Alert Dialog。
4. 角色服务端按名称进行字面量模糊搜索，沿用 IAM 鉴权、总数和分页约定。通过真实 HTTP 和 PostgreSQL 验证有匹配、无匹配、通配符及拒绝访问。
5. 在隔离环境回归时区、改密、用户及角色管理、只读权限和删除限制，检查桌面和窄屏的明暗主题及键盘操作。

### 源码对照与补齐

1. 移植模板 Users/Roles 的表格、选择、显示控制和筛选结构，分页统一到解析规则使用的共享组件，按 IAM 模型替换演示字段。
2. 使用全局主题令牌统一按钮、选择、焦点、应用底色、卡片、侧栏及覆盖层，核验实测颜色和层级。
3. 使用已有数据库时间投影、服务端筛选、权限目录和资料设置实现缺失的轻量交互。
4. 同一视口和主题逐页对照，并打开分页、筛选、列表菜单、详情和表单。通过隔离环境验证写入与访问边界。
