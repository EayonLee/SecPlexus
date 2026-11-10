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
| [audit-role-settings-consolidation](audit-role-settings-consolidation/spec.md) | 审计与角色管理简化、设置区合并 | 审计单一列表（操作者/时间/结果筛选）；角色去 code/kind；创建/编辑改右侧抽屉；删除有用户持有的角色被拒；设置区合并为 `/settings` 单入口 | 实施中 | [spec](audit-role-settings-consolidation/spec.md) | [plan](audit-role-settings-consolidation/plan.md) |
| [management-pages-consistency](management-pages-consistency/spec.md) | 管理页一致性与用户列表去重 | 四个管理页标题与宽度统一；工具条一行（查询左、主操作右）；列全左对齐；右侧抽屉统一 SheetPanel 宽度；删除重复说明；用户列表按用户聚合；角色抽屉不重复展示用户 | 已实施（界面规则随页面改写保留） | [spec](management-pages-consistency/spec.md) | [plan](management-pages-consistency/plan.md) |
| [go-quality-gates](go-quality-gates/spec.md) | Go 质量闸门：lint、格式化与可达漏洞真正生效 | golangci-lint 显式配置（default: none + 17 项）；gofmt/goimports 作为 formatter；govulncheck 常驻 CI；工具链固定 go1.27.3；存量 10 个问题清零 | 已实施（PR #40） | [spec](go-quality-gates/spec.md) | [plan](go-quality-gates/plan.md) |
| [sqlc-data-layer](sqlc-data-layer/spec.md) | 数据访问层改造：SQL 可校验、HTTP JSON 回到 Go | sqlc 以 goose 迁移为 schema 源；repository 只返回类型化行；动态过滤改静态 narg；`jsonb_build_object` 4 处清零；GORM 最终移除；沉淀 `docs/development/data-access.md` | 已实施：B0–B6 全部合并（#41–#54），GORM 已移除；逐文件迁移轨迹见 plan 第十一节 | [spec](sqlc-data-layer/spec.md) | [plan](sqlc-data-layer/plan.md) |
| [ingest-and-normalization](ingest-and-normalization/spec.md) | 数据接入与归一化（ingestor 首版） | syslog/HTTP/Kafka 三通道接收；格式探测 + 厂商 YAML 模板映射 OCSF；发布 `spx.ingest.event`；connectors 注册表。不做告警提升/合并（归 correlator），不写 ClickHouse | 待审核，未实施 | [spec](ingest-and-normalization/spec.md) | [plan](ingest-and-normalization/plan.md) |

## 维护规则

- 先查已有主题。同一需求的修订更新原文；新的交付目标单独建立 `docs/plans/<request-slug>/`，不要把一个模块的所有需求持续追加到同一计划。
- 目录使用描述具体需求的 `kebab-case` 名称；目录内保留 `spec.md` 和 `plan.md`。日期、版本、范围和状态写在文档中，不用宽泛模块名替代需求名称。
- Spec 记录行为契约与验收标准，Plan 记录方案、任务依赖、进度及验证证据；模块 README 维护长期业务与技术说明。
- 新增、重命名、范围或审核状态变化时同步本索引及引用；状态必须基于实际进展，不能把设计完成写成实现完成。
- 计划按 [AGENTS.md](../../AGENTS.md) 审核后实施；设计确认不自动授权提交、推送或部署。
- 仅保留可公开的需求、设计和验证信息；内部调研环境、私有项目标识和工作区绝对路径不进入仓库。
