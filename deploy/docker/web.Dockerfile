FROM node:24-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53 AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1 HUSKY=0
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY src/apps/web/package.json src/apps/web/package.json
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY src/apps/web src/apps/web
COPY src/apps/web/.env.example src/apps/web/.env.production
RUN pnpm --dir src/apps/web build

FROM node:24-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1
COPY --from=build --chown=node:node /app /app
USER node
EXPOSE 8080
CMD ["node", "src/apps/web/node_modules/next/dist/bin/next", "start", "src/apps/web", "--hostname", "0.0.0.0", "--port", "8080"]
