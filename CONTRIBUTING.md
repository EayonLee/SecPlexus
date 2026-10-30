# Contributing to SecPlexus

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh-CN.md)

Thank you for contributing to SecPlexus. Code, reproducible bug reports, tests,
documentation, and usability improvements are all welcome. This guide explains
how to prepare a change that fits the project and can be reviewed and verified.

Everyone is welcome to open issues, submit pull requests, and participate in
reviews. You do not need write access to contribute: fork the repository and
submit changes from a branch in your fork to this repository's `main` branch.

## Before you start

Search the [existing issues](https://github.com/EayonLee/SecPlexus/issues) and
[pull requests](https://github.com/EayonLee/SecPlexus/pulls) before opening a new
one. Keep each contribution focused on one problem.

- For bugs, include the revision, environment, reproduction steps, expected and
  actual behavior, and relevant sanitized logs. Screenshots help with UI issues.
- For features, explain the user problem, proposed behavior, affected modules,
  and compatibility or data changes. Discuss substantial work before implementing it.
- Small fixes and documentation corrections can go directly to a pull request.

For suspected vulnerabilities, use GitHub private vulnerability reporting when
available. Otherwise, ask a maintainer for a private reporting channel without
posting exploit details, credentials, or affected private data in a public issue.

## Understand the codebase

SecPlexus currently consists of two Go services and a React application in one
repository. PostgreSQL stores IAM data and sessions; Redis supports rate limiting.

| Location | Responsibility |
| --- | --- |
| [`src/apps/iam/`](src/apps/iam/README.md) | Accounts, authentication, sessions, organizations, roles, authorization, and identity audit |
| [`src/apps/console/`](src/apps/console/README.md) | Business-service entry point; currently organization context and authorized SSE connections |
| [`src/apps/web/`](src/apps/web/README.md) | Next.js, React, TypeScript, Tailwind CSS, and shadcn/ui; shared application shell and product pages |
| [`src/packages/`](src/packages/README.md) | Implementations actually shared across applications |
| `tests/` and `src/apps/web/tests/` | Integration-test runners and Web browser E2E tests |
| `scripts/` and `deploy/` | Development, build, test, and deployment tooling |
| [`docs/plans/`](docs/plans/README.md) | Requirements, implementation plans, approval status, and validation evidence |

Read the affected module's README and existing implementation first. The
[architecture documents](docs/architecture/README.md) include earlier platform
proposals; heed their version notes. Use module documentation and approved
specifications for the current implementation. Console business workflows and
UI demonstration pages must not be presented as completed product capabilities.

## Set up a development environment

Tool versions follow `go.mod` and `package.json`:

| Tool | Version or requirement |
| --- | --- |
| Go | 1.27.3 |
| Node.js | 22.12 or later; 24 recommended |
| pnpm | 10.32.1 |
| Make | Root development commands |
| PostgreSQL and Redis | Reachable development instances; the supplied Compose stack uses PostgreSQL 16 and Redis 7 |
| Docker with Compose | Needed when you choose container-based dependencies or tests |

The template font registry uses `next/font/google`: a cold Web build needs access
to Google Fonts stylesheet and font endpoints. A cached build does not verify that
network requirement. See [build troubleshooting](docs/configuration.md#首次构建与网络排障)
if font downloads fail; do not change the template fonts to hide a network failure.

Fork the repository to your GitHub account, then clone your fork and add the
project repository as `upstream`. Replace `YOUR_USERNAME` with your GitHub username:

```sh
git clone https://github.com/YOUR_USERNAME/SecPlexus.git
cd SecPlexus
git remote add upstream https://github.com/EayonLee/SecPlexus.git
```

If you already have a checkout, use it. Contributors with repository write access
can also work on branches in the project repository. Run the following from the
repository root; keep `.env` if it already exists:

```sh
pnpm install --frozen-lockfile --ignore-scripts
test -e .env || cp .env.example .env
chmod 600 .env
```

Fill in `.env` with your development database connection, configuration-managed administrator
password, CSRF key, and service credential. Follow the
[configuration guide](docs/configuration.md) for field requirements and reverse proxy access.
Keep `.env`, credentials, and logs out of version control.

Connect to existing development PostgreSQL and Redis instances, or explicitly
create the supplied dependencies:

```sh
pnpm compose:up postgres redis
```

Then start the applications:

```sh
pnpm config:check
make dev
```

`pnpm config:check` validates all selected modules offline, without database connections,
migrations or application logs. `make dev` installs frontend dependencies, validates
all selected configurations, applies IAM migrations, and starts IAM, Console and Web
with hot reload. It connects to configured infrastructure and does not start Docker.
Use a development database. The unique system administrator defaults to
`admin/secplexus`; its identity and password remain authoritative in configuration.
Change `SPX_IAM_BOOTSTRAP_USERNAME` / `SPX_IAM_BOOTSTRAP_PASSWORD` and restart all
IAM replicas to update this account. Application pages and password-reset commands
cannot modify it. Ordinary accounts can change their passwords.

| Command | Purpose |
| --- | --- |
| `make build` | Build both Go services and Web without requiring `.env` |
| `pnpm preview` | Start the backends and preview the built Web application; configure the matching public origin |
| `pnpm compose:up` | Explicitly build and deploy the complete Compose stack |
| `make stop` | Stop this checkout's applications and Compose containers while retaining data |

## Design and implementation

Features, significant designs, API contract changes, and database migrations
need `docs/plans/<request-slug>/spec.md` and `plan.md`, with an entry in the
[plan index](docs/plans/README.md). Describe behavior, boundaries, compatibility,
and acceptance criteria in the specification; record the approach, tasks, and
validation in the plan. Obtain maintainer approval before implementation.
Small fixes do not need a separate plan.

### Service boundaries

Each module owns its configuration, startup, and database tables. Communicate
through public contracts; do not import another application's private packages
or read its tables. Console obtains identity and authorization through IAM's
authenticated API.

| Go location | Responsibility |
| --- | --- |
| `cmd/` | Startup arguments and entry point |
| `internal/config/` | Configuration loading, environment expansion, and validation |
| `internal/bootstrap/` | Dependency creation, assembly, startup, and cleanup |
| `internal/router/` | Route registration and middleware composition |
| `internal/handler/` | Request binding, use-case calls, and responses |
| `internal/service/` | Business rules, authorization scope, transactions, and audit ordering |
| `internal/repository/` | Database access and transaction-bound operations |

Read environment variables during configuration loading and pass dependencies
explicitly. Configuration loading must not open network connections or run
migrations. Add shared packages only for actual cross-application reuse. Add
versioned migrations in the owning module and preserve already applied scripts.

### API contracts and authorization

All APIs use `/api/v1/{module}/...` on one listener per service. Routes explicitly require a user session, a service credential, or allow public access. Service authorization also validates the original user session and permissions. Application services use HTTP; external HTTPS is terminated by a reverse proxy.

1. Edit the owning module's native Go DTOs, Gin routes in `internal/router`, and handlers. Update module documentation and actual callers together.
2. Bind requests with `httpx`, use shared HTTP middleware, and return ordinary JSON through `result`. Preserve real status codes, empty bodies and streaming semantics.
3. Web declares only consumed Zod models in `src/spx/api/iam/schemas.ts`; infer TypeScript types from them and call module APIs through Axios. Verify changes with real HTTP and browser tests.
4. Only when changing shared result codes or permissions, run `pnpm catalogs:generate` and `pnpm catalogs:check`. Their sources live in Go `result` and IAM `authorization`; do not edit `.gen.*` exports manually.

Server-side authorization remains authoritative. Preserve organization scope,
CSRF protection, session revocation, and transaction and audit guarantees.
UI visibility does not grant API access. Keep the default same-origin model;
cross-origin credential access requires an explicit design.

### Code and documentation style

Follow [AGENTS.md](AGENTS.md) and the
[project code standard](.agents/skills/spx-mode/SKILL.md).

- Keep changes scoped. Avoid unrelated formatting, dependency upgrades, and
  speculative abstractions. Validate unknown input at its owning boundary and
  pass typed values inward; retain authorization and current-state checks.
- Format changed Go files with `gofmt` and organize imports with `goimports`.
  Use Chinese documentation comments beginning with the exported name and
  `snake_case` database columns.
- In TypeScript, prefer inference, named imports, early returns, and explicit
  types at boundaries; avoid `any` and unnecessary wrappers. Web uses Biome.
- Preserve the project component, style, and theme baseline.
  Follow the official [shadcn Skill](.agents/skills/shadcn/SKILL.md), add project
  behavior in new files, and inspect rendered interactions and responsive layouts.
  See the Web README for product routes, selective template adoption and next-intl
  localization. Add professional translations to both catalogs; use ICU messages
  and locale formatters instead of language branches in components.
- Update module READMEs when behavior changes. Keep English and Chinese versions
  of paired documents consistent. Preserve upstream licenses and attribution;
  exclude secrets, private project details, and external workspace paths.

## Verify your change

Start behavior changes with a focused failing test. Choose the lowest layer
that reliably protects the behavior: unit tests for rules and boundaries,
integration tests for real component contracts, and E2E tests for core user flows
and access boundaries. See the [testing guide](docs/testing.md) for isolation,
resource cleanup, and targeted test commands.

Run relevant checks from the repository root:

```sh
pnpm catalogs:check
gofmt -l src
go vet ./src/...
pnpm lint
pnpm test:unit
pnpm typecheck
make build
```

`gofmt -l src` must produce no paths. For contract changes, also regenerate and
check that generated artifacts are consistent with their sources. Use `pnpm catalogs:check` for catalog consistency and `make vuln-go` for Go
dependency vulnerability scanning. These are local checks; no remote CI is required.

For changes affecting persistence, service integration, authentication,
authorization, or core browser flows, run the applicable suites:

```sh
pnpm test:integration --docker --race
pnpm --dir src/apps/web exec playwright install chromium
pnpm test:e2e --docker
```

These `--docker` commands create isolated test containers and clean up their own
resources. External test connections are also supported; use dedicated test
databases and follow the testing guide's permissions and cleanup requirements.
Never point tests at development or production data. Use bounded readiness
checks and assertions; fixed sleeps and retries must not hide failures.

Documentation-only changes need link, command, and content checks relevant to
the edit. Report what you ran, the results, and any unverified scope. A check
that was not run is not a passing check.

## Submit a pull request

1. Start a work branch from the latest project `main`.
2. Make a focused change and run the checks relevant to it.
3. Commit your changes and push the work branch to your fork.
4. Open a PR with `EayonLee/SecPlexus:main` as the base and your work branch as
   the head. Use a draft PR when you want early feedback or are still working.
5. Respond to review feedback and push follow-up commits to the same branch;
   they appear in the existing PR automatically.

For a checkout with the `upstream` remote configured above, start a branch with:

```sh
git fetch upstream
git switch -c session-expiry upstream/main
```

Name branches with at most three lowercase words separated by hyphens, such as
`iam-login` or `contribution-guide`. Do not use slashes or type prefixes.
An issue is not required for every change.

Use `type(scope): summary` for commit messages and PR titles; the scope is
optional. Supported types are `feat`, `fix`, `docs`, `chore`, `refactor`, and
`test`. For example: `fix(iam): revoke sessions after password reset`.

Describe the problem, resulting behavior, design decisions reviewers need to
understand, and actual validation results. Link the related issue and approved
plan where applicable. Include screenshots for UI changes and describe migration
or compatibility impacts. Keep review discussion specific and constructive.

Before marking a PR ready for review:

- [ ] The change addresses one problem and follows the approved scope.
- [ ] Contracts, generated clients, migrations, and documentation are synchronized where affected.
- [ ] Relevant tests and checks pass; skipped or unavailable checks are explained.
- [ ] UI changes have been checked in the browser, including affected interactions and layouts.
- [ ] No credentials, sensitive logs, private data, or unrelated files are included.

## Review and merge

Community members are welcome to review PRs, ask questions, test changes, and
suggest improvements. Authors should explain their decisions and address
feedback in the PR so the discussion remains available to future contributors.

Project maintainers make the final acceptance decision and merge changes into
the protected `main` branch. A review approval records a reviewer's assessment;
it does not grant permission to merge. Contributing through a fork or opening a
PR does not require merge permission.

If your PR has conflicts, update your work branch with the latest project `main`
and resolve them locally. Once the PR is merged, you can delete the work branch
and sync your fork for the next contribution.

AI-assisted contributions follow the same process. You are responsible for
understanding the submitted changes, checking generated code, and verifying the
reported test results.

### Framework integration

Use native Gin handlers and middleware with module-owned DTOs. IAM credential use cases take typed values; Argon2id, SCS, CSRF and the Redis limiter own their mechanisms. Use GORM projections/transactions and Goose migrations, and Viper/mapstructure with Cobra for configuration and commands. See the [actual role management walkthrough](docs/development/role-management.md); do not create another generic utility or routing layer for an existing framework capability.

Use Axios for module API calls and infer types from consumed Zod models. Query owns server state; RHF and Zod own forms. Sensitive password operations stay outside MutationCache. Preserve the template components/styles and next-intl catalogs. Run `make lint-go`, `make vuln-go` and the applicable checks in the [testing guide](docs/testing.md); these are local checks, not a deployment or CI workflow.
