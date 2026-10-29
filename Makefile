.DEFAULT_GOAL := dev

.PHONY: dev build stop

dev:
	@sh scripts/dev/start.sh

build:
	@pnpm build

stop:
	@node scripts/stop.mjs

# Local checks only; toolchain and tool versions are pinned.
.PHONY: lint-go vuln-go
lint-go:
	@GOTOOLCHAIN=go1.27.3 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./src/...

vuln-go:
	@GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./src/...
