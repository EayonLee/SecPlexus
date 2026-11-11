# 参与贡献 SecPlexus

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh-CN.md)

欢迎为 SecPlexus 贡献代码、测试、文档、可复现的缺陷报告和体验改进。本指南说明
如何准备符合项目架构、便于审查和验证的变更。

参与即表示同意遵守[行为准则](CODE_OF_CONDUCT.md)。发现疑似漏洞请使用
[SECURITY.md](SECURITY.md)中的私密渠道，不要开公开 Issue。

## 开始之前

先搜索[已有 Issue](https://github.com/EayonLee/SecPlexus/issues) 和
[Pull Request](https://github.com/EayonLee/SecPlexus/pulls)，避免重复工作。
每次贡献聚焦一个问题。

- 缺陷报告：版本、环境、精确复现步骤、预期与实际行为、脱敏日志。界面问题可附截图。
- 功能建议：用户问题、预期行为、涉及模块、兼容性或数据影响。投入较大工作前先讨论。
- 小修复和文档纠错可以直接提交 PR。
- 安全报告：[GitHub 私密漏洞报告](https://github.com/EayonLee/SecPlexus/security/advisories/new)。

## 了解代码结构

SecPlexus 在同一仓库内维护两个 Go 服务和一个 Next.js 应用。PostgreSQL 存储 IAM
数据与会话，Redis 用于登录限流。

| 位置 | 职责 |
| --- | --- |
| [`src/apps/iam/`](src/apps/iam/README.md) | 账号、认证、会话、用户、角色、扁平权限、授权与审计 |
| [`src/apps/console/`](src/apps/console/README.md) | 运营入口；当前提供身份上下文与带授权的 SSE 连接 |
| [`src/apps/web/`](src/apps/web/README.md) | Next.js、React、TypeScript、Tailwind CSS、shadcn/ui；应用框架与产品页面 |
| [`src/packages/go/`](src/packages/README.md) | 应用之间真正复用的代码 |
| `scripts/` 与 `deploy/` | 开发、构建、测试与部署工具 |
| [`docs/plans/`](docs/plans/README.md) | 需求、实施计划、审核状态与验证证据 |

先读受影响模块的 README 和当前实现。模块文档与已批准的规格是当前事实来源。
Console 未实现的业务工作流和演示页面不是已交付能力。

架构、数据库与分层规则：

- [架构](docs/design/architecture/README.md)
- [数据库设计](docs/development/database-design.md)
- [后端工程结构](docs/development/backend-structure.md)
- [AGENTS.md](AGENTS.md)

## 开发环境

| 工具 | 版本或要求 |
| --- | --- |
| Go | 1.27.4 |
| Node.js | 22.12 及以上；建议 24 |
| pnpm | 10.32.1 |
| GNU Make | 三个平台执行根命令都需要 |
| PostgreSQL / Redis | 可访问的实例，或仓库提供的 Compose |
| Docker 与 Compose | 仅用于容器依赖或容器测试 |

Windows 可通过 w64devkit、Chocolatey、MSYS2 等安装 GNU Make，然后在 PowerShell
或 `cmd.exe` 中运行 `make`；不需要 POSIX shell。

Fork 仓库，克隆自己的 Fork，并把项目仓库添加为 `upstream`：

```sh
git clone https://github.com/YOUR_USERNAME/SecPlexus.git
cd SecPlexus
git remote add upstream https://github.com/EayonLee/SecPlexus.git
```

先生成配置并检查，再启动：

```sh
make config
```

`make config` 首次运行会从模板生成 `.env` 并停止，便于检查。填写 PostgreSQL/Redis
连接、配置托管管理员密码、CSRF 密钥和 Console 服务凭证。字段要求见
[配置指南](docs/development/configuration.md)；不要将 `.env`、凭据和日志提交到仓库。

可以连接已有 PostgreSQL 与 Redis，也可以启动仓库提供的依赖：

```sh
make compose-up SERVICES="postgres redis"
make dev
```

`make config` 离线校验全部选中模块，不连接数据库、不迁移、不创建业务日志。
`make dev` 安装 Web 依赖、校验配置、执行 IAM 迁移，并启动支持热更新的 IAM、
Console 与 Web；它只连接配置的基础设施，不启动 Docker。

配置托管管理员以配置为准：修改 `SPX_IAM_BOOTSTRAP_USERNAME` /
`SPX_IAM_BOOTSTRAP_PASSWORD` 后重启全部 IAM 副本；页面和重置命令不能修改该账号。
普通账号可以自助改密。

| 命令 | 用途 |
| --- | --- |
| `make build` | 构建两个 Go 服务和 Web，不要求 `.env` |
| `make preview` | 启动后端并预览已构建的 Web；公共 origin 需与实际地址一致 |
| `make compose-up` | 构建并部署整套 Compose 服务 |
| `make stop` | 停止当前工作区应用与容器，保留数据 |
| `make test` | 运行脚本、Go 与 Web 单元测试 |

## 设计与实现

新增功能、重要设计、API 契约变更和数据库迁移需要
`docs/plans/<request-slug>/spec.md` 与 `plan.md`，并更新[计划索引](docs/plans/README.md)。
Spec 说明行为、边界、兼容性与验收标准；Plan 记录方案、任务与验证。获得维护者
批准后再实施；小修复不需要单独计划。

### 服务边界

各模块拥有自己的配置、启动流程和数据库表。模块之间通过公开契约通信，不导入
其他应用的私有包，也不读取其他模块的表。Console 通过经过认证的 IAM API 获取
身份和授权。

| Go 代码位置 | 职责 |
| --- | --- |
| `cmd/` | 启动参数与进程入口 |
| `internal/config/` | 配置加载、展开与校验 |
| `internal/bootstrap/` | 依赖创建、组装、启动与清理 |
| `internal/router/` | 路由注册与中间件组合 |
| `internal/handler/` | 请求绑定、DTO 映射与响应 |
| `internal/service/` | 用例、授权、事务与审计顺序 |
| `internal/repository/` | 数据访问与事务内操作 |

只在配置加载阶段读取环境变量，依赖显式传递。共享包只承载真正的跨应用复用。
版本化迁移放在所属模块，已经执行的迁移不得修改。

### API 契约与授权

全部 API 使用 `/api/v1/{module}/...`，每个服务一个监听。路由明确声明用户会话、
服务凭证或公开访问；服务鉴权还要验证原用户会话及权限。外部 HTTPS 由反向代理终止。

1. 修改所属模块的原生 Go DTO、`internal/router` Gin 路由和 Handler，同时更新模块
   文档与真实调用方。
2. 请求绑定使用 `httpx`，公共能力使用共享 HTTP 中间件，普通 JSON 使用 `result`；
   保留真实状态码、空响应和流式语义。
3. Web 在 `src/spx/api/iam/schemas.ts` 只声明实际消费的 Zod 模型，类型由模型推导，
   模块 API 使用 Axios；用真实 HTTP 和浏览器测试验证。
4. 跨语言结果码或权限目录变更只能通过其 Go 源执行：`make catalogs-generate` 与
   `make catalogs`；不要手改 `.gen.*` 导出。

服务端授权始终是最终依据。保留路由声明的权限校验、CSRF 防护、会话撤销，以及事务
和审计保证；页面可见不代表 API 已授权。默认保持同源访问，跨域凭证访问需要明确设计。

### 代码与文档风格

遵循 [AGENTS.md](AGENTS.md)、[数据库设计](docs/development/database-design.md)、
[后端工程结构](docs/development/backend-structure.md)和[项目代码规范](.agents/skills/spx-mode/SKILL.md)。

- 保持变更聚焦，不混入无关格式化、依赖升级或推测性抽象。在所属边界校验未知输入，
  向内传递类型化值；保留授权与当前状态检查。
- 封闭取值使用 `CHECK` 约束和模块 `internal/enum` 常量；比较常量，不写字面量。
- 使用 `logging.From(ctx)` 和简单英文日志；禁止记录密码、令牌、Cookie、CSRF 值、
  请求体或私有数据。
- 修改的 Go 文件运行 `gofmt`。导出声明使用以名称开头的中文注释，数据库列使用
  `snake_case`。
- TypeScript 优先类型推导、具名导入、早返回和边界明确类型，避免 `any` 与无必要包装。
  Web 使用 Biome。
- 保留组件、样式与主题基准，遵循 [shadcn Skill](.agents/skills/shadcn/SKILL.md)，检查
  实际交互和响应式布局，并同步维护 `en` / `zh-CN` next-intl 词典。
- 行为变化时更新模块 README；成对的中英文文档保持一致。保留上游许可证与署名，
  不写入密钥、私有项目细节或外部工作区路径。

## 验证变更

行为变更从一个聚焦的失败测试开始。选择能可靠保护该行为的最低层：单元测试覆盖规则
和边界，集成测试覆盖真实组件契约，E2E 覆盖核心用户流程与访问边界。隔离、清理和
定向执行见[测试指南](docs/development/testing.md)。

在仓库根目录执行：

```sh
make test
make vet
make lint
make typecheck
make catalogs
make build
gofmt -l src
```

`gofmt -l src` 必须没有输出。修改契约时重新生成并确认产物与源定义一致。
`make vuln-go` 执行 Go 依赖漏洞扫描。GitHub Actions 会在 Linux、macOS、Windows
运行同一套 `make test`，并执行静态检查和生产构建。

涉及持久化、服务集成、认证、授权或核心浏览器流程时，运行适用的套件：

```sh
make integration ARGS="--docker --race"
make install-browsers
make test-e2e ARGS="--docker"
```

`--docker` 命令创建隔离测试容器并清理自己拥有的资源。也支持外部测试连接，但必须
使用专用测试库，并遵守测试指南中的权限与清理要求。测试不能连接开发或生产数据。
依赖就绪和结果断言使用有界条件等待，不用固定 sleep 或重试掩盖失败。

纯文档变更执行与编辑内容相关的链接、命令和内容核对。交付时说明实际执行的检查、
结果及未验证范围；未执行不能写成通过。

## 提交 Pull Request

1. 从项目最新的 `main` 创建工作分支。
2. 完成一个聚焦的变更，并执行相关检查。
3. 提交变更，将工作分支推送到自己的 Fork。
4. 向 `EayonLee/SecPlexus:main` 创建 PR；需要提前反馈时使用 Draft PR。
5. 后续提交推送到同一分支，已有 PR 会自动更新。

```sh
git fetch upstream
git switch -c session-expiry upstream/main
```

分支名使用最多三个由连字符连接的小写英文单词，例如 `iam-login`、
`contribution-guide`；不使用斜杠或类型前缀。不要求每次变更都创建 Issue。

提交消息和 PR 标题使用 `type(scope): summary`，scope 可省略。类型包括 `feat`、
`fix`、`docs`、`chore`、`refactor` 和 `test`。例如：
`fix(iam): revoke sessions after password reset`。

PR 描述说明问题、变更后的行为、审查所需的设计决策和实际验证结果，按需关联 Issue
与已批准计划。UI 变更附截图，涉及迁移或兼容性时说明影响。
[PR 模板](.github/PULL_REQUEST_TEMPLATE.md)列出了必需部分。

将 PR 标记为可审查之前：

- [ ] 变更解决一个明确问题，并符合已批准的范围。
- [ ] 受影响的契约、生成客户端、迁移与文档已同步。
- [ ] 相关测试和检查通过；跳过或无法执行的项目已说明。
- [ ] UI 变更已在浏览器检查受影响的交互与布局。
- [ ] 未包含凭证、敏感日志、私有数据或无关文件。

## 代码审查与合并

欢迎社区成员参与 PR 审查、提出问题、验证变更和提供改进建议。作者应在 PR 中说明
设计理由并回应反馈，让后续贡献者也能了解讨论过程。

项目维护者负责最终接收和合并，将变更合入受保护的 `main` 分支；决策与人员见
[GOVERNANCE.md](GOVERNANCE.md) 和 [MAINTAINERS.md](MAINTAINERS.md)。Approve 表示
审查者认可当前变更，不赋予合并权限。通过 Fork 贡献或创建 PR 均不需要合并权限。

如果 PR 存在冲突，将项目最新 `main` 更新到工作分支并在本地解决。合并后可以删除
工作分支，同步 Fork 后继续下一次贡献。

使用 AI 辅助开发的贡献遵循相同流程。提交者需要理解所提交的变更，检查生成代码，
并验证所报告的测试结果。

### 框架接入

使用 Gin 原生 Handler/中间件与模块自有 DTO。IAM 凭证用例接收类型化值；
Argon2id、SCS、CSRF 和 Redis limiter 分别负责机制；数据使用 sqlc 查询与显式事务，并配合
Goose 迁移，配置/命令使用 Viper、mapstructure 和 Cobra。参照
[实际角色管理链路](docs/development/role-management.md)，已有框架能力不要再套一层
通用工具或影子路由。

Web 使用 Axios 模块 API 和 Zod 推导类型。Query 管理服务端状态，React Hook Form 和
Zod 管理表单；密码操作不进入 MutationCache。保留模板组件、样式和 next-intl 词典。
