# Contributing to SecPlexus

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh-CN.md)

Thank you for contributing to SecPlexus. Code, tests, documentation,
reproducible bug reports and usability improvements are all welcome. This guide
explains how to prepare a change that fits the project and can be reviewed and
verified.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
For suspected vulnerabilities, use the private channel in [SECURITY.md](SECURITY.md),
never a public issue.

## Before you start

Search [issues](https://github.com/EayonLee/SecPlexus/issues) and
[pull requests](https://github.com/EayonLee/SecPlexus/pulls) first. Keep each
contribution focused on one problem.

- Bug reports: revision, environment, exact reproduction steps, expected and
  actual behavior, and sanitized logs. Screenshots help for UI defects.
- Feature requests: the user problem, proposed behavior, affected modules, and
  compatibility or data impact. Discuss substantial work before implementing it.
- Small fixes and documentation corrections can go straight to a pull request.
- Security reports: [private vulnerability reporting](https://github.com/EayonLee/SecPlexus/security/advisories/new).

## Understand the codebase

SecPlexus is two Go services and one Next.js application in a single repository.
PostgreSQL stores IAM data and sessions; Redis supports login rate limiting.

| Location | Responsibility |
| --- | --- |
| [`src/apps/iam/`](src/apps/iam/README.md) | Accounts, authentication, sessions, users, roles, flat permissions, authorization and audit |
| [`src/apps/console/`](src/apps/console/README.md) | Operations entry point; currently identity context and authorized SSE connections |
| [`src/apps/web/`](src/apps/web/README.md) | Next.js, React, TypeScript, Tailwind CSS and shadcn/ui; application shell and product pages |
| [`src/packages/go/`](src/packages/README.md) | Code genuinely shared across applications |
| `scripts/` and `deploy/` | Development, build, test and deployment tooling |
| [`docs/plans/`](docs/plans/README.md) | Requirements, implementation plans, approval status and validation evidence |

Read the affected module README and current implementation first. Use module
documentation and approved specifications as the source of truth. Console
business workflows and demonstration pages are not delivered capabilities.

Architecture, database and layering rules:

- [Architecture](docs/design/architecture/README.md)
- [Database design](docs/development/database-design.md)
- [Backend structure](docs/development/backend-structure.md)
- [AGENTS.md](AGENTS.md)

## Development setup

| Tool | Version or requirement |
| --- | --- |
| Go | 1.27.3 |
| Node.js | 22.12 or later; 24 recommended |
| pnpm | 10.32.1 |
| GNU Make | Required for root commands on all platforms |
| PostgreSQL / Redis | Reachable instances, or the supplied Compose stack |
| Docker with Compose | Only for container dependencies or container-based tests |

On Windows, install GNU Make (w64devkit, Chocolatey, MSYS2 or equivalent) and run
`make` from PowerShell or `cmd.exe`. No POSIX shell is required.

Fork the repository, clone your fork and add the project as `upstream`:

```sh
git clone https://github.com/YOUR_USERNAME/SecPlexus.git
cd SecPlexus
git remote add upstream https://github.com/EayonLee/SecPlexus.git
```

Create configuration and review it before starting:

```sh
make config
```

`make config` creates `.env` from the template on first use and stops so you can
review it. Set the PostgreSQL/Redis connection, the configured administrator
password, the CSRF key and the Console service token. Follow the
[configuration guide](docs/development/configuration.md) for field requirements;
keep `.env`, credentials and logs out of version control.

Use existing PostgreSQL and Redis instances, or start the supplied ones:

```sh
make compose-up SERVICES="postgres redis"
make dev
```

`make config` validates all selected modules offline: no database connection,
migration or business log. `make dev` installs Web dependencies, validates
configuration, applies IAM migrations and starts IAM, Console and Web with hot
reload. It connects to configured infrastructure and never starts Docker.

The configured administrator is authoritative in configuration. Change
`SPX_IAM_BOOTSTRAP_USERNAME` / `SPX_IAM_BOOTSTRAP_PASSWORD` and restart every IAM
replica; pages and reset commands cannot change it. Ordinary accounts can change
their own password.

| Command | Purpose |
| --- | --- |
| `make build` | Build both Go services and Web; does not require `.env` |
| `make preview` | Start backends and preview the built Web; configure the matching public origin |
| `make compose-up` | Build and deploy the complete Compose stack |
| `make stop` | Stop this checkout's applications and containers; retain data |
| `make test` | Run script, Go and Web unit tests |

## Design and implementation

Features, significant designs, API contract changes and database migrations need
`docs/plans/<request-slug>/spec.md` and `plan.md`, plus an entry in the
[plan index](docs/plans/README.md). The specification records behavior,
boundaries, compatibility and acceptance criteria; the plan records approach,
tasks and verification. Obtain maintainer approval before implementation. Small
fixes do not need a separate plan.

### Service boundaries

Each module owns its configuration, startup and database tables. Communicate
through public contracts; do not import another application's private packages or
read its tables. Console obtains identity and authorization through IAM's
authenticated API.

| Go location | Responsibility |
| --- | --- |
| `cmd/` | Startup arguments and process entry |
| `internal/config/` | Configuration loading, expansion and validation |
| `internal/bootstrap/` | Dependency creation, assembly, startup and cleanup |
| `internal/router/` | Route registration and middleware composition |
| `internal/handler/` | Request binding, DTO mapping and response |
| `internal/service/` | Use cases, authorization, transactions and audit ordering |
| `internal/repository/` | Database access and transaction-bound operations |

Read environment variables only during configuration loading and pass dependencies
explicitly. Add shared packages only for actual cross-application reuse. Add
versioned migrations in the owning module and never modify an applied migration.

### API contracts and authorization

All APIs use `/api/v1/{module}/...` on one listener per service. Routes explicitly
require a user session, a service credential, or allow public access. Service
authorization also validates the original user session and permissions. External
HTTPS is terminated by a reverse proxy.

1. Edit the owning module's native Go DTOs, Gin routes in `internal/router` and
   handlers. Update module documentation and actual callers together.
2. Bind requests with `httpx`, use shared HTTP middleware and return ordinary JSON
   through `result`. Preserve real status codes, empty bodies and streaming.
3. Web declares only consumed Zod models in `src/spx/api/iam/schemas.ts`, infers
   TypeScript types and calls module APIs through Axios. Verify changes with real
   HTTP and browser tests.
4. Change shared result codes or permissions only through their owning Go catalogs:
   run `make catalogs-generate` and `make catalogs`. Never edit `.gen.*` exports.

Server-side authorization remains authoritative. Preserve declared permission
checks, CSRF protection, session revocation, and transaction and audit guarantees.
UI visibility does not grant API access. Keep the default same-origin model;
cross-origin credential access requires an explicit design.

### Code and documentation style

Follow [AGENTS.md](AGENTS.md), the [database design](docs/development/database-design.md),
the [backend structure](docs/development/backend-structure.md) and the
[project code standard](.agents/skills/spx-mode/SKILL.md).

- Keep changes scoped. Avoid unrelated formatting, dependency upgrades and
  speculative abstractions. Validate unknown input at its owning boundary and
  pass typed values inward; retain authorization and current-state checks.
- Closed stored values use a `CHECK` constraint plus the module's
  `internal/enum` constants. Compare constants, not string literals.
- Log with `logging.From(ctx)` and simple English messages. Never log passwords,
  tokens, cookies, CSRF values, request bodies or private data.
- Format changed Go files with `gofmt`. Use Chinese Go comments beginning with
  the exported name and `snake_case` database columns.
- In TypeScript, prefer inference, named imports, early returns and explicit types
  at boundaries; avoid `any` and unnecessary wrappers. Web uses Biome.
- Preserve the component, style and theme baseline. Follow the
  [shadcn Skill](.agents/skills/shadcn/SKILL.md), inspect rendered interactions and
  responsive layouts, and update both `en` and `zh-CN` next-intl catalogs.
- Update module READMEs when behavior changes. Keep paired English/Chinese
  documents consistent. Preserve upstream licenses and attribution; exclude
  secrets, private project details and external workspace paths.

## Verify your change

Start behavior changes with a focused failing test. Choose the lowest layer that
reliably protects the behavior: unit tests for rules and boundaries, integration
tests for real component contracts, and E2E tests for core user flows and access
boundaries. See the [testing guide](docs/development/testing.md) for isolation,
cleanup and targeted commands.

Run from the repository root:

```sh
make test
make vet
make lint
make typecheck
make catalogs
make build
gofmt -l src
```

`gofmt -l src` must print nothing. For contract changes, regenerate and check that
generated artifacts match their sources. `make vuln-go` runs Go dependency
vulnerability scanning. GitHub Actions runs the same `make test` suite on Linux,
macOS and Windows, plus static checks and a production build.

For persistence, service integration, authentication, authorization or core
browser flows, run the applicable suites:

```sh
make integration ARGS="--docker --race"
make install-browsers
make test-e2e ARGS="--docker"
```

The `--docker` commands create isolated test containers and clean up their own
resources. External test connections are supported; use dedicated test databases
and follow the testing guide's permissions and cleanup rules. Never point tests at
development or production data. Use bounded readiness checks; fixed sleeps and
retries must not hide failures.

Documentation-only changes need link, command and content checks relevant to the
edit. Report what you ran, the results, and any unverified scope. A check that was
not run is not a passing check.

## Submit a pull request

1. Start a work branch from the latest project `main`.
2. Make a focused change and run the checks relevant to it.
3. Commit and push the branch to your fork.
4. Open a PR against `EayonLee/SecPlexus:main`. Use a draft PR for early feedback.
5. Push follow-up commits to the same branch; they appear in the existing PR.

```sh
git fetch upstream
git switch -c session-expiry upstream/main
```

Branch names use at most three lowercase words separated by hyphens, such as
`iam-login` or `contribution-guide`. Do not use slashes or type prefixes. An issue
is not required for every change.

Commit messages and PR titles use `type(scope): summary`; the scope is optional.
Supported types are `feat`, `fix`, `docs`, `chore`, `refactor` and `test`.
Example: `fix(iam): revoke sessions after password reset`.

Describe the problem, resulting behavior, design decisions reviewers need to
understand, and actual validation results. Link the related issue and approved
plan. Include screenshots for UI changes and describe migration or compatibility
impact. The [pull request template](.github/PULL_REQUEST_TEMPLATE.md) lists the
required sections.

Before marking a PR ready for review:

- [ ] The change addresses one problem and follows the approved scope.
- [ ] Contracts, generated clients, migrations and documentation are synchronized.
- [ ] Relevant tests and checks pass; skipped or unavailable checks are explained.
- [ ] UI changes were checked in the browser, including affected interactions and layouts.
- [ ] No credentials, sensitive logs, private data or unrelated files are included.

## Review and merge

Community members are welcome to review PRs, ask questions, test changes and
suggest improvements. Authors should explain decisions and address feedback in the
PR so the discussion stays available.

Maintainers make the final acceptance decision and merge into the protected
`main` branch; see [GOVERNANCE.md](GOVERNANCE.md) and [MAINTAINERS.md](MAINTAINERS.md).
A review approval records an assessment; it does not grant merge permission. Contributing through a fork or opening a PR does not require merge
permission.

If a PR has conflicts, update the work branch with the latest `main` and resolve
them locally. After merge, delete the work branch and sync your fork.

AI-assisted contributions follow the same process. You are responsible for
understanding the submitted changes, checking generated code and verifying the
reported results.

### Framework integration

Use native Gin handlers and middleware with module-owned DTOs. IAM credential use
cases take typed values; Argon2id, SCS, CSRF and the Redis limiter own their
mechanisms. Use sqlc queries and explicit transactions and Goose migrations; use
Viper/mapstructure with Cobra for configuration and commands. See the
[role management walkthrough](docs/development/role-management.md); do not build a
second generic utility or shadow router for an existing framework capability.

Web uses Axios for module APIs and infers types from consumed Zod models. Query
owns server state; React Hook Form and Zod own forms. Password operations stay
outside MutationCache. Preserve template components, styles and next-intl catalogs.
