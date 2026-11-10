# Go 质量闸门：实施计划

关联 Spec：[spec.md](spec.md)。后续模块的强制规范见本文「模块必须遵守的规范」。

## 一、实测清单（本计划要清零的存量问题）

用候选配置（见下）在 `GOTOOLCHAIN=go1.27.3` 下实测，**共 10 个真实问题 + 1 个格式问题**：

| # | 位置 | linter | 问题 | 修法 |
|---|---|---|---|---|
| 1 | `src/apps/console/internal/bootstrap/serve_test.go:60` | errcheck | `case <-done:` 处未检查错误返回值 | 显式接收并断言 |
| 2 | `src/apps/iam/internal/authorization/policy_test.go:23` | errorlint | `err != SomeErr` 对包装错误不可靠 | 改 `errors.Is` |
| 3 | 同上 `:56` | errorlint | 同上 | 同上 |
| 4 | 同上 `:69` | errorlint | 同上 | 同上 |
| 5 | 同上 `:75` | errorlint | 同上 | 同上 |
| 6 | `src/packages/go/configuration/load.go:40` | errorlint | `err != io.EOF` | `errors.Is(err, io.EOF)` |
| 7 | `src/packages/go/httpx/gin.go:56` | errorlint | 错误比较 | `errors.Is` |
| 8 | `src/apps/iam/internal/handler/server.go:46` | gocritic | 局部变量名 `ReadyCheck` 不该大写 | 改名 |
| 9 | `src/packages/go/httpx/serve.go:23` | noctx | `net.Listen` 未用 `net.ListenConfig` | 改 `(&net.ListenConfig{}).Listen(ctx, ...)` |
| 10 | `src/apps/iam/internal/repository/administrator.go:103` | staticcheck QF1001 | De Morgan 化简 | 等价改写 |
| 11 | `src/apps/iam/internal/config/config.go` | gofmt | 格式未对齐（CRLF 假象之外的**唯一**真实项） | `gofmt -w` |

修复顺序按 linter 分组，便于 review：errorlint（6） → errcheck/gocritic/noctx/staticcheck（4） → gofmt（1）。

## 二、`.golangci.yml`（定稿内容）

参照 `cli/cli` 的 "default: none + 显式清单"，只启用与**正确性和本项目技术面**（HTTP + SQL）相关的 linter。
门禁作用于整个 Go 代码树（`./src/...`）：IAM 与 Console 同时受约束（Console 目前无 SQL，见 [sqlc-data-layer](../sqlc-data-layer/spec.md)）。

```yaml
version: "2"

linters:
  default: none
  enable:
    - bodyclose
    - copyloopvar
    - durationcheck
    - errcheck
    - errorlint
    - gocheckcompilerdirectives
    - gocritic
    - govet
    - ineffassign
    - misspell
    - nilerr
    - noctx
    - nolintlint
    - rowserrcheck
    - sqlclosecheck
    - staticcheck
    - unused
  settings:
    govet:
      enable: [httpresponse]
  exclusions:
    rules:
      # 测试里构造请求不需要 ctx 传播与 body 关闭断言，和 cli/cli 的处理一致。
      - path: _test\.go$
        linters: [bodyclose, noctx]

formatters:
  enable:
    - gofmt
    - goimports

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```

每条启用理由：

| linter | 为什么需要 |
|---|---|
| errcheck / ineffassign / unused / govet / staticcheck | 基础正确性，且已有 2 个真实问题 |
| errorlint | 本项目大量用 `%w` 包装错误（实测 6 个真实问题） |
| bodyclose / noctx | 有 HTTP 客户端与服务端实现 |
| rowserrcheck / sqlclosecheck | 有大量 `rows.Next()`/`rows.Close()` 手写循环（迁移到 sqlc 后仍保留 sqlclosecheck 作为兜底） |
| gocritic | 只开默认检查，能发现命名/冗余类缺陷 |
| misspell | AGENTS 要求日志与事件名为英文，拼写必须能自动拦 |
| nilerr / nolintlint | 防"检查了错误却返回 nil"与随意 `//nolint` |
| copyloopvar / durationcheck / gocheckcompilerdirectives | Go 1.22+ 时代的零成本卫生项，当前 0 问题 |
| formatters: gofmt + goimports | AGENTS 已要求，之前无人执行 |

暂不启用并记录原因：gosec（需逐条 triage 既有告警，`cli/cli` 也选择延后）、revive/godot（中文注释结尾是「。」，
规则不适用）、wrapcheck（当前错误都带上下文，开了是噪声）、exhaustruct/prealloc（无收益）。

## 三、Makefile 改造

现状：

```make
lint-go:
	@go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./src/...

vuln-go:
	@go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./src/...
```

改为显式工具链（并用注释固定环境事实，避免下次再踩）：

```make
# 工具版本集中在这里；升级工具是独立 PR，不要分散到各 target。
# 注意：本机安装的 go 可能是 1.24.x，模块声明 go 1.27.3，平时靠 GOTOOLCHAIN=auto 下载。
# golangci-lint 要求"构建它的 Go 版本 >= 被分析模块的 Go 版本"，所以必须显式指定同一工具链，
# 否则会报 "Go language version used to build golangci-lint is lower than the targeted Go version"。
GOTOOLCHAIN := go1.27.3
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION := v1.9.0

lint-go:
	@GOTOOLCHAIN=$(GOTOOLCHAIN) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./src/...

vuln-go:
	@GOTOOLCHAIN=$(GOTOOLCHAIN) go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./src/...
```

## 四、CI 改造（quality job 增加两步）

在现有 `quality` job（已装 Go 1.27.3、已 checkout）末尾追加：

```yaml
      - name: Go lint
        uses: golangci/golangci-lint-action@<sha> # v9.3.0
        with:
          version: v2.15.0
          # 关键：用 CI 自己的 Go 1.27.3 从源码构建，避免预编译二进制 Go 版本落后于 go.mod。
          install-mode: goinstall
          args: --config .golangci.yml ./src/...

      - name: Go reachable vulnerabilities
        run: make vuln-go
```

说明：

- `@<sha>` 实施时取 `golangci-lint-action` v9.3.0 的 40 位 commit SHA 固定（与仓库其他 action 一致，浮动标签不允许）。
- `install-mode: goinstall` 由官方 action 支持（README「install-mode」一节），`version` 必须写成精确版本
  （`goinstall` 模式不接受 `v2` 这类浮动写法）。
- 首次运行会下载并编译 golangci-lint（约 1–2 分钟）；本项目 test 矩阵在关键路径上，quality job 并行执行，
  不增加流水线总时长。若later 变慢，再加 `actions/cache` 缓存 `~/go/pkg/mod`。
- 不加 `only-new-issues`：存量已清零，全量门禁才有意义。

## 五、任务拆分与依赖

| 任务 | 内容 | 依赖 |
|---|---|---|
| T1 | 修 6 个 errorlint 问题（4 个测试断言 + 2 个包内比较） | — |
| T2 | 修 errcheck / gocritic / noctx / staticcheck 4 项 | — |
| T3 | `gofmt -w src/apps/iam/internal/config/config.go` | — |
| T4 | 新增 `.golangci.yml` | T1–T3（保证新增即绿） |
| T5 | `Makefile` 引入工具链与版本变量 | — |
| T6 | CI quality job 加两步 | T4、T5 |
| T7 | 更新 [AGENTS.md](../../../AGENTS.md) 的"Testing and delivery"：新增"Go 闸门"一小节，指向本计划 | T6 |

先 T1–T3，再 T4–T6 合成一个 PR（可评审：代码修复与工具配置分开成两个提交）。

## 六、验证证据（实施时必须逐条实测并记录）

```bash
# 1. 本地门禁
make lint-go          # 期望：0 issues
make vuln-go          # 期望：No vulnerabilities found / 0 affected
# 2. 门禁有效性（临时引入问题后必须失败，验证完删除）
# 3. LF 检出下的格式检查
git archive HEAD | tar -x -C /tmp/lfx && cd /tmp/lfx && gofmt -l ./src   # 期望：空
```

## 七、风险与回滚

| 风险 | 处理 |
|---|---|
| golangci-lint 与 Go 1.27.3 不兼容（未来升级） | 版本集中在 Makefile；升级单独 PR 并跑全量 |
| `goinstall` 使 CI 变慢 | 先观察一次实际耗时；必要时加模块缓存 |
| 新增 linter 引发大量既有告警 | 已在候选集上实测为 10 个问题并全部修复；其余 linter 明确延后 |
| Windows 本地 CRLF 造成假阳性 | 文档写明"以 LF 检出为准"，并在 CI（Linux）作为判定 |

回滚：单独 `chore(ci)` 提交即可摘除 CI 两步；`.golangci.yml` 与代码修复可独立保留。

## 八、模块必须遵守的规范（新增模块照此执行）

1. 不允许在代码里写 `fmt.Print`/`log.Printf`；统一 `logging.From(ctx)`（misspell/govet 会兜部分，主要靠 review 与 AGENTS）。
2. 新增 HTTP 客户端调用必须传 `ctx`（`noctx`）并关闭 body（`bodyclose`）。
3. 任何 `rows` 循环必须 `defer rows.Close()` 并检查 `rows.Err()`（`sqlclosecheck`/`rowserrcheck`）。
4. 错误比较一律 `errors.Is/As`；包装一律 `%w`（`errorlint`）。
5. 提交前跑 `make lint-go && make vuln-go`；CI 会以同样参数复跑。
6. 需要豁免时只允许单行 `//nolint:<linter> // <原因>`；批量豁免会被 `nolintlint` 拒绝。
