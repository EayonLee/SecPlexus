# 实施与验收

依据 [spec.md](spec.md)。规范先行，再按判定表补存量。

## 依赖关系

第 1 至 3 项是规范与基线，必须先做，否则补齐没有判据。
第 4 项起按包推进，彼此独立，可并行或分批提交。

## 任务

### 第一组：规范落地

- [x] `docs/testing.md` 增加「改动类型与必需测试层」判定表，含「不需要」列，明确纯样式与文案不补测试。
- [x] `docs/testing.md` 补强制规则：先失败测试、禁止 sleep 掩盖、禁止删测试变绿、禁止断言实现细节、集成测试隔离清理。
- [x] `docs/testing.md` 补「集成测试缺 `SPX_TEST_POSTGRES_URL` 时跳过而非失败」的明确约定，避免被误判为缺陷。
- [x] `AGENTS.md` 的 Testing 段增加强制最低要求与禁止清单，保持三到五条，继续指向 `docs/testing.md` 作为唯一细节来源。
- [x] 检查 `CONTRIBUTING.md` 与 `CONTRIBUTING.zh-CN.md` 是否需要同步一句话指向判定表；两份文档保持一致。

### 第二组：基线

- [x] 记录当前 Go 覆盖率基线，区分「默认不跑的集成测试」与「真的没有测试」两类 0%。
- [x] 记录 Web 侧测试现状作为基线。实测：单元测试 17 个文件（15 个 `.test.ts` + 2 个 `.test.tsx`），`vitest run` 收集 17 文件 / 51 测试；E2E 1 个规格文件 `tests/e2e/identity.spec.ts`。原计划写的 18 个文件与实测不符，以实测为准。
- [x] 把基线写入 `plan.md` 的验收证据区，作为后续补齐的对照。

### 第三组：存量补齐（按对外行为，可分批）

- [x] `iam/internal/handler/management`：新增参数绑定契约单测（10 个用例），断言非法输入收集 `ErrInvalid` 且不触达业务用例；允许/拒绝路径由 `handler` 包集成测试覆盖（`TestManagementBindingRejectsInvalidInputBeforeService`）。
- [x] `iam/internal/router`：路由注册与鉴权分组契约（由集成测试覆盖）。
- [x] `iam/internal/service`：新增 `validEmail`/`validName` 表驱动单测；账号、成员、组织、密码、角色、预览的关键路径由既有集成测试覆盖。带 `-tags=integration` 覆盖率 43.7%。
- [x] `iam/internal/repository`：新增时区读写（不递增 auth_epoch、空值存 NULL、列可空）、维护清理保留期边界、恢复后撤销会话三组集成测试。带 `-tags=integration` 覆盖率 40.4%。
- [x] `console/internal/handler`：SSE 与访问上下文集成测试。
- [x] `console/internal/bootstrap`：启动装配与配置校验测试。
- [x] `console/internal/router`：路由契约测试。

每批完成的判据：新增测试在移除对应实现后确实失败。

## 验证入口

```sh
pnpm test:unit
go test ./src/...
go test -tags=integration ./src/apps/iam/...     # 需 SPX_TEST_POSTGRES_URL 或 --docker
pnpm typecheck
pnpm lint
pnpm catalogs:check
node scripts/build.mjs
```

## 基线记录

实施前的实测（`go test -cover ./src/...`，不含 integration 标签）：

- 全仓 Go 覆盖率约 63%。
- 真正无测试文件的包：`console/internal/handler`、`console/internal/bootstrap`、`console/internal/router`、`iam/internal/router`、`iam/internal/handler/management`、`iam/internal/service`（仅有 1 个 integration 测试文件）、`iam/internal/repository`（17 源文件 3 测试）。
- 显示 0% 但实际有集成测试、只是默认不跑的包：`iam/internal/service`、`iam/internal/authentication`。

实施后的实测：

| 包 | 实施前 | 实施后 |
|---|---|---|
| `console/internal/handler` | 0%（无测试文件） | 29.0%（8 个用例） |
| `console/internal/bootstrap` | 0%（无测试文件） | 78.9%（2 个用例） |
| `console/internal/router` | 0%（无测试文件） | 87.5%（3 个用例） |
| `iam/internal/identity` | 已有 | 新增 11 个表驱动子用例（时区校验） |
| `iam/internal/handler` | 已有 8 个集成测试 | 新增 2 个（时区偏好、跨组织拒绝） |

## 验收证据

最终验证（逐条实测，命令与退出码）：

| 检查 | 命令 | 结果 |
|---|---|---|
| Go 构建 | `go build ./...` | 0 |
| Go 静态检查 | `go vet ./src/...` | 0 |
| Go 单测 | `go test ./src/...` | 0 |
| Go 格式 | `gofmt -l src` | 无输出 |
| 类型检查 | `pnpm typecheck`（`next typegen && tsc --noEmit`） | 0 |
| 单元测试 | `pnpm test:unit`（含 `scripts/*.test.mjs` + Go + vitest） | 0，vitest 17 文件 / 51 测试 |
| lint 门禁 | `pnpm lint`（即 `biome lint`） | 0 |
| 根构建 | `node scripts/build.mjs` | 0 |
| 业务目录校验 | `pnpm catalogs:check` | 0 |
| E2E | `node tests/e2e/run.mjs` | 13/13 通过 |

集成测试：`go test -tags=integration ./src/apps/iam/... ./src/apps/console/...` 除一项外全部通过。唯一失败是 `TestBackupRestoreRevokesRestoredCredentialsBeforeServing`，原因是本机未安装 `pg_dump`/`psql`（`restore_integration_test.go:77` 显式要求匹配服务器主版本的客户端工具）。该用例在 HEAD 已存在且本轮未改动，属环境限制而非代码缺陷，已用 `command -v pg_dump` 确认工具缺失。

覆盖率基线：见上文对照表。整体实测（`go test -cover ./src/...`，不含 integration 标签）：25 个包，平均 47.6%。

`biome check` 与 `pnpm lint` 的差异需说明：`biome check` 额外包含格式化检查，共报 7 个 error，全部落在本轮未触碰的文件上——`src/app/layout.tsx` 的解构换行、`src/auth/{permissions,result}.gen.ts` 两个生成文件、`tests/e2e/run.mjs`、`tests/unit/{input-models,proxy-errors}.test.ts`、`tsconfig.json` 的格式差异，以及 `data.tsx`/`layout-controls.tsx`/`account.tsx`/`login.tsx`/`profile.tsx`/`workbench/index.tsx` 的 6 条 lint 提示。逐项取 HEAD 版本用 `biome check --stdin-file-path` 复核，同样报错，证明均非本轮引入；其中 `profile.tsx` 的告警行在本轮只被新增的 import 向下推移了一行，原 className 与 HEAD 完全一致。本轮改动的 12 个文件在 `biome lint` 下零问题。

补齐结果（本轮实测，均带 `-tags=integration` 连真实 PG/Redis）：

| 包 | 补齐前 | 补齐后 |
|---|---|---|
| `iam/internal/service` | 0.7%（默认不跑集成时） | 43.7% |
| `iam/internal/repository` | 3.8%（同上） | 40.4% |
| `iam/internal/handler/management` | 0%（无测试文件） | 10.1%（绑定契约单测） |
| `iam/internal/handler` | — | 74.7% |
| `console/internal/handler` | 0% | 29.0% |

新增测试文件：`service/validation_test.go`、`repository/timezone_integration_test.go`、`repository/maintenance_integration_test.go`、`handler/management/binding_test.go`，并在 `handler/server_integration_test.go` 追加 `TestManagementBindingRejectsInvalidInputBeforeService`。

仍存在的缺口（诚实记录）：

- `service` 与 `repository` 的剩余未覆盖部分集中在需要复杂业务夹具的跨模块用例（成员生命周期并发、角色委派边界、组织并发版本冲突）。这些不是「没有测试」，而是需要逐条设计隔离数据；按本计划判定表属可接受残留，不应为凑覆盖率写无意义断言。
- `management` 包的响应映射（状态码与响应体）不在包内测试，因为它依赖 `handler.Server.Errors`，由 `handler` 包集成测试覆盖；包内单测只覆盖它真正拥有的绑定契约。
- `iam/internal/router` 未单独建测试文件。路由注册由 `handler` 包的集成测试经真实 HTTP 覆盖（请求确实穿过路由表），按判定表视为已覆盖，但无独立文件。
- 集成测试需要外部 PostgreSQL 与 Redis；Docker 不可用时须显式提供连接串，否则跳过。

## 风险与约束

- 工作量集中在第三组。允许分批，但每批必须自洽可验证，不得留下半套测试。
- 补齐过程中若发现实现本身有缺陷，按「只修本次测试暴露的问题」处理，不做顺带重构。
- 不为达到覆盖率数字而写无意义断言。
