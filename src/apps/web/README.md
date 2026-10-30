# SecPlexus Web

## Routes and real identity

| Route | Purpose |
| --- | --- |
| `/login` | Real IAM username/password login and mandatory password change |
| `/dashboard` | Authenticated security workbench based on CRM V1; template sample data |
| `/settings/profile` | Profile details and organization membership |
| `/settings/account` | Account identity and password changes |
| `/management/orgs/{org_id}/...` | Organization settings, members, roles, permission previews and audit |
| `/management/platform/...` | Platform accounts, organization directory and platform audit |

The product shell uses the shared Sidebar, form, dialog and table components, plus its existing layout controls and theme switcher. Navigation follows the server capability projection. The single workbench retains CRM V1 sample data and never counts as real business statistics. The Dashboards group contains only “安全智驾工作台”; real IAM navigation remains capability-based.

IAM credentials stay in HttpOnly cookies. Zustand holds only the current in-memory identity and capability projection. No session, token, role, permission or business data is persisted in localStorage; only a remembered username and presentation preferences may persist. Identity epochs and cancellation reject stale responses after logout or organization changes. BroadcastChannel synchronizes session changes across tabs without sharing secrets. URL organization context remains independent per tab.

`src/app/api/v1/[module]/[...path]/route.ts` streams IAM/Console API requests to server-configured origins. It forwards Cookie, Origin and CSRF context, preserves response status and Set-Cookie, and does not forward Authorization headers or receive service credentials. Backend authorization remains authoritative. A dependency failure returns 503 and pauses interaction without turning it into a successful logout.

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

Root startup reads the root `.env` as the shared configuration source. `SPX_IAM_HOST/PORT` and `SPX_CONSOLE_HOST/PORT` supply server-only connection locations to every caller. `SPX_WEB_PORT` controls the HTTP listener; `SPX_WEB_ORIGIN` is an IAM security input describing the trusted browser entry; Web metadata does not need an absolute origin. Application servers use HTTP; Nginx or another reverse proxy owns HTTPS and certificates.

Web startup loads native environment files with official `@next/env`, then invokes the Next CLI on the configured port, defaulting its bind address to `0.0.0.0`. Standalone Web uses the module `.env.example` → `.env.local` with the same keys. Root startup already injects them, so a second file is unnecessary. Root `pnpm dev web` / `pnpm preview web` starts only Web against existing services, without Go migration or startup.

Root builds use the same filtered environment, falling back to non-secret example settings when no private file exists. Web never receives database/Redis credentials, administrator passwords or service credentials. Web does not receive the IAM origin setting or maintain a separate metadata URL. Restart after changing runtime configuration.

For independent Web development, copy [.env.example](.env.example) to `.env.local`, configure the public API origins, and run `pnpm dev` here. This uses Next.js native environment loading and the default port 3000. Keep IAM's public origin consistent with the browser entry. Never place secrets in `NEXT_PUBLIC_*`.

Production uses `next build` followed by `next start`, including server components and API routes. It is not a static export. The Web Dockerfile runs Next.js as an unprivileged Node user. See [configuration](../../../docs/configuration.md), [testing](../../../docs/testing.md), and the [replacement plan](../../../docs/plans/replace-web-template/plan.md).

## Contracts and skills

Shared permission/result constants are exported into `src/auth/` by `pnpm catalogs:generate`; `pnpm catalogs:check` verifies them. API calls and Zod consumption models are authored under `src/spx/api/`.

Follow the repository [AGENTS.md](../../../AGENTS.md), project [shadcn Skill](../../../.agents/skills/shadcn/SKILL.md), and [project code standard](../../../.agents/skills/spx-mode/SKILL.md). Use shared components and semantic theme tokens; keep UI rules in one place.

## Module-owned tooling

Frontend dependencies, Vitest/Playwright configuration, `tests/unit/`, and `tests/e2e/` live here; project skills remain in the root `.agents/skills/`. Root commands delegate to this module. The root pnpm lockfile is the only dependency lockfile. The root `/` route redirects to the authenticated workspace.

The Web TypeScript project includes `.mts` tooling configuration and `.mjs` scripts with ESNext modules. The theme-preset generator keeps its separate CommonJS script configuration.

```sh
pnpm typecheck
pnpm test:unit
pnpm exec playwright install chromium
pnpm test:e2e --docker
```

These commands run in `src/apps/web/`. E2E provisions owned isolated infrastructure and uses the repository startup entry; it does not read developer credentials. For remote or reverse-proxy access, set root `SPX_WEB_ORIGIN` to the actual browser origin so IAM can validate requests and apply Secure Cookie policy.

## Selective template adoption

The project keeps one CRM V1-based workbench in `src/spx/pages/workbench/`, real IAM pages, and shared UI, fonts and theme infrastructure. Other template pages, demo identities, menus and feature-only dependencies are removed. Appearance controls live in `src/spx/components/` alongside the product shell.

Treat the template clone as a read-only reference. For each requested feature, copy its page or block and the necessary local components, types, data and dependencies; reuse existing project UI and theme tokens. Do not overwrite project configuration, lockfiles, identity/session code or already customized components. Preserve required licenses and attribution. Compare the selected feature with the fixed template version and verify its interactions after integration.

Replace sample data with the owning module's generated API client when that business feature is implemented. IAM APIs stay with IAM; Console owns security operations and useful operational query aggregation, not duplicate wrappers around every module. See [Console boundaries](../console/README.md).

## Internationalization

The application uses `next-intl` with `en` and `zh-CN` catalogs in `messages/`. English is the default. A direct English/Chinese toggle is available on sign-in, authenticated pages, service-error and not-found views. The validated `spx-locale` preference cookie is separate from IAM credentials and is written by a Server Action; Next updates the current UI and layouts after the cookie changes. Routes and API contracts keep their existing paths without locale prefixes.

`src/i18n/config.ts` owns supported locales, defaults and the display time zone. `request.ts` loads the matching catalog per request. The root provider shares that configuration with client components, and `html lang`, metadata and page titles follow the same locale. Dashboard is the English page name; 安全智驾工作台 is the Chinese name.

Use typed `useTranslations` / `getTranslations` message keys and ICU placeholders or plurals. Add every message to both catalogs; do not concatenate sentence fragments or add locale conditionals to components. Format numbers, money, lists and timestamps with `useFormatter`; displayed timestamps use UTC consistently across both languages. Currency is preserved (the CRM sample uses USD), not converted when switching languages. Pass the locale into components such as the date picker.

Translate platform text, stable permission labels and built-in role names. Preserve account names, organization names, custom role names, identifiers and other user data. API errors use stable result-code-to-message-key mappings; never translate or display raw internal error bodies. CRM sample labels retain their sales meaning: leads are 销售线索 and won projects are 成交项目, not invented security metrics.

Unit checks cover catalog parity, ICU interpolation/plurals, and actual Dashboard component rendering in both languages. Existing IAM E2E selects Chinese explicitly, so it does not depend on the default locale. Run the local unit/type/build checks after adding translations; browser checks should verify switching, refresh persistence, translated accessibility labels, metadata and narrow layouts.

## Typed API, query and form boundaries

`src/spx/api/client.ts` uses Axios interceptors for platform results, bounded requests, cancellation and safe errors. `api/iam/` owns native API functions and consumed Zod models; TypeScript types are inferred from those models. Routes and models are synchronized with Go through real HTTP and browser tests. There is no API code-generation prerequisite. Internal IAM authorization remains server-only.

`spx/iam/session.ts` configures Axios identity/CSRF behavior before use: same origin, CSRF, bounded requests, identity cancellation and safe HTTP errors. `query.tsx` configures explicit retry/stale/gc/focus/reconnect policies; cache keys include session, identity and organization authorization versions. Logout/organization changes cancel and clear caches. Password login, confirmation, change and reset call module API functions directly and never enter MutationCache or persistent storage.

IAM lists use Query and the existing TanStack Table with controlled server pagination. RHF uses authored Zod schemas and existing shadcn Field/FieldError controls; business pages declare fields explicitly. There is no string-map form DSL or effect/revision list fetcher. See the [role implementation walkthrough](../../../docs/development/role-management.md).


## 可信入口与协议边界

直接使用根 `pnpm dev` / `pnpm preview`。根 `.env` 控制公开 origin 和监听地址，后端地址由实际 Go 配置推导；不需要安装独立入口或配置内部 Web 端口。详见[配置说明](../../../docs/configuration.md#nextjs-web)。

本机链路为浏览器 → Next → IAM/Console。Next 不透传客户端来源头；当前来源限流按代理连接来源计数，多人公网真实来源隔离尚未实现。Cookie/CSRF、Allow/Retry-After、多 Set-Cookie 与 SSE 保留各自协议，IAM API 不绕行 Console。

Axios 负责传输、JSON、超时和取消，平台拦截器按模块 API 声明核对成功状态和 Zod 消费模型；204 校验空正文；HTML 或未声明成功状态不能触发 Mutation 成功。身份范围由会话/授权版本和能力决定，变更时统一取消请求并清缓存，二次认证开始及成功切换均隔离旧请求，错误密码保留有效身份。


## Settings and administration experience

The avatar menu exposes Settings and System. Settings has separate Profile and Account routes under the same application shell. Device management is not exposed. System has three task sections: Members & access, Organization management and Audit logs. Capability-filtered tabs retain the existing management deep links and distinguish organization membership from platform account administration; merging navigation never grants access.

The read-only system security overview and its unused API/permission were removed. Session policy, authentication, authorization and personal security controls remain. Organization and form pickers use the existing shadcn Select; toolbar actions use ghost buttons. Semantic status badges share success, danger, warning and information colors across both themes, with visible text. Categories remain neutral. This iteration targets desktop browsers; it does not add mobile-specific layouts or tests. See the [settings plan](../../../docs/plans/settings-experience/spec.md).


The single system administrator is configuration-managed. Its identity, profile, status, membership and password cannot be changed through the application or operator password-reset command. Personal settings shows a read-only explanation; account/member lists omit mutation actions for that user. Other accounts keep their normal password and membership workflows. IAM applies its existing bootstrap username/password configuration during explicit initialization and startup, rotating identity versions only when configuration changes. No UI can grant or remove the system administrator identity.

Data tables default to 10 rows, with 10/20/30/40/50 rows-per-page controls. Server pagination includes page size in the scoped query key and resets to the first page when the size changes. Selection catalogs are not paginated data tables.

UI copy avoids repeating page titles, tab labels and field purposes as permanent descriptions. Status belongs to the corresponding object or details field. Necessary constraints and recovery guidance appear at the relevant control; configuration-managed password guidance is available on hover or keyboard focus. Localized copy follows professional product terminology and the conventions of each language.

Member selection uses the template's existing Combobox with server search and TanStack infinite queries (10 options per page). Scrolling loads subsequent pages; disabled members cannot be selected. Identity confirmation happens inside the active FormDialog, retaining its RHF draft and selected member. A successful confirmation refreshes the session scope without unmounting the page; the member explicitly submits the original form afterward. Confirmation passwords are not sent through MutationCache, persisted, logged, or replayed. The standalone Account confirmation action is removed. Organization name and identifier limits follow the IAM README.

## Offline configuration check

Root startup injects the shared environment; run `pnpm config:check` from the repository root before starting services. For standalone Web, copy this module’s `.env.example` to `.env.local`, then run `pnpm config:check` in the module directory. Use `pnpm config:check --development` to include `.env.development*`; the default checks the production environment used by `start`. This validates listener and upstream fields without starting Next.js or connecting to IAM/Console. Restart after environment changes. See [configuration](../../../docs/configuration.md) for required values and service boundaries.
