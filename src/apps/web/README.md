# SecPlexus Web

React + Arco Design, initialized from the official
[Arco Design Pro Vite template](https://github.com/arco-design/arco-design-pro/tree/main/arco-design-pro-vite).
The upstream template is assembled with shared files from `arco-design-pro-next`,
following the copy map in its `scripts/vite.js`. Layout, login, dashboard, table,
form, visualization, and settings pages are retained for gradual adaptation.
Original MIT terms are preserved in [LICENSE.arco-design-pro](LICENSE.arco-design-pro).

## Toolchain

Current stable React 19, Vite 8, TypeScript 7, React Router 7, Redux 5 /
React Redux 9, ECharts 6, Less, and Biome. Exact versions are recorded in `package.json`
and the root `pnpm-lock.yaml`. React's native lazy/Suspense replaces the legacy
loadable wrapper; Vite's official React plugin provides Fast Refresh.
The template's BizCharts renderer was replaced with modular ECharts because its
React 16 reconciler is incompatible with React 19. The legacy color picker was
replaced by Arco's ColorPicker, and Arco's React 19 adapter is loaded at startup.

## Commands

Run `make dev` from the repository root for dependency installation and startup.
`make build` creates the production build. `make stop` stops running project
services, including Web started directly through pnpm in development or preview mode.

For additional checks, run `pnpm typecheck` and `pnpm lint` from the repository
root. The latter checks formatting with Biome.

## Mock boundary

Mock data is enabled only in Vite development mode, unless
`VITE_ENABLE_MOCK=false` is set. Production builds never enable mock handlers.
Demo login is `admin` / `admin`. This is template navigation, not real IAM
integration; mock permission checks are not a backend security boundary.
The login form remembers only the username, never the password.

To connect a real backend, copy `.env.example` to `.env.local`, set
`VITE_ENABLE_MOCK=false` and `VITE_API_PROXY_TARGET` to the backend address.
There is no backend implemented by this change. Never put secrets in `VITE_`
variables: Vite exposes these values to browser code.

## Theme

The visual reference is [Arco theme 1376, 火山引擎](https://arco.design/themes/preview/1376).
The app uses the current Arco React Less styles with the overrides in
`src/theme.ts`. It does not stack the Pro component theme or the Vue theme on top.
The original Pro pages and their license are retained.

- `src/settings.json`: primary color, navigation visibility and menu width.
- `src/theme.ts`: shared component tokens, including the 4/6/12px radius scale.
- `src/style/layout.module.less`: the page canvas and application layout.
- `src/components/Chart/`: ECharts implementations of the original chart designs.

Vite applies the same Less tokens in development and production, with
`math: 'always'` for Arco's existing size expressions. Keep input focus, error,
warning and disabled styling in Arco's component tokens; do not add global
outline or border overrides. Newer components that are absent from the old
reference preview use the current Arco React defaults and shared theme tokens.

The reference preview uses Arco React 2.24.1. This app keeps its current React
component version and its behavior fixes; it does not copy the old compiled CSS.
See the [theme audit](../../../docs/development/theme-audit-1376.md) for the
reference, changes and verification scope.

## Brand assets

`src/components/Brand` is shared by the login header, login form and navigation.
It uses a locally hosted Space Grotesk font; its OFL notice is in
`src/assets/fonts/OFL.space-grotesk.txt`. The three product illustrations live
in `src/pages/login/assets` and load without third-party image requests.

## Project skill

Use [spx-arco-design](../../../.agents/skills/spx-arco-design/SKILL.md) for
component references. Keep npm package names as `@arco-design/web-react`.

## Template limitations

The upstream Arco components still emit an element-ref deprecation message in
React 19 development mode. Browser acceptance checks found no uncaught runtime
errors. Some demo illustrations still use upstream-hosted media. The retained
admin templates primarily target desktop screens; the login page also fits
narrow screens. The complete demo build still has large shared chunks, to be
revisited as product pages replace the examples.
