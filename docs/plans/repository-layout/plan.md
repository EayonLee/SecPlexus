# 仓库布局整改计划

本次完成目录审查和离线拆分试验。以下运行行为变更待实施，可独立交付。运行模块的目录规范、业务与存储边界、源码和部署入口由[平台架构复审计划](../platform-architecture-review/plan.md)承接。

| 优先级 | 审查发现 | 修改及可观察的验收结果 |
|---|---|---|
| 高 | Web Dockerfile 显式复制公开模板为构建时的 `.env.production`，根忽略规则 `**/.env.*` 同时排除了输入文件。 | 在广义排除规则之后精确放行 `!src/apps/web/.env.example`。以干净上下文构建 Web 镜像，确认公开模板可读；用临时哨兵文件验证私有环境文件仍不进入上下文。 |
| 高 | 根 `catalogs:check` 直接执行迁移边界脚本，原入口缺少执行 `main()` 的逻辑。 | 已接通 CLI，并适配 `migrations/postgresql`。临时目录正负例和 `pnpm catalogs:check` 均通过。 |
| 中 | 根 sqlc 配置绑定了两套私有实现；vet 硬编码模块列表，按文本插入规则并写固定的根临时文件。同一 checkout 并发运行会互相覆盖或清理文件。 | 按下述步骤拆分，验证生成、数据库预编译及并发清理。 |
| 低 | 解析语料说明引用了不存在的根 `tests/e2e/`。 | 本次已修正为 Web 实际位置，同时保留根 `make test-e2e` 使用方法。 |

[Web Dockerfile](../../../deploy/docker/web.Dockerfile) · [构建上下文](../../../.dockerignore) · [根命令](../../../package.json) · [迁移检查 CLI](../../../scripts/migrations-check.mjs) · [语料说明](../../../src/apps/ingest/internal/parser/testdata/corpus/README.md)

## sqlc 迁移顺序

1. 把两个查询集分别放入 `src/apps/iam/sqlc.yaml` 和 `src/apps/ingest/sqlc.yaml`。每份配置的 schema、queries、out 使用模块相对路径。保留 IAM 的 database/sql、枚举导入全路径以及 Ingest 的 pgx/v5 和类型覆盖。
2. 根工具按稳定顺序发现 `src/apps/*/sqlc.yaml`，由 generate、diff、vet 共同使用。保留 `make sqlc`、`make sqlc-check`、`make sqlc-vet` 与固定工具版本；任何模块失败须令聚合命令失败。生成和 diff 仍可离线执行。
3. vet 继续只在本次创建的测试库上应用模块 PG 迁移并执行 db-prepare。将 vet 专用配置保存在每次运行独占的位置，正确解析模块路径，连接串通过环境传入。验证两次并行执行及其中一次失败后，各自清理所属资源。
4. 在所有入口切换后移除根配置、旧临时文件约定及相应 ignore 项，更新数据访问指南和测试说明。CI 继续调用根入口，在 IAM 与 Ingest 都验证过后再关闭此项。

**已验证**　当前根配置通过 `make sqlc-check`。在临时目录分别复制两个模块的迁移、查询与现有生成代码，只调整三个文件系统路径，各用 sqlc v1.31.1 执行 `diff -f`，均退出 0。直接调用迁移边界的 `main()` 能检查当前仓库并通过，在临时目录构造跨 schema 外键后，直接执行脚本仍退出 0，显式调用同一脚本的 main 则退出 1 并报告违规目标。

**待验证**　sqlc 模块配置拆分、并发 vet 和 Web 镜像构建仍按各自步骤验收。Ingest 目录调整后，现有根入口的 `sqlc-check` 与真实 PostgreSQL `db-prepare` 已通过。

[现有 sqlc 配置](../../../sqlc.yaml) · [Make 入口](../../../Makefile) · [vet 工具](../../../scripts/sqlc-vet.mjs) · [CI 调用](../../../.github/workflows/ci.yml)

## Ingest 持久化整理

1. 将标准 Event 消费和存储实现迁入各自私有包。把接入点读写、启用配置和解析规则操作归到对应文件，沿用原有事务、查询和调用行为。
2. 移动 PostgreSQL 原始 SQL 和 ClickHouse DDL 模板并更换嵌入路径。保留迁移文件内容、版本编号、账本和现有运行命令。sqlc 与 PostgreSQL 迁移边界检查读取明确的 PostgreSQL 路径。
3. 运行 Ingest 单元及实库迁移回归、sqlc 检查、边界检查 CLI 正负例、真实 Kafka 与隔离 ClickHouse 写入回归。对照迁移文件与生成 DDL，验证目录移动前后的字节一致性。

Ingest 竞态测试、真实 PostgreSQL 仓储测试、Kafka 与隔离 ClickHouse 写入测试通过。`make sqlc-check`、`make sqlc-vet` 及 `pnpm catalogs:check` 通过。五份 PostgreSQL 迁移、ClickHouse 模板及 30 天保留期生成的 DDL 均与移动前的 SHA-256 相同。
