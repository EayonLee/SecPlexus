# 需求与设计计划索引

本目录按具体需求组织 Spec 与 Plan。索引用于查找主题和审核状态，实施任务、验证证据和剩余事项以对应 Plan 为准，不在这里重复维护。

当前处于初期开发阶段，交付以本地实现、构建、测试及必要的容器验证为准。远程 CI、生产部署与生产升级暂不开展，后续另行安排，不作为当前计划的未完成项；已有相关配置不代表已经执行。

| 计划 ID | 需求名称 | 范围 | 状态 | Spec | Plan |
|---|---|---|---|---|---|
| [timezone-and-user-preference](timezone-and-user-preference/spec.md) | 全平台时区统一与用户时区偏好 | 服务端固定 UTC；用户级时区偏好存库；显示按用户/浏览器回落 | 已实施 | [spec](timezone-and-user-preference/spec.md) | [plan](timezone-and-user-preference/plan.md) |
| [test-strategy-and-coverage](test-strategy-and-coverage/spec.md) | 测试规范固化与存量测试补齐 | 改动类型与必需测试层判定表；存量无测试包补齐 | 规范已落地，存量部分完成 | [spec](test-strategy-and-coverage/spec.md) | [plan](test-strategy-and-coverage/plan.md) |
| [org-tree-and-role-model](org-tree-and-role-model/spec.md) | 组织树、角色权限模型与成员管理重构 | 信息架构重划；组织树（深度上限 3，根 `master`）；权限目录扩展；字段校验统一 | 已实施，未提交 | [spec](org-tree-and-role-model/spec.md) | [plan](org-tree-and-role-model/plan.md) |
| [org-inheritance-and-management](org-inheritance-and-management/spec.md) | 组织授权继承与管理界面修正 | 祖先角色向下继承；**取消组织深度硬上限**；管理员不变量归位应用层；成员子树聚合（可开关）；树形只读展示；reparent 递归重算 depth；脏数据清理 | 待审核（v3：15 缺陷已修，待实施） | [spec](org-inheritance-and-management/spec.md) | [plan](org-inheritance-and-management/plan.md) · [审查](org-inheritance-and-management/review-findings.md) |
| [audit-role-settings-consolidation](audit-role-settings-consolidation/spec.md) | 审计与角色管理简化、设置区合并 | 审计单一列表（操作者/时间/结果筛选）；角色去 code/kind 与内置角色；创建/编辑改右侧抽屉；删除有成员角色被拒；设置区合并为 `/settings` 单入口 | 实施中 | [spec](audit-role-settings-consolidation/spec.md) | [plan](audit-role-settings-consolidation/plan.md) |
| [org-user-management-merge](org-user-management-merge/spec.md) | 组织/用户管理合并与「成员 → 用户」统一命名 | 组织管理改为左树右列表；成员页与平台用户页合并为用户管理；全库表/列/权限键/文案 members→users | 实施中 | [spec](org-user-management-merge/spec.md) | — |
| [ingest-and-normalization](ingest-and-normalization/spec.md) | 数据接入与归一化（ingestor 首版） | syslog/HTTP/Kafka 三通道接收；格式探测 + 厂商 YAML 模板映射 OCSF；发布 `spx.ingest.event`；connectors 注册表。不做告警提升/合并（归 correlator），不写 ClickHouse | 待审核，未实施 | [spec](ingest-and-normalization/spec.md) | [plan](ingest-and-normalization/plan.md) |

## 维护规则

- 先查已有主题。同一需求的修订更新原文；新的交付目标单独建立 `docs/plans/<request-slug>/`，不要把一个模块的所有需求持续追加到同一计划。
- 目录使用描述具体需求的 `kebab-case` 名称；目录内保留 `spec.md` 和 `plan.md`。日期、版本、范围和状态写在文档中，不用宽泛模块名替代需求名称。
- Spec 记录行为契约与验收标准，Plan 记录方案、任务依赖、进度及验证证据；模块 README 维护长期业务与技术说明。
- 新增、重命名、范围或审核状态变化时同步本索引及引用；状态必须基于实际进展，不能把设计完成写成实现完成。
- 计划按 [AGENTS.md](../../AGENTS.md) 审核后实施；设计确认不自动授权提交、推送或部署。
- 仅保留可公开的需求、设计和验证信息；内部调研环境、私有项目标识和工作区绝对路径不进入仓库。
