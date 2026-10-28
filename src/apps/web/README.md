# SecPlexus Web

## Routes and real identity

| Route | Purpose |
| --- | --- |
| `/login` | Real IAM username/password login and mandatory password change |
| `/dashboard` | Authenticated security workbench based on CRM V1; template sample data |
| `/account/profile`, `/account/security` | Personal identity, password changes, recent authentication, device revocation and logout |
| `/management/orgs/{org_id}/...` | Organization settings, members, roles, permission previews and audit |
| `/management/platform/...`, `/management/system/security` | Platform users, organizations, audit and read-only security overview |

The product shell uses the shared Sidebar, form, dialog and table components, plus its existing layout controls and theme switcher. Navigation follows the server capability projection. The single workbench retains CRM V1 sample data and never counts as real business statistics. The Dashboards group contains only “安全智驾工作台”; real IAM navigation remains capability-based.

IAM credentials stay in HttpOnly cookies. Zustand holds only the current in-memory identity and capability projection. No session, token, role, permission or business data is persisted in localStorage; only a remembered username and presentation preferences may persist. Identity epochs and cancellation reject stale responses after logout or organization changes. BroadcastChannel synchronizes session changes across tabs without sharing secrets. URL organization context remains independent per tab.

`src/app/api/v1/[module]/[...path]/route.ts` streams public IAM/Console requests to server-configured origins. It forwards Cookie, Origin and CSRF context, preserves response status and Set-Cookie, and does not expose internal APIs or service credentials. Backend authorization remains authoritative. A dependency failure returns 503 and pauses interaction without turning it into a successful logout.

## Development and build

From the repository root:

```sh
pnpm install --frozen-lockfile --ignore-scripts
make dev
pnpm typecheck
pnpm lint
make build
pnpm preview
make stop
```

Root startup reads the root `.env`, derives backend addresses, and supplies `SPX_WEB_IAM_PROXY_TARGET` and `SPX_WEB_CONSOLE_PROXY_TARGET` only to the Next server. `SPX_WEB_HOST` and `SPX_WEB_PORT` control the CLI listener. `make dev` applies IAM migrations against configured development infrastructure; it never starts Docker.

Root builds also load the root `.env` when present and pass filtered Web configuration. `SPX_PUBLIC_URL` supplies the default public metadata origin, with an optional `NEXT_PUBLIC_APP_URL` override. Process values take precedence. Public values are fixed by the production build, so rebuild after changing them; runtime listener or proxy changes require a restart. A build can run without creating a development `.env`.

For independent Web development, copy [.env.example](.env.example) to `.env.local`, configure the public API origins, and run `pnpm dev` here. This uses Next.js native environment loading and the default port 3000. Keep IAM's public origin consistent with the browser entry. Never place secrets in `NEXT_PUBLIC_*`.

Production uses `next build` followed by `next start`, including server components and API routes. It is not a static export. The Web Dockerfile runs Next.js as an unprivileged Node user. See [configuration](../../../docs/configuration.md), [testing](../../../docs/testing.md), and the [replacement plan](../../../docs/plans/replace-web-template/plan.md).

## Contracts and skills

Run `pnpm contracts:generate` from the root after editing authoritative contracts. Generated clients remain in `src/auth/`; they are not part of the upstream template.

Follow the repository [AGENTS.md](../../../AGENTS.md), project [shadcn Skill](../../../.agents/skills/shadcn/SKILL.md), and [project code standard](../../../.agents/skills/spx-code-standard/SKILL.md). Use shared components and semantic theme tokens; keep UI rules in one place.

## Module-owned tooling

Frontend dependencies, Vitest/Playwright configuration, `tests/unit/`, and `tests/e2e/` live here; project skills remain in the root `.agents/skills/`. Root commands delegate to this module. The root pnpm lockfile is the only dependency lockfile. The root `/` route redirects to the authenticated workspace.

The Web TypeScript project includes `.mts` tooling configuration and `.mjs` scripts with ESNext modules. The theme-preset generator keeps its separate CommonJS script configuration.

```sh
pnpm typecheck
pnpm test:unit
pnpm exec playwright install chromium
pnpm test:e2e --docker
```

These commands run in `src/apps/web/`. E2E provisions owned isolated infrastructure and uses the repository startup entry; it does not read developer credentials. For deployment, set `NEXT_PUBLIC_APP_URL` to the browser origin before building. This is public metadata, never a credential.

## Selective template adoption

The project keeps one CRM V1-based workbench in `src/spx/pages/workbench/`, real IAM pages, and shared UI, fonts and theme infrastructure. Other template pages, demo identities, menus and feature-only dependencies are removed. Appearance controls live in `src/spx/components/` alongside the product shell.

Treat the template clone as a read-only reference. For each requested feature, copy its page or block and the necessary local components, types, data and dependencies; reuse existing project UI and theme tokens. Do not overwrite project configuration, lockfiles, identity/session code or already customized components. Preserve required licenses and attribution. Compare the selected feature with the fixed template version and verify its interactions after integration.

Replace sample data with the owning module's generated API client when that business feature is implemented. IAM APIs stay with IAM; Console owns security operations and useful operational query aggregation, not duplicate wrappers around every module. See [Console boundaries](../console/README.md).

## Internationalization

The application uses `next-intl` with `en` and `zh-CN` catalogs in `messages/`. English is the default. A language menu is available on sign-in, authenticated pages, service-error and not-found views. The validated `spx-locale` preference cookie is separate from IAM credentials and is written by a Server Action; Next updates the current UI and layouts after the cookie changes. Routes and API contracts keep their existing paths without locale prefixes.

`src/i18n/config.ts` owns supported locales, defaults and the display time zone. `request.ts` loads the matching catalog per request. The root provider shares that configuration with client components, and `html lang`, metadata and page titles follow the same locale. Dashboard is the English page name; 安全智驾工作台 is the Chinese name.

Use typed `useTranslations` / `getTranslations` message keys and ICU placeholders or plurals. Add every message to both catalogs; do not concatenate sentence fragments or add locale conditionals to components. Format numbers, money, lists and timestamps with `useFormatter`; displayed timestamps use UTC consistently across both languages. Currency is preserved (the CRM sample uses USD), not converted when switching languages. Pass the locale into components such as the date picker.

Translate platform text, stable permission labels and built-in role names. Preserve account names, organization names, custom role names, identifiers and other user data. API errors use stable result-code-to-message-key mappings; never translate or display raw internal error bodies. CRM sample labels retain their sales meaning: leads are 销售线索 and won projects are 成交项目, not invented security metrics.

Unit checks cover catalog parity, ICU interpolation/plurals, and actual Dashboard component rendering in both languages. Existing IAM E2E selects Chinese explicitly, so it does not depend on the default locale. Run the local unit/type/build checks after adding translations; browser checks should verify switching, refresh persistence, translated accessibility labels, metadata and narrow layouts.
