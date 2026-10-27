.DEFAULT_GOAL := dev

.PHONY: dev build stop

dev:
	@sh scripts/dev/start.sh

build:
	@pnpm build

stop:
	@node scripts/stop.mjs
