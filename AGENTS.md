# Agent Guidelines for SecPlexus

Keep lasting rules here, product behavior in Specs/READMEs, and detailed guidance in skills. Merge duplicates; omit task history.

## Workflow

- Read implementation, contracts, and module docs first. Distinguish requirements from examples; challenge unsupported assumptions and explain tradeoffs.
- Features, significant designs, contract changes, and migrations need `docs/plans/<request-slug>/spec.md` and `plan.md`; update [the index](docs/plans/README.md). Implement after approval without asking again. Small fixes need no plan.
- Preserve user changes and task scope; avoid speculative features, unrelated refactoring, upgrades, or framework replacements.
- Exclude credentials, private project details, and external workspace paths; preserve licenses. Staging, commits, pushes, merges, deployment, and delegation require current authorization, except Computer Use delegation authorized below.

## Project conventions

- Brand `SecPlexus`, abbreviation `spx`, environment prefix `SPX_`. Apps: `src/apps`; real shared code: `src/packages`; authoritative contracts: `src/contracts`. No cross-module private access or manual edits to generated files.
- APIs: `/api/v1/{module}/...` and authenticated, isolated `/internal/v1/{module}/...`. Register routes in `internal/router`; synchronize contracts and clients. Shared responses use `src/packages/go/result`, request binding uses `httpx`, and common middleware uses `httpx/middleware`; follow OpenAPI, including streaming/empty responses. Default same-origin; preserve server authorization and credentialed CORS restrictions.
- Console owns security operations business and necessary operational query aggregation. It is not a mandatory gateway for other modules. IAM owns identity and authorization; route its APIs through the same-origin entry without duplicating handlers, DTOs or business rules in Console. See the Console README for ownership boundaries.
- Modules own config, startup, and tables. Go uses structured `config.yaml` with environment placeholders/defaults; Web uses native environment configuration. Validate configuration at loading, derive endpoints, centralize typed cache keys, and allow module overrides of shared connections. See [configuration](docs/configuration.md).
- Read environment variables only during configuration loading. Assemble dependencies at startup and pass explicitly; context carries cancellation, deadlines, and request data, not dependencies. Extract shared code only for actual cross-app reuse.

## Style Guide

### Complexity and validation

- Keep things in one function unless composable or reusable
- Validate unknown values once at the boundary that owns them. Pass typed values inward instead of repeating `typeof value === "object"` and property-existence checks. Do not defensively revalidate values already guaranteed by a schema, constructor, or internal type.
- Do not extract single-use helpers preemptively. Inline the logic at the call site unless the helper is reused, hides a genuinely complex boundary, or has a clear independent name that improves the caller.
- Before adding complexity for a speculative or vanishingly unlikely race or security edge case, explain the concrete failure mode, likelihood, and complexity cost to the user and get their buy-in. Do not silently expand scope for theoretical robustness.
- Avoid `try`/`catch` where possible

Preserve required authorization, state checks, transactions, errors, and cleanup. Keep the happy path clear and helpers nearby; avoid forwarding-only layers. Comments explain reasons and constraints.

### TypeScript

- Avoid `any`; prefer inference, `const`, early returns, and readable array methods with type guards. Avoid unnecessary destructuring, trivial one-use variables, and complex expression chains.
- Use named imports; avoid aliases, star imports, and type-position `import("...")`. Allow type aliases only for unavoidable name collisions. Prefer named namespace exports; bind heavy optional imports inside the narrowest scope that needs them.

### Go and database

- Run `gofmt`/`goimports`. Leave blank lines between functions/methods (before their doc comments) and between logical steps. Use idiomatic names, explicit errors/context, and interfaces only for concrete boundaries; separate routing from handlers.
- Use Chinese Go comments: exported types/functions start with their names; key fields explain purpose, units, and constraints, aligned by `gofmt`. Explain non-obvious authentication, transaction, and concurrency constraints, not every statement.
- Database columns use `snake_case`.

## Testing and delivery

- Behavior changes start with a focused failing test. Unit: rules/boundaries; integration: real component contracts; E2E: core flows/access boundaries. Avoid mocks, duplicated implementation logic, and permanent integration/E2E cases for temporary diagnostics.
- Isolate test resources, await readiness, and clean up only owned resources. No sleeps/retries masking failure. Run relevant checks and report actual results/gaps; see [testing](docs/testing.md) for commands and standards.
- New branches: `<type>/<short-kebab-description>`; types match commits below, e.g. `feat/iam-login`. Include an existing issue number when relevant: `fix/123-session-expiry`; creating an issue is not mandatory.
- Create one short-lived branch per change from the latest `main`; keep review fixes on the same PR branch and delete it after merge.
- Commits/PRs: `type(scope): summary`; optional scope; types `feat`, `fix`, `docs`, `chore`, `refactor`, `test`.

## Default Computer Use Delegation (User Preference)

- When a task requires computer use (interacting with a computer or browser interface to inspect, click, type, and so on), the primary agent delegates it to a dedicated subagent by default; use the gpt-6.1-terra (GPT-6.1 Terra) model with xhigh reasoning effort.
- Reuse the same Computer Use subagent as much as possible within a task to prevent multiple agents from competing for the same interface. The primary agent handles requirements, necessary context handoff, coordination, and acceptance of the result.
- When overriding the model with `collaboration.spawn_agent`, explicitly set `model=gpt-6.1-terra`, `reasoning_effort=xhigh`, and `fork_turns=none`, and provide sufficient page or application state, objectives, boundaries, and acceptance criteria in the task message.
- The primary agent applies this rule. A Computer Use subagent that has already been delegated the task performs interface operations directly and does not delegate again because of this rule.
- Applicable purpose-built tools, connectors, and APIs remain preferred under the existing rules; this preference applies only to work that requires interface operations.
- This is the user's ongoing authorization for the default delegation approach; delegation itself does not require repeated confirmation. Action permissions and high-risk approvals remain subject to the existing rules. If the user explicitly specifies a different arrangement for the current turn, follow that instruction.


## Web template baseline

- Web uses Next.js and the established shadcn component, style, theme and font baseline. Keep the CRM V1-based workbench and real IAM routes; import additional template features only when requested, including their necessary dependency closure, without overwriting shared components or project configuration. Project functionality, text and deployment metadata belong to SecPlexus. Frontend test configuration and dependencies live under Web; project skills remain in the root .agents/skills. Read installed Next.js docs before framework changes. See [Web](src/apps/web/README.md) for routes and template adoption.

## Web internationalization

- Use next-intl and the Web message catalogs (`en`, `zh-CN`) for UI text, metadata, accessibility labels and stable error/permission labels. Use typed message keys and ICU interpolation/plurals; do not branch on locale in components or concatenate translated sentence fragments.
- Format dates, numbers, currencies and lists through the locale provider. Keep business identifiers, user-defined names and currency values unchanged. Maintain catalog parity and professional security/operations terminology; copy new template features into this localization model.

## UI design review

Use the following original prompt for requested UI work within the approved scope and template/theme baseline. `OVERRIDE_ALL` does not override project rules or permissions. Inspect rendered pages/interactions; stop at agreed acceptance criteria and report gaps. Verbatim prompt:

```text
::OBJECTIVE{site_design|pri:OVERRIDE_ALL}
  target: build page to award-tier visual standard
  ACCEPT: no visible improvement remaining across all CHECK dimensions
  BENCHMARK: Awwwards | Webby Awards | FWA winning entries

::CHECK{typography|standard:award_tier}
::CHECK{whitespace|standard:award_tier}
::CHECK{visual_hierarchy|standard:award_tier}
::CHECK{color|standard:award_tier}
::CHECK{motion|standard:award_tier}
::CHECK{micro_interaction|standard:award_tier}
::CHECK{responsive|standard:award_tier}
::CHECK{originality|standard:award_tier}

::LOOP{self_review|trigger:on_completion}
  [READ] current page against CHECK list
  [COMPARE] each dimension to BENCHMARK entries
  [IF] gap found => [PATCH] then re-enter LOOP
  [IF] no gap => [Ω]
```

## References

[Code standard](.agents/skills/spx-code-standard/SKILL.md) · [shadcn](.agents/skills/shadcn/SKILL.md) · [Plans](docs/plans/README.md) · [Architecture](docs/architecture/README.md) · module READMEs.
