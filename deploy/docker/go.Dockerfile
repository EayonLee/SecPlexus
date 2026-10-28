FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /source
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
ARG APP=iam
RUN CGO_ENABLED=0 go build -trimpath -o /out/service ./src/apps/${APP}/cmd

FROM alpine:3.21@sha256:ee16d92f96db08d9d2d07da48555a27f3a69d8f0759021dbc601195ed0d2b5bd
RUN apk add --no-cache ca-certificates su-exec && addgroup -g 10001 spx && adduser -D -u 10001 -G spx spx
ARG APP=iam
WORKDIR /app
COPY src/apps/${APP}/config.yaml /app/config.yaml
COPY --from=build /out/service /app/service
COPY --chmod=755 deploy/docker/go-entrypoint.sh /usr/local/bin/spx-entrypoint
ENTRYPOINT ["/usr/local/bin/spx-entrypoint"]
