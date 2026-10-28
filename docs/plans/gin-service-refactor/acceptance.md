# Gin 全栈重构验收记录

验收日期：2026-10-29。对应 [Spec](spec.md)、[Plan](plan.md) 和[框架能力与版本](framework-capabilities.md#7-实际接入与固定版本)。以下为本地实际执行结果，不代表远程 CI、部署或生产验证。

## 实现与验收对应

| 验收 | 实现及证据 |
| --- | --- |
| A1 / R1 | IAM 公开/内部及 Console 全部由 Gin 注册；`Engine.Routes()` 对照契约。删除 Chi、Authboss 直接依赖及专用桥接/路由表，保留成熟密码、会话、CSRF 组件 |
| A2 / R2 | 官方 Gin/OpenAPI 校验复用验证值，生成参数类型；唯一 JSON、未知字段、媒体类型、32 KiB/凭证 2 KiB（含 chunked）测试；原生 Recovery、404/405/Allow 与已提交响应测试 |
| A3 | 类型化凭证用例、Argon2id、真实 Redis 限流、PG 锁定/审计/凭证版本；跨副本会话、改密撤销、绝对/空闲期限及 SCS 提交失败/异常补偿集成测试 |
| A4 / R5/R8 | 独立内部 TLS 与调用方凭证，生成 Go 客户端；组织隔离、委派、多角色、最后管理员和故障关闭由真实 PG/HTTP/TLS 集成覆盖；浏览器验证撤权 SSE 停止 |
| A5 / R3 | `gorm.G`、显式公开列投影、Transaction 回调与批量关联；保留业务锁/必要 SQL/复合 FK；真实 PG 覆盖版本、零值、回滚和审计原子性，无修改已执行迁移 |
| A6 / R4/R9 | D1 固定工具组合生成 Go/Gin/内部 strict/client、Web SDK/Zod/Query；40 个挂载操作、15 个权限、17 个结果码检查通过，53 个生成文件集合及内容重生成一致，公开产物无内部授权/凭证类型 |
| A7 / R6 | D2 Viper 实例、显式环境绑定、严格解码与 validator；模块覆盖/共享连接/YAML/default、显式空值、内部 clients map 和 Web 根环境过滤测试通过，删除旧占位解析器 |
| A8 / R14 | Chromium 核心流程及专用人工浏览器验收通过；登录/管理/组织切换/双语/主题/窄屏保持可用。工作台、UI 组件、字体和主题文件相对实现起点无差异；既有模板视觉问题见下文 |
| A9 / R7/R10 | [角色贯通范例](../../development/role-management.md)链接真实协议、Gin、用例、仓储、SDK/Query/RHF 和测试；规则、配置/测试/日志指南、贡献指南与模块文档已同步 |
| A10 / R11 | 版本和来源说明见框架能力记录；日志敏感值排除验证通过，提交范围检查排除私有环境/日志/本机路径；Git 交付依据本次用户条件授权 |
| A11 / R12/R13 | D4 官方 limiter Redis Store 原子计数/TTL/前缀/429/503/取消；Goose Provider/Store 缺表无 DDL、版本落后/领先/并发迁移；Cobra 参数/端点无网络，标准取消/关闭/资源清理 |
| A12 | D5 生成 SDK、Query options/mutation、身份/会话/组织授权版本 key、取消/失效；RHF + 生成 Zod/Controller/FieldError；敏感密码使用短期表单和 SDK，不进入 MutationCache；Table 受控服务端分页 |

D3 中间件次序：共享 Gin 关联标识/访问日志和受控 Recovery → IAM 公开组 SCS 提交屏障 → 错误映射 → CSRF/Origin → 来源限流 → 官方契约校验 → 生成绑定/业务 Handler。内部监听独立服务认证与只读 SCS 加载；Console 原生流不进入提交屏障。不是将旧 HTTP Handler 整体套进 Gin。

## 本地命令结果

| 命令/检查 | 实际结果 |
| --- | --- |
| `pnpm contracts:check` | 通过；实际路由/权限/结果码及 53 个生成文件集合、内容校验通过；校验不改工作区 |
| `pnpm test:unit` | 根脚本、全部 Go 单元及 Web 通过；Web 9 文件、17 用例 |
| `pnpm typecheck` | 通过；Next route types 与 TypeScript |
| `pnpm lint` | 退出 0；仍有 4 个 warning、2 个 info（现有表格嵌套条件、E2E 非空断言、shell 样式排序），未全局关闭规则或调整模板基准 |
| `go vet ./src/...` | 通过 |
| `make lint-go` | 通过，0 issues；golangci-lint 2.15.0 / Go 1.27.3 |
| `make build` | 通过；本机 Go 与 Next 生产构建，不构建应用容器 |
| `pnpm test:integration --docker --race` | 隔离真实 PostgreSQL/Redis 全部通过，含 race 检查；不连接开发库 |
| `pnpm test:e2e --docker` | Chromium 6/6 通过（46.6 秒）；双 IAM 副本、Console、同源 Next、TLS、登录/改密/角色/组织/跨标签/SSE；结构化文件日志与秘密排除检查通过 |
| `pnpm test:e2e --docker --manual` | 专用浏览器代理在同一隔离应用栈验收，完成后通过 stop 信号清理自有进程/容器；细节见下文 |
| Recovery 测试有效性 | 临时反转“响应是否已提交”条件，HTTP/流边界测试如期失败；恢复原文件后测试通过，变异未进入交付 |
| `git diff --check` / 文档检查 | 无尾随空白/冲突标记；修改文档本地链接和代码围栏检查；历史失效引用单独区分 |

真实组件测试覆盖缺失 schema 状态无隐式 DDL、迁移状态/重试/并发、Goose 版本源、GORM PostgreSQL 错误链、密码计算与凭证版本竞争、SCS 提交/异常补偿、CSRF/Origin、服务鉴权、Redis 原子/TTL/失效关闭及上下文取消。未执行生产数据库破坏性备份恢复演练。

## 浏览器验收及边界

按 AGENTS 的 Computer Use 约定使用专用代理操作，自动化 Chromium 与人工浏览器互补：

- 登录、二次认证、退出和跨标签失效；新账号登录/改密由自动化浏览器实际覆盖，人工工具不重复输入新密码。
- 创建/编辑组织；角色创建/信息修改/权限影响预览/保存；添加已有成员并分配、重开验证角色；账号显示名称、禁用及恢复。
- 受限账号仅显示可访问菜单；跨组织拒绝及 Console 403；SSE 在撤权后停止。
- 英文/中文切换、刷新保留；桌面与 390 px 窄屏；4 个主题预设、18 个字体选项和布局选项可选择，抽样确认视觉变化；最终恢复默认主题/字体/布局和英文。

截图：[工作台桌面](screenshots/dashboard-desktop.jpg)、[窄屏](screenshots/dashboard-mobile.jpg)、[角色对话框](screenshots/role-edit.jpg)，均来自隔离测试数据。会话中捕获的浏览器 Console error/warn 为空；正常导航/身份切换有取消请求，写入后通过重新打开验证持久化。网络事件缓冲曾截断，未保留全会话异常事件，因此不宣称“全会话零网络错误”。

已知模板基准问题：漏斗末端 Leads 标签有约 11.6 px 裁切，灰色柱条白字对比偏弱。相关工作台、UI 和样式文件相对 `f921760` 未改；本地上游 `89f28da` 使用相同图表布局，但未做同尺寸上游渲染对照。依用户“核心样式/页面原样”要求保留，不把本次功能验收解释为模板视觉已无可改进项。

## 依赖安全、来源与保留适配

`govulncheck 1.9.0` 使用 Go 1.27.3 扫描：当前可达调用 0 项漏洞；另报 1 项已导入包及 3 项所需模块公告，扫描器判断当前代码未调用受影响符号，不能描述为依赖零公告。

`pnpm audit --prod --audit-level high` 通过门槛：0 中/高/严重，1 低危。低危为 `deepmerge 4.3.1` 的 [GHSA-gcqx-w48m-w2h4](https://github.com/advisories/GHSA-gcqx-w48m-w2h4)，由已有 `shadcn 4.22.2`/registry 工具链间接引入，审计未提供可安装修复版；本次未新增业务对其调用，未为清空公告而改动模板工具链。后续处理仍应跟踪上游修复。

删除对象：Authboss 桥/专用测试与依赖、Chi 及通用路由/Writer/绑定标记、旧 IP 算法、configfile 占位解析器、自写限流 Lua、硬编码迁移版本、旧 TS-only 产物/生成工具、useList/revision/拉取 effect 和 fields/string-map DSL。核心认证/事务/访问边界断言保留在组件与业务集成用例。

保留的适配各有实际约束：Result 预编码保护状态提交；IAM SCS 屏障保护成功 Cookie/正文及补偿；官方校验器溢出标记补足结构化错误丢失；gorilla/csrf 适配；内部生成客户端外的 TLS/凭证/响应上限/授权结果策略。未建设通用 HTTP 框架、反射绑定器、配置方言或代码生成后台。

新增组件通过包管理器接入，版本/许可与来源见框架记录。GVA 固定提交仅作设计参考，无业务源码复制；既有前端模板 MIT 鸣谢和许可保留。未运行部署、远程 CI 或应用容器构建；隔离测试资源均按拥有关系清理。
