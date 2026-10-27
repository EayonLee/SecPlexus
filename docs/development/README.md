# Development conventions

## Current scope

Web is runnable and based on the official Arco Design Pro Vite template, using
React and Arco Design. IAM and Console remain directory scaffolds. There are no
backend implementations, Dockerfiles, or deployment manifests yet.

From the repository root:

```sh
make dev     # Install dependencies and start development
make build   # Build the web app
make stop    # Stop running project services
```

`make dev` installs the locked pnpm dependencies and starts the current development
environment: Web at `http://127.0.0.1:5173`, with React Fast Refresh. Ctrl+C stops
the server. It does not start unimplemented IAM or Console services. The default
port is strict: an occupied port produces an error instead of silently moving.
Use `make stop` from another terminal to stop project services, including Web
started directly through pnpm in development or preview mode. The stop command
identifies local TCP services by their project directory and stops their child
processes too. It also stops containers whose Compose working directory belongs
to this repository when Compose configuration exists. It retains containers and
volumes. Repeating the command when nothing is running is safe.

Use `pnpm typecheck` for TypeScript and `pnpm lint` for Biome formatting checks.
See the [Web README](../../src/apps/web/README.md) for the mock boundary and backend
proxy configuration. The stop script requires `ps` and `lsof` on macOS or Linux;
Docker is needed only when Compose configuration has been added.

## Repository layout

```text
go.mod                      Shared Go module
src/apps/iam/               Identity and access management service
src/apps/console/           Platform application API
src/apps/web/               Web application
src/packages/               Libraries shared by multiple applications, added as needed
src/contracts/proto/        Internal RPC definitions
src/contracts/openapi/      HTTP API definitions
deploy/local/               Native development configuration
deploy/compose/             Docker Compose deployment
deploy/helm/                Kubernetes Helm deployment
scripts/dev/                Development tooling
```

`src/` groups application code, shared libraries, and interface contracts.
`src/apps/` is a repository convention for deployable applications, not a
language-specific standard. Each application retains its language's build and
dependency conventions.

## Go dependencies and imports

The module path follows the configured repository origin:
`github.com/EayonLee/SecPlexus`. The initial Go version is 1.24.0; dependency
adoption can raise it through an explicit toolchain update.

Go services share the root `go.mod`. A service has its executable under
`src/apps/<name>/cmd/<name>/` and private implementation under
`src/apps/<name>/internal/`. A service must not import another service's private
implementation. Extract libraries into `src/packages/` only when multiple
applications need the same implementation. Do not pre-create generic platform
packages.

Do not add per-service `go.mod`, local `replace` directives, or a `go.work` for
this single-module layout. `go.sum` will be generated when dependencies require
it; there is no placeholder checksum file.

## Web and future language workspaces

The root `package.json`, `pnpm-workspace.yaml`, and `pnpm-lock.yaml` manage the
Web workspace. Web owns its application dependencies in `src/apps/web/package.json`.
Use Node.js 22.12+ (24 recommended) and pnpm 10.32.1. Add workspace entries only
when real packages exist, and declare local dependencies using `workspace:`.
Do not import a sibling service's implementation through relative paths.

Python services and uv workspace configuration are outside the current scope.
They will declare their own packages and dependencies when introduced. Sharing
a repository does not require incompatible runtimes to share an environment.

## Local development and deployment

Extend `make dev` with service startup, dependency readiness, migrations, and
coordinated shutdown as backend applications are implemented. The current
entry point manages only the runnable Web application.

Each service will have its own Dockerfile and runtime image. Use the repository
root as the build context so builds can access the root Go module, contracts,
and shared packages. Once a service has a Dockerfile, its build convention is:

```sh
docker build -f src/apps/iam/Dockerfile .
```

Use selective `COPY` instructions and the root `.dockerignore`. Container
composition belongs in `deploy/compose/`; cluster packaging belongs in
`deploy/helm/`. Third-party components retain their own lifecycle and data
directories. Deployment directories currently contain placeholders only.

## References

- [Go module and server layout](https://go.dev/doc/modules/layout)
- [pnpm workspaces](https://pnpm.io/workspaces)
- [Docker build contexts](https://docs.docker.com/build/concepts/context/)
