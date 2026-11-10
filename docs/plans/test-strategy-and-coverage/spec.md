# 测试规范固化与存量测试补齐

状态：待审核，未实施。范围仅测试规范与存量测试补齐；不改动生产行为。

## 背景

用户提出两点：一是 `AGENTS.md` 没有定义「开发需求时该写单元测试、集成测试还是 E2E」，希望统一；二是既有功能中没有测试的部分要全部补齐。方向是测试驱动开发。

## 现状核实

已有基础（不需要重建）：

- `docs/development/testing.md` 存在且内容完整：三层职责表、缺陷修复验证规则、目录约定（Go 单元测试随包、`*_integration_test.go` 加 `integration` 构建标签、Web 单元测试在 `tests/unit/`、E2E 在 `tests/e2e/` 用 Playwright）、隔离与清理要求、可执行入口。
- `AGENTS.md` 第 49 行已指向 `docs/development/testing.md`。
- `CONTRIBUTING.md` 已有「先从一个聚焦的失败测试开始」以及按行为选择最低可靠层次的表述。
- `README.md`、`CONTRIBUTING.zh-CN.md`、各模块 README 均引用 `docs/development/testing.md`，入口没有孤立问题。

真实缺口（本计划要解决的）：

1. 规范散在 `docs/development/testing.md` 与 `CONTRIBUTING.md`，但没有「什么类型的改动必须具备哪一层的测试」的强制对应表，导致同一句原则可以被解释成不同做法。
2. `AGENTS.md` 只有一句话概括，没有可执行的最低要求，也没有禁止清单。
3. 存量代码存在整包无测试：`console/internal/handler`、`console/internal/bootstrap`、`console/internal/router`、`iam/internal/router`、`iam/internal/handler/management` 完全没有测试文件。
4. `iam/internal/service`（12 个源文件）仅有 1 个集成测试；`iam/internal/repository`（17 个源文件）仅 3 个测试。
5. 没有覆盖率基线，无法判断「补齐」到什么程度算完成。

## 参考依据

AiSOC 的 `apps/docs/docs/contributing/guidelines.md` 把自己定义为三层强制检查，任何一层失败都不能合并：①lint 与类型检查；②单元与集成测试（新代码必须带新测试，新连接器至少需要 schema 契约、`normalize()`、`test_connection` 与 `fetch_alerts` 测试）；③触及 agent 相关代码必须重跑评测基线并给出前后差异。

Superpowers 的 `test-driven-development` 技能给出核心法则：**没有失败的测试就不写生产代码**；写了代码才补测试，则删除重写；并明确 Red-Green-Refactor 循环与「验证测试确实失败」这一前置动作。

本计划融合两者，但适配本仓库现状：不引入评测基线这类本仓库没有的能力，不照搬工具链。

## 范围

1. 在 `docs/development/testing.md` 增加「改动类型与必需测试层」对应表，把现行原则变成可判定条目。
2. 在 `AGENTS.md` 增加强制最低要求、禁止清单与 TDD 要求，保持简短并继续指向 `docs/development/testing.md`。
3. 建立覆盖率基线并记录，作为补齐的完成判据。
4. 按对应表补齐存量缺口。

明确不做：

- 不追求行覆盖率数字本身，不设「必须 80%」这类无差别门槛。判据是「每个对外行为至少有一个能失败的测试」。
- 不把临时排障脚本、一次性复现步骤转成长期测试（`docs/development/testing.md` 已禁止，继续遵守）。
- 不为纯文案、配置默认值、生成文件补测试。
- 不接入远程 CI，与仓库现行「只做本地检查」一致。

## 规范内容（要写入的判定表）

| 改动类型 | 必须具备 | 不需要 |
|---|---|---|
| 纯函数、校验、状态转换、配置解析优先级 | 单元测试（表驱动覆盖边界与非法输入） | 集成、E2E |
| HTTP 路由新增或权限变更 | 集成测试：真实路由 + 真实服务 + 真实数据库，覆盖允许与拒绝两侧 | 为每个字段做 E2E |
| 数据库迁移、事务、外键、并发不变量 | 集成测试：真实 PG，含迁移前后状态与失败回滚 | Mock 数据库「证明」事务正确 |
| 跨服务认证、服务凭证、会话持久化 | 集成测试：真实组件契约 | 单元测试断言 Mock 返回值 |
| 用户可见的核心旅程、访问边界 | E2E：真实浏览器 + 真实后端 | 逐按钮、逐字段穷举 |
| 前端展示逻辑、回落顺序、格式化 | 单元测试 | E2E |
| 纯样式、文案、颜色令牌 | 不新增测试；由 DESIGN.md 与视觉验收覆盖 | 断言 CSS 类名或文案字符串 |
| 定时任务、重试、超时 | 单元测试覆盖计算逻辑 + 集成测试覆盖真实调度一次 | 依赖 sleep 等待真实时间流逝 |

强制规则：

- 行为变更必须先有失败测试。未观察到失败就写实现，视为未验证。
- 禁止用 sleep 或重试掩盖时序问题；用条件等待（本仓库已有 `condition-based-waiting` 实践）。
- 禁止删除或跳过失败测试来变绿。
- 禁止断言实现细节（内部函数调用次数、私有字段）代替断言可观察行为。
- 集成测试必须使用自有隔离资源并清理，不得触碰开发库。

## 存量补齐清单

按「对外行为」优先，不是按文件补齐：

| 包 | 现状 | 需要补 |
|---|---|---|
| `iam/internal/handler/management` | 6 个源文件，0 测试 | 用户、角色、权限与审计四组接口的允许/拒绝集成测试 |
| `iam/internal/router` | 1 个源文件，0 测试 | 路由注册与鉴权分组的契约测试（可用集成测试覆盖） |
| `iam/internal/service` | 12 源文件，1 集成测试 | 账号、角色、权限、密码、预览的规则单测 + 关键路径集成测试 |
| `iam/internal/repository` | 17 源文件，3 测试 | 查询与锁、维护清理、迁移状态的集成测试 |
| `console/internal/handler` | 2 源文件，0 测试 | SSE 与访问上下文的集成测试 |
| `console/internal/bootstrap` | 1 源文件，0 测试 | 启动装配与配置校验测试 |
| `console/internal/router` | 1 源文件，0 测试 | 路由契约测试 |

## 验收

- `docs/development/testing.md` 含判定表，`AGENTS.md` 含强制最低要求并指向它。
- 覆盖率基线已记录（当前实测：全仓 Go 约 63%，多个包 0%，其中部分 0% 是因为集成测试需要 `SPX_TEST_POSTGRES_URL` 而默认不运行，需在基线中区分）。
- 上表每一行至少有一个能失败的测试，且该测试在移除对应实现后确实失败。
- `pnpm test:unit`、`go test ./src/...`、`pnpm typecheck`、`pnpm lint`、`node scripts/build.mjs` 全部通过。
- 集成测试在有数据库时通过；无数据库时明确跳过而非报错。

## 风险

- 存量补齐工作量大。按「对外行为」而非「文件数」衡量，允许分多次提交，但每次提交必须是完整可验证的一小组。
- 需要区分「真的没测」与「集成测试默认不跑」。已确认 `iam/internal/service`、`iam/internal/authentication` 的 0% 属于后者（缺 `SPX_TEST_POSTGRES_URL`），误判会导致重复劳动。
