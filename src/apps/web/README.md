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

Root startup reads the root `.env`, derives backend addresses, and supplies `SPX_WEB_IAM_PROXY_TARGET` and `SPX_WEB_CONSOLE_PROXY_TARGET` only to the Next server. `SPX_WEB_HOST` controls the bind address; the browser port comes from `SPX_PUBLIC_URL`. `make dev` applies IAM migrations against configured development infrastructure; it never starts Docker.

Root builds also load the root `.env` when present and pass filtered Web configuration. `SPX_PUBLIC_URL` supplies the default public metadata origin, with an optional `NEXT_PUBLIC_APP_URL` override. Process values take precedence. Public values are fixed by the production build, so rebuild after changing them; runtime listener or proxy changes require a restart. A build can run without creating a development `.env`.

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

## Typed API, query and form boundaries

`src/spx/api/client.ts` uses Axios interceptors for platform results, bounded requests, cancellation and safe errors. `api/iam/` owns native API functions and consumed Zod models; TypeScript types are inferred from those models. Routes and models are synchronized with Go through real HTTP and browser tests. There is no API code-generation prerequisite. Internal IAM authorization remains server-only.

`spx/iam/session.ts` configures Axios identity/CSRF behavior before use: same origin, CSRF, bounded requests, identity cancellation and safe HTTP errors. `query.tsx` configures explicit retry/stale/gc/focus/reconnect policies; cache keys include session, identity and organization authorization versions. Logout/organization changes cancel and clear caches. Password login, confirmation, change and reset call module API functions directly and never enter MutationCache or persistent storage.

IAM lists use Query and the existing TanStack Table with controlled server pagination. RHF uses generated Zod schemas and existing shadcn Field/FieldError controls; business pages declare fields explicitly. There is no string-map form DSL or effect/revision list fetcher. See the [role implementation walkthrough](../../../docs/development/role-management.md).


## 可信入口与协议边界

直接使用根 `pnpm dev` / `pnpm preview`。根 `.env` 控制公开 origin 和监听地址，后端地址由实际 Go 配置推导；不需要安装独立入口或配置内部 Web 端口。详见[配置说明](../../../docs/configuration.md#nextjs-web)。

本机链路为浏览器 → Next → IAM/Console。Next 不透传客户端来源头；当前来源限流按代理连接来源计数，多人公网真实来源隔离尚未实现。Cookie/CSRF、Allow/Retry-After、多 Set-Cookie 与 SSE 保留各自协议，IAM API 不绕行 Console。

Axios 负责传输、JSON、超时和取消，平台拦截器按模块 API 声明核对成功状态和 Zod 消费模型；204 校验空正文；HTML 或未声明成功状态不能触发 Mutation 成功。身份范围由会话/授权版本和能力决定，变更时统一取消请求并清缓存，二次认证开始及成功切换均隔离旧请求，错误密码保留有效身份。
