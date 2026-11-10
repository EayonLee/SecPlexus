# Go 质量闸门：让 lint、格式化与漏洞检查真正生效

## 背景

仓库里已经有 `make lint-go`、`make vuln-go`、`make vet`，但**闸门实际没有生效**，实测证据如下。

### 1. lint 完全跑不起来

```
$ make lint-go
Error: can't load config: the Go language version (go1.26) used to build golangci-lint
       is lower than the targeted Go version (1.27.3)
```

- `make lint-go` 执行 `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./src/...`；
  v2.15.0 是当前最新版本，但它的 `go.mod`/`toolchain` 要求 go1.26.10，`go run` 因此用
  **go1.26.10 构建**该二进制，而本仓库 `go.mod` 声明 `go 1.27.3`，golangci-lint 拒绝分析更高语言版本的模块
  （官方 FAQ 的既定限制：工具的构建 Go 版本必须不低于被分析模块的 Go 版本）。
- 强行加 `run.go: "1.26"` 规避会崩溃（分析器 goroutine panic，exit 2），**不是可行方案**。
- 实测可行方案：`GOTOOLCHAIN=go1.27.3` 让工具用本仓库要求的工具链构建，此时
  `golangci-lint has version 2.15.0 built with go1.27.3`，正常分析并给出结果。
- 因此 CI 的 quality job 只跑 `go vet ./src/...`（[`ci.yml`](../../../.github/workflows/ci.yml)），
  真正的静态分析缺席，仓库里不存在 `.golangci.yml`。

### 2. 环境事实（容易再次踩坑）

- 机器上安装的 `go` 是 **1.24.13**；模块声明 `go 1.27.3`，平时靠 `GOTOOLCHAIN=auto` 自动下载并使用 1.27.3。
  任何"用 `go run`/`go install` 拉工具"的命令都会继承这个行为，于是出现"工具比自己旧"的冲突。
- `core.autocrlf=true` 且未做本地规范时，工作树是 CRLF；`gofmt -l` 会把**几乎所有文件**报成未格式化
  （实测 111 个），这是假象。用 `git archive` 取 LF 检出后实测只有 **1 个文件**真未格式化：
  `src/apps/iam/internal/config/config.go`。任何本地判断都必须基于 LF 检出。

### 3. 漏洞检查结果（可作为常驻门禁）

`make vuln-go`（govulncheck v1.9.0）实测：

```
No vulnerabilities found.
Your code is affected by 0 vulnerabilities.
（另有 1 个"导入但未调用"、3 个"依赖但未调用"）
```

零噪声、可立即设为门禁。它与 CI 已有的 `osv-scanner` 不是重复：govulncheck 做**调用链可达性**分析
（Go 专有、低误报），osv-scanner 覆盖**锁文件级**（含 `pnpm-lock.yaml`）。

### 4. 参照的行业做法

- GitHub 自己的 Go 仓库用显式配置而非默认集：`cli/cli`（GitHub CLI）的 `.golangci.yml` 为 v2 格式，
  `linters.default: none` + 逐条 `enable`（asasalint、bodyclose、copyloopvar、gocritic、govet、ineffassign、
  nilerr、nolintlint、unused 等），`formatters.enable: [gofmt]`，`govet.settings.enable: [httpresponse]`，
  并在 `exclusions.rules` 里按 `_test.go` 放宽 bodyclose/gosec。
  来源：https://github.com/cli/cli/blob/trunk/.golangci.yml
- 官方 action 支持 `install-mode: goinstall`，用 CI 自己的 Go 工具链从源码构建，
  正好绕开"预编译二进制 Go 版本落后"这个冲突；当前最新为 v9.3.0。
  来源：https://github.com/golangci/golangci-lint-action

## 目标

1. `make lint-go` 在本机与 CI 都能跑，且**在 CI 里是阻断门禁**。
2. 门禁包含三件事：静态分析（golangci-lint）、格式化（gofmt + goimports）、可达漏洞（govulncheck）。
3. 配置显式、可评审：不用"默认集 + 猜"，启用哪些 linter、为什么启用，写在 `.golangci.yml` 与本文档里。
4. 让"以后其他模块照着写就合规"成为默认结果，而不是靠人记。

## 非目标

- 不引入 gosec、revive 的风格类规则、wrapcheck、exhaustruct 等会产生大量既有噪声的 linter；
  需要时由后续计划单独评估（与 `cli/cli` 对 gosec/staticcheck/errcheck 的处理方式一致：先不开，等清理完再开）。
- 不引入覆盖率门禁（当前无 SLA 与趋势消费方，加了只会制造红灯）。
- 不改动业务代码行为；所有修复只做等价改写。

## 契约（门禁语义）

| 项 | 契约 |
|---|---|
| 阻断条件 | `golangci-lint run ./src/...` 非 0；`govulncheck ./src/...` 发现**可达**漏洞非 0 |
| 允许的例外 | 只能通过 `//nolint:<linter> // 原因` 在**单行**豁免，且必须写原因；`nolintlint` 会拒绝无原因/未使用的 nolint |
| 格式 | `gofmt` 与 `goimports` 必须无 diff；仓库 LF 为唯一格式（`.gitattributes` 已声明 `eol=lf`） |
| 版本固定 | 工具版本写进 `Makefile` 与 CI（golangci-lint v2.15.0、govulncheck v1.9.0），升级需单独 PR |
| 本地可复现 | `make lint-go` / `make vuln-go` 与 CI 使用同一版本、同一参数 |
| 工具链 | 任何 `go run/go install` 拉起的工具必须显式 `GOTOOLCHAIN=go1.27.3`（或从 `go.mod` 读取同一版本），不得依赖 `auto` |

## 验收标准

1. `make lint-go` 在 LF 检出上退出码 0；在 CI 上退出码 0。
2. 故意引入一个未检查的错误返回，`make lint-go` 与 CI 都必须失败（门禁有效性验证）。
3. `make vuln-go` 退出码 0，且输出 "Your code is affected by 0 vulnerabilities"。
4. 故意把 `goimports` 顺序打乱，门禁失败（格式化有效性验证）。
5. CI quality job 日志里能看到 golangci-lint 的构建 Go 版本为 1.27.3（证明 install-mode 生效）。
6. 仓库存量问题清零：本计划实施前的 10 个 lint 问题与 1 个格式问题全部修复，**不通过放宽规则绕过**。

## 影响面

- 只影响 Go 工具链与 CI；不改 HTTP 契约、不改数据库、不改前端。
- `Makefile`、`.github/workflows/ci.yml`、新增 `.golangci.yml`；10 处代码修复（1 处格式）。
