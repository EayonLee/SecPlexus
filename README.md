# SecPlexus

An AI-native security operations platform.

[English](README.md) | [简体中文](README.zh-CN.md)

<div align="center">
  <img src="https://img.shields.io/badge/Go-1.27.3-00ADD8?logo=go&amp;logoColor=white" alt="Go 1.27.3" />
  <img src="https://img.shields.io/badge/Gin-1.12.0-00ADD8" alt="Gin 1.12.0" />
  <img src="https://img.shields.io/badge/Next.js-16.4.3-000000?logo=nextdotjs&amp;logoColor=white" alt="Next.js 16.4.3" />
  <img src="https://img.shields.io/badge/React-19.3.1-61DAFB?logo=react&amp;logoColor=black" alt="React 19.3.1" />
  <img src="https://img.shields.io/badge/TypeScript-5.9.3-3178C6?logo=typescript&amp;logoColor=white" alt="TypeScript 5.9.3" />
  <img src="https://img.shields.io/badge/Tailwind_CSS-4.3.3-06B6D4?logo=tailwindcss&amp;logoColor=white" alt="Tailwind CSS 4.3.3" />
  <img src="https://img.shields.io/badge/GORM-1.31.2-EF4444" alt="GORM 1.31.2" />
  <a href="https://github.com/EayonLee/SecPlexus/stargazers">
    <img src="https://img.shields.io/github/stars/EayonLee/SecPlexus?style=flat&amp;logo=github" alt="SecPlexus GitHub stars" />
  </a>
</div>

## Development

Install Go 1.27.3, Node.js 22.12+ (24 recommended), and pnpm 10.32.1.

```sh
cp .env.example .env
pnpm certs:generate
# Fill the database/bootstrap passwords, CSRF key and internal token in .env.
# Use existing PostgreSQL/Redis, or explicitly deploy them:
pnpm compose:up postgres redis
make dev
```

`make dev` runs IAM, Console, and Web with hot reload. It connects to the configured
services and never starts Docker. Missing `.env` is created from the example;
startup stops so you can review it. Existing configuration is never overwritten.

```sh
make build       # Build both Go services and Web
pnpm preview     # Run services and preview built Web; configure the matching public origin
pnpm compose:up  # Explicitly build and deploy the complete stack
make stop        # Stop this checkout's applications and containers; retain data
```

IAM provides real login, voluntary password changes, revocable sessions, organizations,
custom roles, and audit records. The initial administrator defaults to `admin/secplexus` for an empty account database.
Override its initial password with SPX_IAM_BOOTSTRAP_PASSWORD.
The management center and personal security pages use real APIs. Console currently
verifies organization access and SSE authorization through IAM; business workflows
are not implemented. Web uses the Next.js/shadcn baseline with one CRM V1-based intelligent security workbench at `/dashboard` and real IAM pages at `/login`, `/account` and `/management`. Other template pages and menus are removed; the workbench currently displays sample data. The Web interface supports English and Simplified Chinese through next-intl.

See [configuration and startup](docs/configuration.md), [testing](docs/testing.md),
[IAM architecture](src/apps/iam/README.md), and [Web](src/apps/web/README.md).

Go modules use plain `config.yaml`, Viper/mapstructure for explicit environment bindings and strict typed decoding, and Cobra commands.
Local rotating JSON logs use `logs/iam.log` and `logs/console.log`; containers use stdout.

## Repository layout

```text
src/apps/       Application code (web, iam, console)
src/packages/   Shared libraries, added as needed
src/contracts/  Interface contracts
deploy/         Deployment configuration
scripts/        Development and release scripts
docs/           Project documentation
```

## Contributing

See the [contribution guide](CONTRIBUTING.md) for development setup, architecture
boundaries, testing, and the pull request workflow.

## Contributors

Thanks to everyone who contributes to SecPlexus.

<p align="center">
  <a href="https://github.com/EayonLee/SecPlexus/graphs/contributors">
    <img src="https://openomy.app/svg?repo=EayonLee/SecPlexus&amp;chart=bubble" alt="SecPlexus community contributions" width="100%" />
  </a>
</p>

## Acknowledgements

Thanks to the maintainers and contributors of these projects.

<p>
  <a href="https://github.com/arhamkhnz/next-shadcn-admin-dashboard" title="next-shadcn-admin-dashboard">
    <img src="docs/assets/arhamkhnz.png" alt="next-shadcn-admin-dashboard" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/shadcn-ui/ui" title="shadcn/ui">
    <img src="docs/assets/shadcn-ui.png" alt="shadcn/ui" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/redis/redis" title="Redis">
    <img src="docs/assets/redis.svg" alt="Redis" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/postgres/postgres" title="PostgreSQL">
    <img src="docs/assets/postgresql.png" alt="PostgreSQL" height="64" />
  </a>
  &nbsp;&nbsp;
  <a href="https://github.com/BerriAI/litellm" title="LiteLLM">
    <img src="docs/assets/litellm.svg" alt="LiteLLM" height="64" />
  </a>
</p>
