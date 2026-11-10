.DEFAULT_GOAL := dev

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
	@go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./src/...

vuln-go:
	@go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./src/...

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
