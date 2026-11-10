# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project intends
to follow [Semantic Versioning](https://semver.org/) after 1.0.

The project has not published a tagged release yet. Entries below describe the
current `main` branch.

## [Unreleased]

### Added

- IAM: login, revocable logical sessions, voluntary password change, administrator
  password reset, users, global roles, flat permissions, authorization and
  append-only audit.
- IAM: versioned Goose migrations with a single accounts baseline, table and
  column comments, and typed constants for every closed value set.
- Console: identity context and authorized SSE connections.
- Web: real IAM pages for login, profile, users, roles and audit; English and
  Simplified Chinese; same-origin API proxy.
- Cross-platform development entry: `make dev`, `make test`, `make build` and
  `make stop` on macOS, Linux and Windows, with process-tree cleanup.
- Repository standards: database design and backend structure guides, issue and
  pull request templates, CI, Dependabot and community health files.

### Changed

- Authorization is flat: users hold roles, roles hold permissions. Organization,
  membership and scope concepts are removed from the current model.
- Audit listing uses `(occurred_at,id)` keyset pagination instead of deep OFFSET.
- Service use cases accept their own typed inputs and no longer depend on HTTP
  DTOs.
- Log messages use plain English and stable event names.

### Fixed

- Leaked pnpm/Next process trees on Windows when `make dev` stopped; ports are
  released and the next start is not blocked by a stale lock.
- Web proxy turned an upstream 401 into 503 when the request body was streamed;
  bodies are buffered and upstream statuses pass through.
- `make config` could fail after a successful check when Windows still held a
  temporary build directory; cleanup is now best effort.
- Deprecated permission status handling and stale authorization tests were
  aligned with the `active/retired` model.

### Security

- Passwords, tokens, cookies, CSRF values and request bodies are excluded from
  logs and audit metadata.
- Audit rows reject updates at the database level; retention deletion requires
  an explicit transaction flag.
