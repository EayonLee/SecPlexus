.DEFAULT_GOAL := dev

# 工具版本与工具链集中在这里；升级工具是独立 PR，不要分散到各 target。
#
# 环境事实：本机安装的 go 可能是 1.24.x，而 go.mod 声明 go 1.27.3，平时靠 GOTOOLCHAIN=auto 下载。
# golangci-lint 要求"构建它的 Go 版本 >= 被分析模块的 Go 版本"，否则报
# "Go language version used to build golangci-lint is lower than the targeted Go version"。
# 所以这里显式指定同一工具链，不依赖 auto 选择。
GOTOOLCHAIN := go1.27.3
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION := v1.9.0


.PHONY: dev build stop config preview compose-up test test-e2e install-browsers integration typecheck lint lint-go vuln-go vet catalogs catalogs-generate security

dev:
	@node scripts/start.mjs $(MODULES)

build:
	@pnpm build

stop:
	@node scripts/stop.mjs

config:
	@pnpm config:check $(MODULES)

preview:
	@node scripts/start.mjs preview $(MODULES)

compose-up:
	@pnpm compose:up $(SERVICES)

test:
	@pnpm test:unit

test-e2e:
	@pnpm test:e2e $(ARGS)

install-browsers:
	@pnpm --dir src/apps/web exec playwright install chromium

integration:
	@pnpm test:integration $(ARGS)

typecheck:
	@pnpm typecheck

lint:
	@pnpm lint

lint-go:
	@GOTOOLCHAIN=$(GOTOOLCHAIN) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./src/...

vuln-go:
	@GOTOOLCHAIN=$(GOTOOLCHAIN) go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./src/...

vet:
	@go vet ./src/...

catalogs:
	@pnpm catalogs:check

catalogs-generate:
	@pnpm catalogs:generate

# 供应链与密钥闸门，与 CI 的 security job 使用同一组工具版本。
# 工作流本身的静态分析（zizmor）只在 CI 运行：本地可用 `uvx zizmor .`。
security:
	@go run github.com/zricethezav/gitleaks/v8@v8.28.0 dir . --config .gitleaks.toml --redact --no-banner
	@go run github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.7.1 scan source --lockfile=go.mod --lockfile=pnpm-lock.yaml --config osv-scanner.toml
