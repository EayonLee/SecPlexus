.DEFAULT_GOAL := dev

.PHONY: dev build stop config preview compose-up test test-e2e install-browsers integration typecheck lint lint-go vuln-go vet catalogs catalogs-generate

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
