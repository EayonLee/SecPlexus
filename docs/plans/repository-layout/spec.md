# 仓库文件归属审查

审查范围为工作树中 Git 已跟踪和未忽略的候选文件。逐项核对了 30 个根文件，按用途检查了 7 个一级目录及其调用入口。检查关注工具如何发现配置、哪个模块解释文件、移动后哪些构建和测试会改变。私有环境值与构建缓存不在审查内容内。

## 根文件逐项结论

| 文件 | 归属和结论 |
|---|---|
| `sqlc.yaml` | 拆到 IAM、Ingest 模块根。两组 SQL 的迁移、查询、输出和 Go 类型都由各自模块维护。现有文件头只说明 IAM 的驱动选择，对 Ingest 不适用。 |
| `Makefile`、`package.json` | 保留仓库级命令。开发者在根目录构建、启动和检查多模块，CI 复用这些入口。 |
| `go.mod`、`go.sum` | 全仓共用一个 Go module，应用及共享包都在它的依赖图内。保留。 |
| `pnpm-workspace.yaml`、`pnpm-lock.yaml` | 工作区发现与冻结安装以根为起点，Web 镜像也需读取。保留。 |
| `.env.example` | 整个平台的本地启动模板。Web 自身的公开模板另有独立构建调用方，保留两处并核对用途。 |
| `.dockerignore` | Compose 使用仓库根作为构建上下文，文件位置正确。排除规则与 Web 的 COPY 存在冲突，见实施计划。 |
| `.gitignore`、`.gitattributes`、`.editorconfig` | 生效范围覆盖全仓。保留在共同上级目录。 |
| `.golangci.yml`、`.gitleaks.toml`、`osv-scanner.toml` | 扫描范围是全仓 Go、密钥或依赖。lint 在根运行，两项安全扫描在 Make 与 CI 显式引用配置。保留当前入口。 |
| `AGENTS.md`、`CLAUDE.md` | 共同开发约束与工具入口。后者引用前者，规则维护一份。 |
| `DESIGN.md`、`.impeccable.md` | 前者拥有视觉规范，后者向设计工具提供产品和用户上下文。样式参数统一引用前者。 |
| `README.md`、`README.zh-CN.md`、`CHANGELOG.md`、`LICENSE` | 项目入口、版本记录与项目许可，适用于整个发行物。保留。 |
| `CONTRIBUTING.md`、`CONTRIBUTING.zh-CN.md`、`CODE_OF_CONDUCT.md`、`GOVERNANCE.md`、`MAINTAINERS.md`、`SECURITY.md`、`SUPPORT.md` | 面向贡献者、维护者与安全报告人的仓库入口。保留现有可发现位置和文档链接。 |

## 一级目录和模块边界

| 位置 | 抽查的边界与处置 |
|---|---|
| `src/apps/` | IAM、Ingest 各自拥有迁移、查询和生成代码。Web 的 Next、Biome、shadcn、Vitest、Playwright 配置、浏览器用例及模板许可证随 Web 保留。拟新增的 Event JSON 和 ClickHouse 迁移随 Ingest，Alert 契约随未来 correlator。 |
| `src/packages/` | Go 跨应用代码、共享结果码及生成清单。现有 `generated-catalogs.json` 列出权限和结果码的四个派生文件，由根生成脚本检查。Event 权威定义单独随领域模块管理。 |
| `scripts/` | 启停、构建、目录生成、跨模块检查及相应 Node 测试由根命令使用。sqlc 的跨模块调度适合留在此处，模块 SQL 和驱动覆盖项下沉。迁移边界 CLI 有独立发现，见计划。 |
| `tests/` | 现有 `tests/integration/` 包含跨应用依赖管理、数据库和 Kafka 测试工具。浏览器入口实际在 `src/apps/web/tests/e2e/`。解析语料继续留在 Ingest 包内的 `testdata/`。 |
| `deploy/` | Compose 描述、Go/Web Dockerfile 与容器入口随部署维护。构建上下文仍为仓库根，检查 Dockerfile 时须联查根忽略规则。 |
| `docs/` | 架构在 design，长期开发约定在 development，待实施工作在 plans。已实施的旧计划保留当时的设计，新路径在现行开发文档与新计划中说明。 |
| `.github/`、`.agents/` | 前者保存 GitHub 自动发现的工作流、依赖更新、所有者与模板。后者由仓库分发技能，技能的资源和许可证随包保留。 |

本地运行可能产生 `.spx/`、日志、二进制、依赖和框架缓存。这些目录的存在本身并不构成源码归属问题。构建上下文与 Git 跟踪范围需各自验证。

## Ingest 持久化目录

`internal/repository` 内接入点、解析规则、启用配置加载和审计按职责命名文件，共用 PostgreSQL 仓储及管理写事务。独立的 Event 持久化放在 `internal/repository/event`，共同维护物理投影、批量写入和结构核对。标准 Event 的 Kafka 消费循环放在 `internal/consumer`。Ingest 按数据库在 `migrations/postgresql` 和 `migrations/clickhouse` 管理版本化文件，Go 嵌入、sqlc 和迁移边界检查各读取对应目录。

Grafana 将业务能力与共享 SQL 设施分开，Snuba 将 Kafka 消费流程与存储写入分开。SecPlexus 结合现有仓储规范，以持久化对象确定包和文件的名称，以数据库方言划分迁移目录。[Grafana 后端包层级](https://github.com/grafana/grafana/blob/main/contribute/backend/package-hierarchy.md) · [Snuba 消费流程](https://getsentry.github.io/snuba/architecture/consumer.html) · [Go 嵌入规则](https://pkg.go.dev/embed)

[实施顺序与验证证据](plan.md) · [数据访问约定](../../development/data-access.md) · [Event 契约](../event-contract/spec.md)
