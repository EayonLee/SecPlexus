# 需求与设计计划索引

本目录按具体需求组织 Spec 与 Plan。索引用于查找主题和审核状态，实施任务、验证证据和剩余事项以对应 Plan 为准，不在这里重复维护。

当前处于初期开发阶段，交付以本地实现、构建、测试及必要的容器验证为准。远程 CI、生产部署与生产升级暂不开展，后续另行安排，不作为当前计划的未完成项；已有相关配置不代表已经执行。

| 计划 ID | 需求名称 | 范围 | 状态 | Spec | Plan |
|---|---|---|---|---|---|
| gin-service-refactor | Gin 与全栈框架能力重构 | IAM / Console / Gin 中间件 / 数据与配置命令 / 限流 / 契约与 Web 工程 / 治理 | 原生 Gin/配置/导航与无 OpenAPI 收敛完成；独立审查修复及分层回归、Preview E2E 7/7 完成；未提交 | [Spec](gin-service-refactor/spec.md) | [Plan](gin-service-refactor/plan.md) |
| local-origin-compatibility | 本机来源兼容 | IAM / CSRF / 浏览器验收 | 实施及本地验收完成；未提交 | [Spec](local-origin-compatibility/spec.md) | [Plan](local-origin-compatibility/plan.md) |
| web-internationalization | Web 中英文国际化 | next-intl / 消息词典 / 语言切换 / 格式与文档 | 实施及本地验证完成；真实认证E2E未执行 | [Spec](web-internationalization/spec.md) | [Plan](web-internationalization/plan.md) |
| web-workbench | 安全智驾工作台与服务职责收敛 | Console 边界 / Web 页面与菜单 / 按需模板接入 | 实施及本地检查完成；登录后浏览器验收受限 | [Spec](web-workbench/spec.md) | [Plan](web-workbench/plan.md) |
| redis-acl-and-configuration-review | Redis 用户配置与配置链路审查 | IAM / 配置 / Compose / 工程审查 | 实施及本地验收完成；未提交 | [Spec](redis-acl-and-configuration-review/spec.md) | [Plan](redis-acl-and-configuration-review/plan.md) |
| simplify-password-policy | 简化密码策略和本地默认认证 | IAM / Web / 配置 / 数据库迁移 | 实施及本地验收完成；未提交 | [Spec](simplify-password-policy/spec.md) | [Plan](simplify-password-policy/plan.md) |
| user-authentication-and-access-control | 用户认证、组织访问控制与管理中心 | Web / IAM / Console；账号与Session、组织成员、自定义角色及权限、管理中心导航 | 首版实施及本地分层验收完成；未提交 | [Spec](user-authentication-and-access-control/spec.md) | [Plan](user-authentication-and-access-control/plan.md) |
| structured-module-configuration | 结构化模块配置与轮转日志 | IAM / Console / Web / 启动与部署 | 实施及本地验收完成；未提交 | [Spec](structured-module-configuration/spec.md) | [Plan](structured-module-configuration/plan.md) |
| go-code-structure-and-conventions | Go 代码结构与协作规范 | 路由 / 代码规范 / AGENTS | 实施及本地验收完成；未提交 | [Spec](go-code-structure-and-conventions/spec.md) | [Plan](go-code-structure-and-conventions/plan.md) |
| go-http-foundation | Go 路由与 HTTP 基础能力 | IAM / Console / 共享包 | 历史记录，后续由平台重构计划承接 | [Spec](go-http-foundation/spec.md) | [Plan](go-http-foundation/plan.md) |
| platform-service-architecture | 平台服务架构与工程规范重构 | HTTP / Go 服务 / 数据 / 缓存 / Web | 实施及本地分层验收完成；未提交 | [Spec](platform-service-architecture/spec.md) | [Plan](platform-service-architecture/plan.md) |
| spx-code-standard-skill | 项目代码规范与审查技能 | 项目技能 / 工程规范 | 制作及本地核对完成；未提交 | [Spec](spx-code-standard-skill/spec.md) | [Plan](spx-code-standard-skill/plan.md) |
| replace-web-template | Web 模板整体替换并重建 IAM | Next.js / shadcn / IAM 页面 / 启动部署 / 文档 | 实施及本地验收完成；未提交 | [Spec](replace-web-template/spec.md) | [Plan](replace-web-template/plan.md) |
| core-observability-and-build-layout | 核心链路日志与构建目录整理 | IAM / Console / Web instrumentation / Redis / Docker / 技能 | 实施及本地分层验收完成；未提交 | [Spec](core-observability-and-build-layout/spec.md) | [Plan](core-observability-and-build-layout/plan.md) |

## 维护规则

- 先查已有主题。同一需求的修订更新原文；新的交付目标单独建立 `docs/plans/<request-slug>/`，不要把一个模块的所有需求持续追加到同一计划。
- 目录使用描述具体需求的 `kebab-case` 名称；目录内保留 `spec.md` 和 `plan.md`。日期、版本、范围和状态写在文档中，不用宽泛模块名替代需求名称。
- Spec 记录行为契约与验收标准，Plan 记录方案、任务依赖、进度及验证证据；模块 README 维护长期业务与技术说明。
- 新增、重命名、范围或审核状态变化时同步本索引及引用；状态必须基于实际进展，不能把设计完成写成实现完成。
- 计划按 [AGENTS.md](../../AGENTS.md) 审核后实施；设计确认不自动授权提交、推送或部署。
- 仅保留可公开的需求、设计和验证信息；内部调研环境、私有项目标识和工作区绝对路径不进入仓库。
