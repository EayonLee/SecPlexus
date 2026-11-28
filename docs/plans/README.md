# 需求与设计计划索引

本目录按具体需求维护 Spec 与 Plan。索引用于查找主题和审核状态，实施任务、验证证据和剩余事项以对应 Plan 为准，不在这里重复维护。

当前处于初期开发阶段，交付以本地实现、构建、测试及必要的容器验证为准。远程 CI、生产部署与生产升级暂不开展，后续另行安排，不作为当前计划的未完成项；已有相关配置不代表已经执行。

| 计划 ID | 需求名称 | 范围 | 状态 | Spec | Plan |
|---|---|---|---|---|---|
| [flat-authorization](flat-authorization/spec.md) | 扁平授权：权限只由用户与角色表达 | 用户→角色→权限单一路径；扁平权限键 7 项；`/api/v1/iam` 扁平路由；内置数据启动幂等写入；Console 与审计无范围维度；单基线迁移 | 已实施（封板待人工验收） | [spec](flat-authorization/spec.md) | [plan](flat-authorization/plan.md) |
| [engineering-standards](engineering-standards/spec.md) | 工程规范固化：数据库设计、后端结构与日志 | 数据库注释与封闭取值枚举化；Go 目录/分层/命名规范；日志简单英文；证据驱动 | 已实施（Windows 实测） | [spec](engineering-standards/spec.md) | [plan](engineering-standards/plan.md) |
| [cross-platform-runtime](cross-platform-runtime/spec.md) | 跨平台运行时与开发工具 | Windows/macOS/Linux 同一 `pnpm` 入口；进程树登记与停止；Make 可选；测试不使用 shell 通配符；构建/E2E 平台适配 | 已实施（Windows 实测） | [spec](cross-platform-runtime/spec.md) | [plan](cross-platform-runtime/plan.md) |
| [timezone-and-user-preference](timezone-and-user-preference/spec.md) | 全平台时区统一与用户时区偏好 | 服务端固定 UTC；用户级时区偏好存库；显示按用户/浏览器回落 | 已实施 | [spec](timezone-and-user-preference/spec.md) | [plan](timezone-and-user-preference/plan.md) |
| [test-strategy-and-coverage](test-strategy-and-coverage/spec.md) | 测试规范固化与存量测试补齐 | 改动类型与必需测试层判定表；存量无测试包补齐 | 规范已落地，存量部分完成 | [spec](test-strategy-and-coverage/spec.md) | [plan](test-strategy-and-coverage/plan.md) |
| [audit-role-settings-consolidation](audit-role-settings-consolidation/spec.md) | 审计与角色管理简化、设置区合并 | 审计单一列表（操作者/时间/结果筛选）；角色去 code/kind；创建/编辑改右侧抽屉；删除有用户持有的角色被拒；设置区合并为 `/settings` 单入口 | 已实施：审计单列表、角色去 code/kind、设置单入口 | [spec](audit-role-settings-consolidation/spec.md) | [plan](audit-role-settings-consolidation/plan.md) |
| [management-pages-consistency](management-pages-consistency/spec.md) | 管理页一致性与用户列表去重 | 页首和控制台工作台共用标题；页首主操作与列表筛选统一；列全左对齐；集合加载、失败、无结果与重试统一；侧向 Sheet 尺寸与表单栅格统一；删除重复说明；用户列表按用户聚合；角色抽屉不重复展示用户 | 已实施，四个系统设置路由统一，Profile、Users、Roles 和 Audit 对齐 Web 模板 | [spec](management-pages-consistency/spec.md) | [plan](management-pages-consistency/plan.md) |
| [web-surface-localization](web-surface-localization/spec.md) | Web 表面层级与国际化入口 | 全局壳层、明暗表面、共享列表和浮层、双语词典及缺词回退 | 已实施并通过隔离浏览器验收 | [spec](web-surface-localization/spec.md) | [plan](web-surface-localization/plan.md) |
| [go-quality-gates](go-quality-gates/spec.md) | Go 质量闸门：lint、格式化与可达漏洞真正生效 | golangci-lint 显式配置（default: none + 17 项）；gofmt/goimports 作为 formatter；govulncheck 常驻 CI；工具链固定 go1.27.3；存量 10 个问题清零 | 已实施（PR #40） | [spec](go-quality-gates/spec.md) | [plan](go-quality-gates/plan.md) |
| [sqlc-data-layer](sqlc-data-layer/spec.md) | 数据访问层改造：SQL 可校验、HTTP JSON 回到 Go | sqlc 以 goose 迁移为 schema 源；repository 只返回类型化行；动态过滤改静态 narg；`jsonb_build_object` 4 处清零；GORM 最终移除；沉淀 `docs/development/data-access.md` | 已实施：B0–B6 全部合并（#41–#54），GORM 已移除；逐文件迁移轨迹见 plan 第十一节 | [spec](sqlc-data-layer/spec.md) | [plan](sqlc-data-layer/plan.md) |
| [ingest-and-normalization](ingest-and-normalization/spec.md) | 数据接入与归一化（Ingest 首版） | Syslog、HTTP、Kafka 接收；解析映射、OCSF 输出、接入点管理 | 主链路已实施；当前事件交付见 [Event 契约](event-contract/spec.md) | [spec](ingest-and-normalization/spec.md) | [plan](ingest-and-normalization/plan.md) |
| [ingest-naming-and-authorization](ingest-naming-and-authorization/spec.md) | Ingest 命名对齐与统一鉴权 | 命名对齐 Splunk（`input` / `protocol` / `mapping`）；鉴权逻辑收敛进 `iamclient`，Console 与 Ingest 零鉴权代码；权限由 IAM 数据库数据控制；登记前几轮审查遗留缺口 | 已实施（#96）：命名对齐、统一鉴权、权限目录漂移检测 | [spec](ingest-naming-and-authorization/spec.md) | [plan](ingest-naming-and-authorization/plan.md) |
| [ingest-closeout](ingest-closeout/spec.md) | Ingest 首版收尾 | 三协议接入与诊断、样本试算、首个厂商映射、审计可靠性、Kafka 和四服务验收 | 功能已验收；持续容量待达标 | [spec](ingest-closeout/spec.md) | [plan](ingest-closeout/plan.md) |
| [ingest-console-redesign](ingest-console-redesign/spec.md) | 控制台数据接入重构 | 独立菜单、接入点批量启停与诊断、规则清单、字段试算与映射、共享列表控件 | 已实施并完成联调与浏览器验收 | [spec](ingest-console-redesign/spec.md) | [plan](ingest-console-redesign/plan.md) |
| [ingest-source-identity](ingest-source-identity/spec.md) | 接入来源与规则清单 | 厂商和产品独立建模、单列表、接收诊断与编辑布局 | 已实施并通过隔离联调及浏览器验收 | [spec](ingest-source-identity/spec.md) | [plan](ingest-source-identity/plan.md) |
| [ingest-operator-workflows](ingest-operator-workflows/spec.md) | 接入向导与解析规则编辑器 | 四步接入、接收诊断、样本与字段逐项对照、宽屏布局 | 已实施并通过四服务联调与浏览器验收 | [spec](ingest-operator-workflows/spec.md) | [plan](ingest-operator-workflows/plan.md) |
| [event-contract](event-contract/spec.md) | 平台标准事件契约 | Ingest 字段契约、Kafka → ClickHouse 归档与 Event 回查；Alert 仅留设计边界 | 首版规则追溯、事件存储、备份恢复与浏览器主流程已验证，生产容量和发布演练待验 | [spec](event-contract/spec.md) | [plan](event-contract/plan.md) |
| [repository-layout](repository-layout/spec.md) | 仓库文件归属审查 | 30 个根文件、模块 sqlc 配置、Ingest 持久化目录、Web 构建上下文及迁移检查入口 | Ingest 持久化目录与迁移检查入口已调整，模块 sqlc 拆分及 Web 镜像整改待实施 | [spec](repository-layout/spec.md) | [plan](repository-layout/plan.md) |
| [platform-architecture-review](platform-architecture-review/spec.md) | 平台架构与存量实现复审 | 各 app 共同规范与 README、开发迭代、升级部署、运行维护与排障演练 | [仓库审查表](platform-architecture-review/review.md)的九项整改已实施，其他事项按各专题推进 | [spec](platform-architecture-review/spec.md) | [plan](platform-architecture-review/plan.md) |
| [ingest-mixed-event-parsing](ingest-mixed-event-parsing/spec.md) | 混合日志解析 | 按日志匹配分支、异常 Event、接入与规则工作区 | 已实施，核心链路已验收，生产容量待验证 | [spec](ingest-mixed-event-parsing/spec.md) | [plan](ingest-mixed-event-parsing/plan.md) |
| [ingest-rule-cleanup](ingest-rule-cleanup/spec.md) | 解析规则清理 | 旧版兼容、多行合并与前向清理 | 已实施并完成隔离联调 | [spec](ingest-rule-cleanup/spec.md) | [plan](ingest-rule-cleanup/plan.md) |
| [ingest-rule-wizard](ingest-rule-wizard/spec.md) | 样本驱动的解析规则与统一字段目录 | 四步向导、字段全集、逐条分类、常用 OCSF 类、术语审核、单一 v1 契约与共享 | 已实施，核心流程已验收，持续容量待验证 | [spec](ingest-rule-wizard/spec.md) | [plan](ingest-rule-wizard/plan.md) |
| [ingest-rule-validation](ingest-rule-validation/spec.md) | 解析规则基础字段校验 | 名称去重、字符与长度、前后端一致、字段级错误、数据库约束 | 已实施 | [spec](ingest-rule-validation/spec.md) | [plan](ingest-rule-validation/plan.md) |
| [ingest-rule-field-workflow](ingest-rule-field-workflow/spec.md) | 按事件类型配置解析规则与字段映射 | 基础配置选类型、标准字段优先、类型化默认值、处理预览和事件结果 | 向导、详情与管理列表已实施 | [spec](ingest-rule-field-workflow/spec.md) | [plan](ingest-rule-field-workflow/plan.md) |
| [ingest-audit-fixes](ingest-audit-fixes/spec.md) | 接入与解析规则走查修复 | 分隔符报文边界、规则详情刷新和验证摘要、Syslog 监听与部署说明 | 已实施并通过浏览器及真实链路验收 | [spec](ingest-audit-fixes/spec.md) | [plan](ingest-audit-fixes/plan.md) |
| [configuration-entrypoints](configuration-entrypoints/spec.md) | 配置入口一致性 | 根配置、服务监听、Compose、Web 代理、归档与测试入口 | 已实施并通过隔离配置验收 | [spec](configuration-entrypoints/spec.md) | [plan](configuration-entrypoints/plan.md) |
| [commercial-delivery](commercial-delivery/spec.md) | 商业交付整改（审查第 2–16 项） | 可信入口、样本与规则修订、接入生效、OCSF、Kafka 重放、恢复与容量 | 实施中，安装包延后 | [spec](commercial-delivery/spec.md) | [plan](commercial-delivery/plan.md) · [同行对照](commercial-delivery/references.md) |
| [first-release-baseline](first-release-baseline/spec.md) | 首次发行基线 | 共享依赖、模块结构、事件链路、数据模型、全栈 Compose、恢复与容量；一次清理试制配置、旧代码与旧模型 | 主链路已实施，生产部署与容量待验收 | [spec](first-release-baseline/spec.md) | [plan](first-release-baseline/plan.md) |

Web、IAM、Ingest 与当前 Console 的收尾范围和验收以[首次发行基线](first-release-baseline/spec.md)为准。已实施的页面交互及验证证据见[实施计划中的前轮要求](first-release-baseline/plan.md#前轮要求与验收入口)。试制兼容、可选 ClickHouse、Event 过期和安装包以当前基线替代旧方案。

## 维护规则

- 先查已有主题。同一需求的修订更新原文；新的交付目标单独建立 `docs/plans/<request-slug>/`，不要把一个模块的所有需求持续追加到同一计划。
- 目录使用描述具体需求的 `kebab-case` 名称；目录内保留 `spec.md` 和 `plan.md`。日期、版本、范围和状态写在文档中，不用宽泛模块名替代需求名称。
- Spec 记录行为契约与验收标准，Plan 记录方案、任务依赖、进度及验证证据；模块 README 维护长期业务与技术说明。
- 新增、重命名、范围或审核状态变化时同步本索引及引用；状态必须基于实际进展，不能把设计完成写成实现完成。
- 计划按 [AGENTS.md](../../AGENTS.md) 审核后实施；设计确认不自动授权提交、推送或部署。
- 仅保留可公开的需求、设计和验证信息；内部调研环境、私有项目标识和工作区绝对路径不进入仓库。
