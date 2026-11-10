# 管理页一致性实施计划

## 任务

| # | 任务 | 文件 | 状态 |
|---|---|---|---|
| 1 | 新增 `PageHeader`（统一标题行；页面主操作不放这里，走工具条） | `spx/components/page-header.tsx` | 完成 |
| 2 | 管理页容器去掉 `max-w-6xl` | `spx/pages/management.tsx` | 完成 |
| 3 | 四个管理页改用 `PageHeader` 与 `Navigation.*` 标题 | `spx/pages/{settings,roles,users,audit}.tsx` | 完成 |
| 4 | 删除页面、抽屉与列表中的重复说明 | `spx/pages/{settings,roles,users,audit}.tsx`、`spx/components/forms.tsx` | 完成 |
| 5 | 契约改名 `user_count`/`affected_users` | `spx/api/iam/schemas.ts`、`spx/pages/roles.tsx`、`tests/unit/response-models.test.ts` | 完成 |
| 6 | 文案键改名、新增与清理，双语同步 | `messages/{en,zh-CN}.json` | 完成 |
| 7 | 用户列表按用户聚合（IAM SQL + 类型 + 页面） | `iam/internal/repository/lists.go`、`service/lists.go`、`handler/management/users.go`、`spx/api/iam/schemas.ts`、`spx/pages/users.tsx` | 完成 |
| 8 | `FormDialog` 增加 `variant="sheet"` 与 `confirmVariant` | `spx/components/forms.tsx` | 完成 |
| 9 | 角色抽屉加宽、两列权限、移除「角色用户」、底栏固定 | `spx/pages/roles.tsx` | 完成 |
| 10 | 工具条抽取为 `Toolbar`（查询在左、操作靠右），四页统一 h-8 / size=sm | `spx/components/toolbar.tsx`、`spx/pages/{roles,users,audit,settings}.tsx` | 完成 |
| 11 | 抽取 `SheetPanel` 作为右侧抽屉唯一实现：`components/ui/sheet.tsx` 默认类含 `data-[side=right]:sm:max-w-sm`（特异性 0,2,0），页面的 `sm:max-w-*`（0,1,0）压不过它，抽屉曾被压回 384px；统一后页面不再指定宽度 | `spx/components/sheet-panel.tsx`、`spx/components/forms.tsx` | 完成 |
| 12 | 表格列全部左对齐 | `spx/components/data.tsx`、`spx/pages/{roles,users,audit}.tsx` | 完成 |
| 13 | 验证 | 见下 | 完成 |

## 验证证据

| 检查 | 命令 | 结果 |
|---|---|---|
| Web 类型检查 | `pnpm --dir src/apps/web typecheck` | 通过 |
| Web Lint | `pnpm --dir src/apps/web lint` | 通过（0 error；余下 3 warning / 3 info 均为改动前既有项：模板页 Tailwind 类排序、`data.tsx` 嵌套三元、`layout-controls` 与 `shell` 的未用符号） |
| Web 单测 | `pnpm --dir src/apps/web test:unit` | 通过 |
| Web 构建 | `pnpm --dir src/apps/web build` | 通过 |
| Go 构建 | `go build ./src/...` | 通过 |
| Go 测试 | `go test -count=1 ./src/apps/iam/...` | 通过（config 需在无 `SPX_*` 环境下运行，否则默认值断言失败） |
| 浏览器几何实测 | Playwright + 本机 dev 栈 | 标题下 16px 即内容；工具条与内容顶对齐（差 0px）；主按钮右边缘与表格右边缘差 1px；按钮与搜索框同一行（中心线差 ≤2px）；抽屉宽度由 `SheetPanel` 统一，抽屉与页面均无横向滚动 |

## 未覆盖

- 未做人工视觉验收：坐标级实测通过，观感仍需人工确认。
- 用户角色超过 3 个时列表只显示前 3 个徽标与 `+N`，全部角色在行内抽屉查看。
