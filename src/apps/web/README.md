# SecPlexus Web

## Routes and real identity

| Route | Purpose |
| --- | --- |
| `/login` | Real IAM username/password login and mandatory password change |
| `/dashboard` | Authenticated security workbench based on CRM V1; template sample data |
| `/ingest` | Console data ingestion workspace, opening the first permitted section |
| `/ingest/inputs` | Inputs, connection setup and per-input reception diagnostics |
| `/ingest/parsing-rules` | Parsing rules, input references, field editor and sample validation |
| `/settings/profile` | Profile details, account security and display time zone |
| `/settings`, `/profile` | Permanent redirects to `/settings/profile` |
| `/settings/users` | Users, their roles and access administration |
| `/settings/roles` | Roles, permissions and permission previews |
| `/settings/audit` | Audit log |

The product shell uses the shared Sidebar, form, dialog and table components, plus its existing layout controls and theme switcher. Navigation follows the server capability projection. The single workbench retains CRM V1 sample data and never counts as real business statistics. The Dashboards group contains only “安全运营工作台”; real IAM navigation remains capability-based.

IAM credentials stay in HttpOnly cookies. Zustand holds only the current in-memory identity and capability projection. No session, token, role, permission or business data is persisted in localStorage; only a remembered username and presentation preferences may persist. Identity versions and cancellation reject stale responses after logout or identity changes. BroadcastChannel synchronizes session changes across tabs without sharing secrets.

`src/app/api/v1/[module]/[...path]/route.ts` streams IAM, Console and Ingest API requests to server-configured origins. It forwards Cookie, Origin and CSRF context, preserves response status and Set-Cookie, and forwards the device bearer token only for the Ingest event push endpoint. Service credentials stay with the Go applications. Backend authorization remains authoritative. A failed collection request keeps the current session and navigation available. A 503 from the IAM identity or CSRF context pauses the shell until the user reconnects.

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

Root startup reads the root `.env`. See the [shared template](../../../.env.example) for deployment settings. `SPX_IAM_HOST/PORT` and `SPX_CONSOLE_HOST/PORT` supply server-only connection locations to every caller. `SPX_WEB_PORT` controls the HTTP listener when running from source and the host port in Compose. `SPX_WEB_ORIGIN` is an IAM security input describing the trusted browser entry; Web metadata does not need an absolute origin. Application servers use HTTP; Nginx or another reverse proxy owns HTTPS and certificates.

Web startup loads native environment files with official `@next/env`, then invokes the Next CLI on the configured port, defaulting its bind address to `0.0.0.0`. Standalone Web uses the module `.env.example` → `.env.local` with the same keys. Root startup already injects them, so a second file is unnecessary. Root `pnpm dev web` / `pnpm preview web` starts only Web against existing services, without Go migration or startup.

Root builds use the same filtered environment, falling back to non-secret example settings when no private file exists. Web never receives database/Redis credentials, administrator passwords or service credentials. Web does not receive the IAM origin setting or maintain a separate metadata URL. Restart after changing runtime configuration.

For independent Web development, copy [.env.example](.env.example) to `.env.local`, configure the server-side upstream targets, and run `pnpm dev` here. Next.js loads the module environment files, and the template sets port 5173. Keep IAM's public origin consistent with the browser entry. Never place secrets in `NEXT_PUBLIC_*`.

Production uses `next build` followed by `next start`, including server components and API routes. It is not a static export. The Web Dockerfile runs Next.js as an unprivileged Node user. See [configuration](../../../docs/development/configuration.md) and [testing](../../../docs/development/testing.md).

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

## Console layout

Every authenticated route inherits its content frame from the `(secplexus)` layout and `Shell`. The toolbar and page share the `console-frame` rule in `globals.css`. Put the page header, toolbar, table or editor directly inside that frame. Use responsive grids for the page body and local width limits only for controls, code or reading columns. The root `data-content-layout` preference applies on navigation, on reload and while the sidebar changes width.

The dashboard template's root `globals.css`, `SidebarInset`, `Card`, Dialog and Sheet define the surface roles. SecPlexus assigns the canvas to the shell and gutters, and a grey content surface to the sidebar, cards, `ListSurface`, detail bodies and editor bodies. Menus and overlays get their background from the shared popover token, including when mounted in a portal. Table headers and rows inherit their list's background. Reserve muted surfaces for subgroups and blue for primary actions, focus and selection. Theme settings and the document root drive both modes, including toasts and browser form chrome. [Surface and token mapping](../../../DESIGN.md#secplexus-implementation-mapping)

The browser suite toggles the actual appearance control and measures tables in both the ingestion workspace and IAM settings. See the [design mapping](../../../DESIGN.md#console-content-width).

### Resource states and side panels

Wrap remote collections in `CollectionBoundary` from `spx/components/collection-state.tsx`. Pass the TanStack query and whether the current view has usable content. Initial loading or failure occupies the content region under the toolbar. A failed background refresh keeps the records or draft in view and offers a scoped retry. For an empty result use `CollectionState` with `kind="empty"` and an action appropriate to the user's permissions. For a search with zero matches use `kind="filtered"` and supply `onClear`. Form validation belongs next to its field.

Use React Hook Form's default submit-time validation. A blank field stays neutral when users tab through a form. A wizard validates required fields when users advance. Recheck a field with a visible error as users edit it.

Use `SheetPanel` for side-by-side editing and details, or `FormDialog variant="sheet"` for an RHF form. The sheet primitive reads `--sheet-inline-size`. Header, scrollable body and footer share alignment, standard field groups switch to two columns when the panel has room, and keyboard focus returns to the opener. Mobile navigation sets its own navigation width via the same token. Dimensions and breakpoints are recorded in DESIGN.md.

## Selective template adoption

Use the corresponding block from `next-shadcn-admin-dashboard`. The selected reference implementations live under its `src/app/(main)/dashboard` directory.

| Surface | Template block | SecPlexus adaptation |
| --- | --- | --- |
| Workbench and page titles | CRM V1 workbench header | `PageHeader` and `PageTitle` share the workbench typography across routes. |
| Profile | `profile/_components/profile-header.tsx`, `profile-overview.tsx` and `profile-personal-details.tsx` | Breadcrumb, identity header, line tabs and overview rail. The IAM session supplies profile contact, creation date, assigned roles and display time zone. The overview rail shows the current session. Password and login has its own tab. |
| Users | `users/_components/users.tsx`, `users-table.tsx` and `users-columns.tsx` | Card header, a toolbar with search followed by role and status filters, selection, column visibility, list/grid views, CSV export, and the shared list pager. Export includes selected rows or the current page. IAM supplies page results and creation dates. |
| Roles | `roles/_components/roles.tsx` and `roles-table/{table,columns}.tsx` | The role list toolbar puts search and type on the left and Create role on the right. The permission catalog tab, grouped permission chips, and shared list pager follow the template layout. |
| Audit logs | `tasks/_components/tasks.tsx` and `tasks-toolbar.tsx` | Compact filter toolbar, padded event rows and cursor pagination. On narrow layouts, the actor appears beneath the timestamp and the request ID beneath the action. |
| Profile and IAM forms | `products/_components/add-product-drawer.tsx`, shadcn Alert Dialog | Short forms use a dialog. User and role details and long forms use the Product drawer's 32rem maximum width, aligned header and body, and a scrollable form body. Editable forms keep their footer in reach. Deletion uses an alert dialog with the record name. |
| Input and parsing-rule lists | `roles/_components/roles.tsx`, `products/_components/products.tsx` | Header actions, a bordered list surface, compact search and filters, muted table headers and row actions. Keep server pagination and permission checks. |
| Input setup and diagnostics | Layout Controls option groups, searchable Combobox, Profile detail rows | A single form spans source, connection and parser settings, then review. Only the review action creates the input. The result and detail share the receiver diagnostics. The HTTP token expires from the view on dismissal or reload. |
| Parsing-rule wizard | Product Form field groups, Tasks toolbar, Invoice editable rows and preview, standalone Mail list/details, Infrastructure collapsible groups | Setup collects raw samples. Extraction shows filtered sample fields and a per-field processing sheet. Mapping uses grouped, keyboard-searchable field choices across the v1 catalog. The Mail pattern keeps selection and result visible together on wide screens, with a sample selector on narrow screens. Classification is an optional section. |

Business references guide the setup, validation and diagnosis workflow. Render it with the template components and project tokens. Add only controls backed by a real action. The template's MIT attribution is retained in [Third-party notices](THIRD_PARTY_NOTICES.md). Web lint disallows raw page headings and direct Sheet/Empty imports in business pages. The existing layout E2E compares the workbench's computed title typography across routes.

## List controls and navigation

Business tables use shared Table slots for left-aligned headers and data. Row controls align to the same edge. The cross-route browser test checks the rendered columns in the workbench, settings and ingest lists.

The Data ingestion sidebar group has two links: Data ingestion and Parsing rules. Each requires its own read permission. Creation, details and edit routes highlight their parent. The group disappears when neither list is visible. The legacy entry and settings bookmarks retain their redirects.

| Concern | Implementation | Data behavior |
| --- | --- | --- |
| Search and choices | `SearchField`, `ListSelect`, `ListSearchTrigger`, `ListSearchContent`, `ListSearchOption`, `ListDateFilter` | Long search text uses an ellipsis and retains its full accessible label. Searchable filters share the compact Select trigger styling. The popup opens with an empty search field and single-line choices that truncate when needed. Ingest filters and paginates on the server. IAM directories support debounced search and immediate submission with Enter. Calendar dates preserve the selected day in the API. |
| Results and actions | `ListSurface`, `Toolbar`, shadcn Table, Button and DropdownMenu, `CollectionBoundary` | Show editable fields, last receipt and failure counts from real input records. Parsing rules expose format, event type, source, update time and data input counts when authorized. Use ListRecordLink for long record names. Use the named list container to adapt columns to the space remaining beside local navigation. At narrow widths, keep the name and row actions on screen. For rules, keep source and data input count alongside them. Open the name to inspect a record and use the row menu to edit. |
| Paging | `ListPagination` | The parsing-rule footer is the baseline for all lists. Known totals appear on the left, with page size, position and navigation on the right. Audit uses issued cursors and shows position with previous/next. Inputs and rules page the filtered API collection and retain query, filters and position in the URL. Changing a filter or page size starts on the first page. |

Biome rejects raw business-page controls and bypass imports. This follows the approach of [Primer React's design-system lint rules](https://github.com/primer/eslint-plugin-primer-react). Browser checks also open the result filter, apply it to the audit request, follow the link from a rule to its inputs, and enter the input editor from the row menu.

### Input and rule workflows

Inputs display their configured state separately from reception evidence. The details page groups identity, protocol settings, the bound rule and current-process reception statistics. Bulk enable and disable apply to selected inputs on the current page. The confirmation identifies the affected subset. Failed rows stay selected for retry.

Search parsing rules by name, description, vendor, or product and filter by event type, format, or origin. Search data inputs by name, vendor, or product and filter by parsing rule, protocol, or status. Both management lists default to 20 rows and keep their filters and page in the URL. A rule name opens its four-step read-only view. Create and edit use the same wizard sections and controls. The saved-sample preview uses rule read permission. Draft preview uses rule write permission. The data input count uses input read permission and opens the input list with that rule selected. The input filter shares the binding picker's remote search. Layout follows the template's Invoice form and Tasks toolbar.

The two lists retain the local dashboard template's toolbar, table and pagination structure. Input creation and details live in their own components. The parsing-rule wizard opens independently of its list, with setup, extraction, mapping and review steps. Each available step is a full-width control with a centered label. The step and footer action use the same checks. Setup shows the options for the selected parser type. Optional source encoding is under Advanced settings in the extraction step. The wizard keeps samples and drafts in the active session. Both extraction and complete validation use authenticated endpoints, with requests cancelled or discarded when their draft changes. Choices come from the current v1 directory; the browser caches it using the existing identity-scoped TanStack Query lifecycle. Template routes are evaluated by operator task: Products and Roles for listings, Tasks for search, Profile for details, Mail for selecting and reading a sample, Invoice for adaptive editable rows, and Infrastructure for grouped operational status.

Extraction and field mapping display three-line read-only samples beside the main work area. Focus a sample to inspect its values. Process text fields in the compact SheetPanel used for management forms, and add output mappings in the mapping step. Revalidate after changing the mapping or samples. The rule editor uses one definition. Saving requires a successful sample run.

Rule descriptions, regular expressions, editable log samples and read-only sample previews use the shared three-line Textarea. Long content scrolls inside the field. The first step checks JSON with the browser parser and uses Ingest extraction for all supported formats. Format errors appear under the affected sample.

### Event field catalog and release compatibility

Ingest owns the versioned event catalog. The rule editor uses the same-origin class and field endpoints to show a searchable target path, the localized name, field type, cardinality, requirement and enum meanings. The selected field and source sample drive the mapping preview. Web owns layout and interaction copy. Storage projection and release procedure are documented in the Ingest module. [API client](src/spx/api/ingest/event-schema.ts) · [Ingest README](../ingest/README.md#event-字段与存储设计)

Class details are cached by catalog revision and class within the existing identity boundary. A catalog request failure leaves the draft available and provides retry. The form uses the contract version from the selected class detail. Release the compatible Ingest API before the Web editor and verify authorized and denied requests, field selection, sample validation, saving and reopening with a real session.

## Internationalization

The application uses `next-intl` with `en` and `zh-CN` catalogs in `messages/`. Simplified Chinese is the default. A direct English/Chinese toggle is available on sign-in, authenticated pages, service-error and not-found views. The validated `spx-locale` preference cookie is separate from IAM credentials and is written by a Server Action. Next updates the current UI and layouts after the cookie changes. Routes and API contracts keep their existing paths without locale prefixes.

`src/i18n/config.ts` owns supported locales and defaults. `request.ts` loads the matching catalog and resolves the display time zone per request. The root provider passes the request configuration to the entire application, including dialogs and menus. Preserve next-intl diagnostics for missing or invalid messages. The shared server and client fallback display `—` in place of an internal key. A list filter can retain its already-localized field and option labels when shared formatting copy is missing.

Use typed `useTranslations` / `getTranslations` message keys and ICU placeholders or plurals. Add every message to both catalogs. Format numbers, money, lists and timestamps with `useFormatter`. Timestamps use the account or browser display time zone. Currency is preserved when switching languages. Pass the typed locale into locale-aware controls and localized API data.

Translate platform text, stable permission labels and built-in role names. Preserve account names, custom role names, identifiers and other user data. API errors use stable result-code-to-message-key mappings; never translate or display raw internal error bodies. CRM sample labels retain their sales meaning: leads are 销售线索 and won projects are 成交项目, not invented security metrics.

Unit checks cover catalog parity, ICU interpolation, fallback and actual component rendering in both languages. The browser flow switches languages, navigates between Console and Settings, opens select menus and a profile dialog, and checks for visible namespace keys and next-intl errors. For new workflows, verify translated controls, accessibility labels and narrow layouts alongside unit, type and build checks.

## Typed API, query and form boundaries

`src/spx/api/client.ts` uses Axios interceptors for platform results, bounded requests, cancellation and safe errors. `api/iam/` owns native API functions and consumed Zod models; TypeScript types are inferred from those models. Routes and models are synchronized with Go through real HTTP and browser tests. There is no API code-generation prerequisite. Internal IAM authorization remains server-only.

`spx/iam/session.ts` configures Axios identity/CSRF behavior before use: same origin, CSRF, bounded requests, identity cancellation and safe HTTP errors. `query.tsx` configures explicit retry/stale/gc/focus/reconnect policies; cache keys include session and identity versions. Logout or identity changes cancel and clear caches. Password login, confirmation, change and reset call module API functions directly and never enter MutationCache or persistent storage.

IAM lists use Query and the existing TanStack Table with controlled server pagination. RHF uses authored Zod schemas and existing shadcn Field/FieldError controls; business pages declare fields explicitly. There is no string-map form DSL or effect/revision list fetcher. See the [role implementation walkthrough](../../../docs/development/role-management.md).


## 可信入口与协议边界

直接使用根 `pnpm dev` / `pnpm preview`。根 `.env` 控制公开 origin 和监听地址，后端地址由实际 Go 配置推导；不需要安装独立入口或配置内部 Web 端口。详见[配置说明](../../../docs/development/configuration.md)。

本机链路为浏览器 → Next → IAM/Console。Next 不透传客户端来源头；当前来源限流按代理连接来源计数，多人公网真实来源隔离尚未实现。Cookie/CSRF、Allow/Retry-After、多 Set-Cookie 与 SSE 保留各自协议，IAM API 不绕行 Console。

Axios 负责传输、JSON、超时和取消，平台拦截器按模块 API 声明核对成功状态和 Zod 消费模型；204 校验空正文；HTML 或未声明成功状态不能触发 Mutation 成功。身份范围由会话/授权版本和能力决定，变更时统一取消请求并清缓存，二次认证开始及成功切换均隔离旧请求，错误密码保留有效身份。


## Settings and administration experience

The avatar menu opens System settings at `/settings/profile`. The other entries are `/settings/users`, `/settings/roles` and `/settings/audit`. Each page starts with a shared breadcrumb whose System settings parent is plain text. Navigation and route access use the IAM capability projection.

The profile time-zone dialog writes the preference to IAM and refreshes the session and date display. Changing the password signs the user out. Users and roles use server search and bounded offset pagination. User details open in view or edit mode. Identity, status, role assignments and password reset each have an independent submission. Role metadata and permissions save independently. The role details item shows its full permission list. Read-only operators can open details from the row menu. Built-in roles are read-only and referenced roles cannot be deleted.

The read-only system security overview and its unused API/permission were removed. Session policy, authentication, authorization and personal security controls remain. Role and form pickers use the existing shadcn Select. Use the primary button for the page action and the outline variant for filter reset. Semantic status badges share success, danger, warning and information colors across both themes, with visible text. Categories remain neutral. Filters wrap at narrow widths and wide tables scroll within the list surface.


The configured administrator is created from IAM configuration and holds the built-in administrator role. IAM applies the bootstrap username and password during explicit initialization and startup, rotating identity versions when configuration changes. Profile explains why password changes are locked. User management locks identity, status, password reset and deletion for this account. IAM rejects disabling, deleting and resetting the account, keeps its required role assigned, and checks delegation and administrator availability when updating other assignments.

Data tables default to 10 rows, with 10/20/30/40/50 rows-per-page controls. Server pagination includes page size in the scoped query key and resets to the first page when the size changes. Selection catalogs are not paginated data tables.

UI copy avoids repeating page titles, tab labels and field purposes as permanent descriptions. Status belongs to the corresponding object or details field. Necessary constraints and recovery guidance appear at the relevant control; configuration-managed password guidance is available on hover or keyboard focus. Localized copy follows professional product terminology and the conventions of each language.

User selection uses the template's existing Combobox with server search and TanStack infinite queries (10 options per page). Scrolling loads subsequent pages; disabled users cannot be selected. Identity confirmation happens inside the active FormDialog, retaining its RHF draft and selected user. A successful confirmation refreshes the session projection without unmounting the page; the operator explicitly submits the original form afterward. Confirmation passwords are not sent through MutationCache, persisted, logged, or replayed. The standalone Account confirmation action is removed. Account and role limits follow the IAM README.

## Offline configuration check

Root startup injects the shared environment. Run `make config` from the repository root before starting services. For standalone Web, copy this module’s `.env.example` to `.env.local`, then run `pnpm config:check` in the module directory. Use `pnpm config:check --development` to include `.env.development*`. The check validates listener and upstream fields without contacting services. The same-origin Route Handler resolves IAM, Console and optional Ingest targets at runtime. Set `SPX_INGEST_HOST/PORT` together or use `SPX_INGEST_ORIGIN` for an explicit HTTP(S) origin. Restart Web after an upstream change. See [configuration](../../../docs/development/configuration.md).

## 依赖边界

`shadcn` 包是**构建期依赖**：`src/app/globals.css` 直接 `@import "shadcn/tailwind.css"` 取设计令牌，
删掉它会让 `next build` 失败；组件的交互式生成仍用 `pnpm dlx shadcn@latest add <component>`，
不把 CLI 当项目依赖固定版本。

### 数据接入与规则工作区

数据接入和解析规则是独立菜单。接入向导依次选择来源、填写连接、验证样本和确认。步骤与主表单同宽，在宽屏旁置实时摘要，窄屏随步骤纵向排列。详情用分隔线组织连接、来源和接收诊断，短配置复用 `SheetPanel`。查看规则可从该详情直接进入。

规则工作区按内容宽度安排配置和样本结果。先看命中分支及事件类型，再对照源字段、提取值、标准字段和归一化值。字段 Combobox 读取对应契约版本，数字编码放在字段详情或输出预览。内置规则使用四步只读向导，可复制为自定义规则。临时样本保留在表单会话内。列表和表单的模板对应关系见 [页面线框](../../../docs/plans/ingest-mixed-event-parsing/plan.md#页面线框)。

规则描述、正则表达式、日志样本输入和只读展示共用三行文本框，长内容在框内滚动。基础配置用浏览器检查 JSON 样本，并通过 Ingest 提取接口验证各解析类型。格式问题显示在对应样本下方。

Chromium 验收使用独立运行目录。配置隔离的 `SPX_TEST_CLICKHOUSE_URL` 后可用 `pnpm --filter @secplexus/web test:e2e --clickhouse` 加跑ClickHouse 事件回读。已配置的测试 PostgreSQL、Redis 和 Kafka 继续复用，测试资源按本次 ID 回收。


### 解析规则修订

规则历史使用模板 Mail 的主从结构，左侧按修订选择，右侧查看验证摘要及变更。复用 `SheetPanel`、`IngestConfirm` 和当前规则详情，窄屏按顺序展示。更改内容按需展开。恢复确认展示受影响的接入数量并使用当前规则版本防止覆盖编辑。
