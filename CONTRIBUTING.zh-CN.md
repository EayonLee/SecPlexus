# 参与贡献 SecPlexus

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh-CN.md)

欢迎为 SecPlexus 贡献代码、缺陷报告、测试、文档和体验改进。本指南说明如何准备符合项目架构、便于审查和验证的变更。

任何人都可以创建 Issue、提交 Pull Request（PR）和参与代码审查。贡献不需要仓库写权限：Fork 仓库后，在自己的工作分支上修改，再向本仓库的 `main` 分支提交 PR 即可。

## 开始之前

先查看[已有 Issue](https://github.com/EayonLee/SecPlexus/issues) 和
[Pull Request](https://github.com/EayonLee/SecPlexus/pulls)，避免重复工作。每次贡献聚焦一个问题。

- 报告缺陷时，提供代码版本、运行环境、复现步骤、预期与实际行为，以及脱敏后的相关日志。界面问题可附截图。
- 提议功能时，说明用户问题、预期行为、涉及模块，以及兼容性或数据变化。投入较大工作前先讨论方案。
- 小修复和文档纠错可以直接提交 PR。

发现疑似安全漏洞时，优先使用 GitHub 私密漏洞报告功能（如已启用）。若不可用，先向维护者询问私密报告渠道，不在公开 Issue 中披露利用细节、凭证或受影响的私有数据。

## 了解代码结构

SecPlexus 当前在同一仓库内维护两个 Go 服务和一个 React 应用。PostgreSQL 存储 IAM 数据与会话，Redis 用于限流。

| 位置 | 职责 |
| --- | --- |
| [`src/apps/iam/`](src/apps/iam/README.md) | 账号、认证、会话、组织、角色、授权与身份审计 |
| [`src/apps/console/`](src/apps/console/README.md) | 业务服务入口；当前提供组织上下文与带授权检查的 SSE 连接 |
| [`src/apps/web/`](src/apps/web/README.md) | Next.js、React、TypeScript、Tailwind CSS、shadcn/ui；统一应用框架与产品页面 |
| [`src/packages/`](src/packages/README.md) | 应用之间实际复用的实现 |
| `tests/` 和 `src/apps/web/tests/` | 集成测试运行器与 Web 浏览器 E2E 测试 |
| `scripts/` 和 `deploy/` | 开发、构建、测试与部署工具 |
| [`docs/plans/`](docs/plans/README.md) | 需求、实施计划、审核状态与验证证据 |

先读受影响模块的 README 和已有实现。[架构文档](docs/architecture/README.md) 中保留了早期平台设想，请注意版本说明；当前实现以模块文档和已批准的规格为准。Console 尚未实现的业务工作流，以及界面演示页面，不能当作已交付的产品能力。

## 准备开发环境

工具版本以 `go.mod`、`package.json` 和 CI 工作流为依据：

| 工具 | 版本或要求 |
| --- | --- |
| Go | 1.27.3 |
| Node.js | 22.12 及以上；建议使用 CI 采用的 24 |
| pnpm | 10.32.1 |
| Make 与 OpenSSL | 根目录开发命令与本地 TLS 证书生成 |
| PostgreSQL 与 Redis | 可访问的开发实例；仓库 Compose 使用 PostgreSQL 16 与 Redis 7 |
| Docker 与 Compose | 选择容器依赖或容器测试时需要 |

先将仓库 Fork 到自己的 GitHub 账号，再克隆 Fork，并将项目仓库添加为 `upstream`。将 `YOUR_USERNAME` 替换为你的 GitHub 用户名：

```sh
git clone https://github.com/YOUR_USERNAME/SecPlexus.git
cd SecPlexus
git remote add upstream https://github.com/EayonLee/SecPlexus.git
```

已有本地仓库时可以继续使用；有仓库写权限的协作者也可以在项目仓库内创建工作分支。以下命令均在仓库根目录执行；已有 `.env` 时保留原文件。

```sh
pnpm install --frozen-lockfile --ignore-scripts
test -e .env || cp .env.example .env
pnpm certs:generate
```

在 `.env` 中填写开发数据库连接、初始管理员密码、CSRF 密钥和内部服务凭证，字段要求与 TLS 配置见[配置指南](docs/configuration.md)。不要将 `.env`、证书、私钥和日志纳入版本控制。

可以连接已有的开发 PostgreSQL 与 Redis，也可以显式创建仓库提供的依赖：

```sh
pnpm compose:up postgres redis
```

随后启动应用：

```sh
make dev
```

`make dev` 安装前端依赖、执行 IAM 迁移，并启动支持热更新的 IAM、Console 和 Web。它按配置连接基础设施，不启动 Docker，请使用开发数据库。空账号库会初始化管理员 `admin`，密码使用配置中的初始密码，默认为 `secplexus`；登录后可在账号设置中主动修改。

| 命令 | 用途 |
| --- | --- |
| `make build` | 构建两个 Go 服务和 Web，不要求 `.env` |
| `pnpm preview` | 启动后端并预览已构建的 Web；公共 origin 需与实际访问地址一致 |
| `pnpm compose:up` | 显式构建并部署整套 Compose 服务 |
| `make stop` | 停止当前工作区应用与 Compose 容器，保留数据 |

## 设计与实现

新增功能、重要设计、API 契约变更和数据库迁移，需要建立
`docs/plans/<request-slug>/spec.md` 与 `plan.md`，并更新[计划索引](docs/plans/README.md)。
Spec 说明行为、边界、兼容性与验收标准；Plan 记录实施方案、任务与验证方式。获得维护者批准后再实施，小修复不需要单独建计划。

### 服务边界

各模块拥有自己的配置、启动流程和数据库表。模块之间通过公开契约通信，不导入其他应用的私有包，也不读取其他模块的表。Console 通过经过认证的 IAM 内部 API 获取身份和授权。

| Go 代码位置 | 职责 |
| --- | --- |
| `cmd/` | 启动参数与入口 |
| `internal/config/` | 配置加载、环境变量展开与校验 |
| `internal/bootstrap/` | 依赖创建、组装、启动与清理 |
| `internal/router/` | 路由注册与中间件组合 |
| `internal/handler/` | 请求绑定、调用用例与输出响应 |
| `internal/service/` | 业务规则、授权范围、事务与审计顺序 |
| `internal/repository/` | 数据访问与事务内操作 |

只在配置加载阶段读取环境变量，依赖显式传递。配置加载不能建立网络连接或执行迁移。共享包只承载真实的跨应用复用。版本化迁移放在所属模块，保留已经执行的迁移脚本。

### API 契约与授权

公开 API 使用 `/api/v1/{module}/...`；内部 API 使用
`/internal/v1/{module}/...`，并保持认证与隔离。

1. 修改所属模块的原生 Go DTO、`internal/router` Gin 路由及 Handler，同时更新模块说明和真实调用方。
2. 请求绑定使用 `httpx`，公共能力使用共享 HTTP 中间件，普通 JSON 使用 `result`；保留真实状态、空响应和流式语义。
3. Web 在 `src/spx/api/iam/schemas.ts` 维护实际消费的 Zod 模型，类型由模型推导，模块 API 使用 Axios。字段同步通过真实 HTTP 和浏览器测试验证。
4. 仅在修改跨语言结果码或权限目录时执行 `pnpm catalogs:generate`、`pnpm catalogs:check`。业务源归 Go `result` 和 IAM `authorization`；不手改 `.gen.*` 导出。

服务端授权始终是最终依据。保留组织范围、CSRF 防护、会话撤销，以及事务和审计保证；页面或按钮可见不代表 API 已授权。默认保持同源访问，跨域凭证访问需要明确设计。

### 代码与文档风格

遵循 [AGENTS.md](AGENTS.md) 和[项目代码规范](.agents/skills/spx-mode/SKILL.md)。

- 保持变更聚焦，不混入无关格式化、依赖升级或推测性抽象。在所属边界校验未知输入，向内传递类型化值；保留授权与当前状态检查。
- 修改的 Go 文件运行 `gofmt`，使用 `goimports` 整理导入。导出声明使用以名称开头的中文注释，数据库列采用 `snake_case`。
- TypeScript 优先使用类型推导、具名导入、早返回和边界上的明确类型，避免 `any` 与无必要的包装层。Web 使用 Biome。
- 保留项目组件、样式与主题基准，遵循官方 [shadcn Skill](.agents/skills/shadcn/SKILL.md)，在新增文件中实现项目功能，检查实际交互和响应式布局。产品路由、按需模板接入和 next-intl 国际化约定见 Web README；中英文词典同步维护，采用专业术语、ICU消息和国际化格式工具，不在组件中按语言分支。
- 行为变化时更新模块 README；成对的中英文文档保持一致。保留上游许可证与署名，不写入密钥、私有项目细节或外部工作区路径。

## 验证变更

行为变更从一个聚焦的失败测试开始，选择能可靠保护该行为的最低层：单元测试覆盖规则和边界，集成测试覆盖真实组件契约，E2E 覆盖核心用户流程与访问边界。隔离、清理和定向执行方式见[测试指南](docs/testing.md)。

在仓库根目录执行与变更相关的检查：

```sh
pnpm catalogs:check
gofmt -l src
go vet ./src/...
pnpm lint
pnpm test:unit
pnpm typecheck
make build
```

`gofmt -l src` 应无路径输出。修改契约时，还需重新生成并确认产物与源定义一致。
[CI 工作流](.github/workflows/verify.yml) 包含具体的一致性检查和依赖漏洞扫描命令。

涉及持久化、服务集成、认证、授权或核心浏览器流程时，运行适用的测试套件：

```sh
pnpm test:integration --docker --race
pnpm --dir src/apps/web exec playwright install chromium
pnpm test:e2e --docker
```

这些 `--docker` 命令创建隔离的测试容器，并清理自己拥有的资源。也支持外部测试连接，但必须使用专用测试库，并遵守测试指南中的权限与清理要求。测试不能连接开发或生产数据。依赖就绪和结果断言使用有界条件等待，不用固定 sleep 或重试掩盖失败。

纯文档变更执行与编辑内容相关的链接、命令和内容核对即可。交付时说明实际执行的检查、结果及未验证范围；未执行不能写成通过。

## 提交 Pull Request

1. 从项目最新的 `main` 创建工作分支。
2. 完成一个聚焦的变更，并执行与之相关的检查。
3. 提交变更，将工作分支推送到自己的 Fork。
4. 创建 PR，目标仓库与分支选择 `EayonLee/SecPlexus:main`，来源选择自己的工作分支。尚未完成或希望提前讨论时，可以创建 Draft PR。
5. 根据审查意见继续修改，将后续提交推送到同一分支，已有 PR 会自动更新。

如果已经按照前文配置了 `upstream`，可以这样创建工作分支：

```sh
git fetch upstream
git switch -c session-expiry upstream/main
```

新分支名称使用最多三个由连字符连接的小写英文单词，例如`iam-login`、`contribution-guide`，不使用斜杠或类型前缀。不要求每次变更都创建Issue。

提交消息和 PR 标题使用 `type(scope): summary`，scope 可省略。类型包括
`feat`、`fix`、`docs`、`chore`、`refactor` 和 `test`，例如：
`fix(iam): revoke sessions after password reset`。

PR 描述说明问题、变更后的行为、审查所需的设计决策和实际验证结果，按需关联 Issue 与已批准的计划。UI 变更附截图，涉及迁移或兼容性时说明影响。审查讨论保持具体、尊重事实。

将 PR 标记为可审查之前，请确认：

- [ ] 变更解决一个明确问题，并符合已批准的范围。
- [ ] 受影响的契约、生成客户端、迁移与文档已同步。
- [ ] 相关测试和检查通过；跳过或无法执行的项目已说明。
- [ ] UI 变更已在浏览器检查受影响的交互与布局。
- [ ] 未包含凭证、敏感日志、私有数据或无关文件。

## 代码审查与合并

欢迎社区成员参与 PR 审查、提出问题、验证变更和提供改进建议。作者应在 PR 中说明设计理由并回应反馈，让后续贡献者也能了解讨论过程。

项目维护者负责最终接收和合并，将变更合入受保护的 `main` 分支。Approve 表示审查者认可当前变更，不会赋予合并权限。通过 Fork 贡献或创建 PR 均不需要合并权限。

如果 PR 存在冲突，将项目最新的 `main` 更新到自己的工作分支，并在本地解决。PR 合并后，可以删除工作分支，同步 Fork 后继续下一次贡献。

使用 AI 辅助开发的贡献遵循相同流程。提交者需要理解所提交的变更，检查生成的代码，并验证所报告的测试结果。

### 框架接入

使用Gin原生Handler/中间件与模块自有DTO。IAM凭证用例接收类型化值，Argon2id、SCS、CSRF和Redis limiter分别负责机制；数据使用GORM投影/事务与Goose迁移，配置/命令使用Viper、mapstructure和Cobra。参照[实际角色管理链路](docs/development/role-management.md)，已有框架能力不要再套一层通用工具或影子路由。

Web使用Axios模块API和Zod推导类型。Query管理服务端状态，RHF和Zod管理表单；密码操作不进入MutationCache。保留模板组件/样式及next-intl词典。执行`make lint-go`、`make vuln-go`和[测试指南](docs/testing.md)中的相关检查；均为本地检查，不是部署或CI流程。
